package issues

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
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
