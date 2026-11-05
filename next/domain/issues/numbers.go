package issues

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/guilbep/git-board/next/kernel"
	bsync "github.com/guilbep/git-board/next/sync"
)

// Counter is the project numbers' counter (ADR 0016; datastores.md, The
// counter): a prefix, the first number, and each numbered issue's.
type Counter struct {
	Prefix string
	Start  int
	ByID   map[string]int
}

var prefixRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)

// ParseCounter reads a numbers file, as v1 reads it: blank lines, and
// keys a later version may write, are skipped; a number twice is refused.
func ParseCounter(data []byte) (Counter, error) {
	c := Counter{ByID: map[string]int{}}
	taken := map[int]string{}
	for i, line := range strings.Split(string(data), "\n") {
		key, value, _ := strings.Cut(strings.TrimSpace(line), " ")
		n, notNumber := strconv.Atoi(key)
		switch {
		case key == "":
		case key == "prefix":
			c.Prefix = value
		case key == "start":
			s, err := strconv.Atoi(value)
			if err != nil {
				return Counter{}, fmt.Errorf("numbers, line %d: start %q is not a number", i+1, value)
			}
			c.Start = s
		case notNumber != nil: // a later version's
		case value == "":
			return Counter{}, fmt.Errorf("numbers, line %d: %d names no issue", i+1, n)
		case taken[n] != "":
			return Counter{}, fmt.Errorf("numbers, line %d: %d is both %s and %s", i+1, n, taken[n], value)
		default:
			taken[n] = value
			c.ByID[value] = n
		}
	}
	if !prefixRe.MatchString(c.Prefix) || c.Start < 1 {
		return Counter{}, fmt.Errorf("numbers: a prefix and a start, not %q and %d", c.Prefix, c.Start)
	}
	return c, nil
}

// Bytes is the numbers file, its lines by number.
func (c Counter) Bytes() []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "prefix %s\nstart %d\n", c.Prefix, c.Start)
	for _, id := range slices.SortedFunc(maps.Keys(c.ByID), func(x, y string) int { return cmp.Compare(c.ByID[x], c.ByID[y]) }) {
		fmt.Fprintf(&b, "%d %s\n", c.ByID[id], id)
	}
	return []byte(b.String())
}

// Next is the number handed out next.
func (c Counter) Next() int {
	next := c.Start
	for _, n := range c.ByID {
		next = max(next, n+1)
	}
	return next
}

// NumberOf is an issue's number, 0 for none yet: its line, else its
// GitHub number below Start.
func (c Counter) NumberOf(id string, forges []Forge) int {
	if n, ok := c.ByID[id]; ok {
		return n
	}
	for _, f := range forges {
		if n, err := strconv.Atoi(f.Number); f.Name == "GitHub" && err == nil && n < c.Start {
			return n
		}
	}
	return 0
}

// Format is a number as people write it: CO2-3075.
func (c Counter) Format(n int) string { return fmt.Sprintf("%s-%d", c.Prefix, n) }

// counter is the board's counter, nil while the project doesn't number
// its issues. One that doesn't read is told to notice, and the issues
// show without numbers, as v1 shows them.
func (s *Service) counter(ctx context.Context, notice Notice) *Counter {
	c := s.Board.Store().Caps().Counter
	if c == nil {
		return nil
	}
	data, _, err := c.Count(ctx)
	if err == nil && data == nil {
		return nil
	}
	var cnt Counter
	if err == nil {
		cnt, err = ParseCounter(data)
	}
	if err != nil {
		if notice != nil {
			notice("refs/board/seq: %v; the issues show without their numbers", err)
		}
		return nil
	}
	return &cnt
}

// numbered is an issue's own number and the counter's mark on it.
func (is *Issue) numbered(c *Counter) {
	if c == nil {
		return
	}
	is.Counted = true
	if n := c.NumberOf(is.ID, is.Forges); n > 0 {
		is.Number = c.Format(n)
	}
}

// ErrNumbered: the project numbers its issues already; it starts once.
var ErrNumbered = errors.New("the project already numbers its issues")

// ErrNumbersMoved: other writers kept numbering first, try after try.
var ErrNumbersMoved = errors.New("the numbers kept moving")

// ErrNoCounter: the datastore keeps no counter.
var ErrNoCounter = errors.New("project numbers need a datastore that counts")

// CheckNumbers says what's wrong with a prefix and a start.
func CheckNumbers(prefix string, start int) error {
	if !prefixRe.MatchString(prefix) {
		return fmt.Errorf("a prefix is a letter, then letters or digits, like CO2; not %q", prefix)
	}
	if start < 1 {
		return fmt.Errorf("numbers start from 1 at least, not %d", start)
	}
	return nil
}

// countWaiter is a datastore whose counter can't always be written here:
// a Markdown folder's, on the default branch alone.
type countWaiter interface {
	CountWaits(ctx context.Context) error
}

// InitNumbers starts the counter of a datastore that keeps one, shared
// by every writer (PostgreSQL, a Markdown folder): prefix-N from start.
// ErrNumbered when one was started, by someone else first too.
func (s *Service) InitNumbers(ctx context.Context, prefix string, start int) error {
	if err := CheckNumbers(prefix, start); err != nil {
		return err
	}
	c := s.Board.Store().Caps().Counter
	if c == nil {
		return ErrNoCounter
	}
	_, err := c.SetCount(ctx, Counter{Prefix: prefix, Start: start, ByID: map[string]int{}}.Bytes(), "")
	if errors.Is(err, kernel.ErrMoved) {
		data, _, cerr := c.Count(ctx)
		if cerr != nil {
			return cerr
		}
		was, cerr := ParseCounter(data)
		if cerr != nil {
			return cerr
		}
		return fmt.Errorf("%w: %s, from %d", ErrNumbered, was.Prefix, was.Start)
	}
	return err
}

// Number gives every issue without a number the next ones, oldest
// first, on the datastore's counter: one write that lands only if no one
// wrote first, five tries. A GitHub number below the start, from the
// sync records, is the issue's own. It says how many it numbered; a
// project that doesn't number its issues numbers none.
func (s *Service) Number(ctx context.Context) (int, error) {
	ctx = s.Board.Snapshot(ctx)
	ds := s.Board.Store()
	c := ds.Caps().Counter
	if c == nil {
		return 0, nil
	}
	if data, _, err := c.Count(ctx); err != nil || data == nil {
		return 0, err
	}
	if w, ok := ds.(countWaiter); ok {
		if err := w.CountWaits(ctx); err != nil {
			return 0, err
		}
	}
	var last error
	for range 5 {
		data, version, err := c.Count(ctx)
		if err != nil {
			return 0, err
		}
		cnt, err := ParseCounter(data)
		if err != nil {
			return 0, err
		}
		todo, err := s.unnumbered(ctx, cnt, s.reading(ctx, s.notice, false).recs)
		if err != nil || len(todo) == 0 {
			return 0, err
		}
		next := cnt.Next()
		for i, id := range todo {
			cnt.ByID[id] = next + i
		}
		_, err = c.SetCount(ctx, cnt.Bytes(), version)
		if err == nil {
			return len(todo), nil
		}
		if !errors.Is(err, kernel.ErrMoved) {
			return 0, err
		}
		last = err
	}
	return 0, fmt.Errorf("%w, sync again: %w", ErrNumbersMoved, last)
}

// unnumbered are the issues c gives no number, oldest first; a deleted
// one is none.
func (s *Service) unnumbered(ctx context.Context, c Counter, recs map[string][]bsync.Record) ([]string, error) {
	rows, err := Kind.List(ctx, s.Board, kernel.Query{Sort: "created"})
	if err != nil {
		return nil, err
	}
	type issue struct {
		id      string
		created int64
	}
	var todo []issue
	for _, r := range rows {
		var is Issue
		is.forged(recs[string(r.ID)])
		if c.NumberOf(string(r.ID), is.Forges) == 0 {
			todo = append(todo, issue{string(r.ID), r.Row.Created})
		}
	}
	slices.SortFunc(todo, func(x, y issue) int { return cmp.Or(cmp.Compare(x.created, y.created), cmp.Compare(x.id, y.id)) })
	ids := make([]string, len(todo))
	for i, t := range todo {
		ids[i] = t.id
	}
	return ids, nil
}
