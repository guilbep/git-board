package gitbugio

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/git-bug/git-bug/cache"
	"github.com/git-bug/git-bug/repository"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

// The project's own numbers (CO2-3075): refs/board/seq, a chain of commits
// whose tree holds one file, numbers:
//
//	prefix CO2
//	start 3052
//	3052 <git-bug id>
//	3053 <git-bug id>
//
// An issue's number is its line there; else its GitHub number when below
// start, the number it had before the project numbered its own; else none
// yet. Numbering pushes the next commit: a push git refuses (not a
// fast-forward) means someone else took the number, so fetch and take the
// next one.

// NumbersRemote is the one remote that hands numbers out. A mirror never
// does: two counters would give a number twice.
// ponytail: origin only, as internal/branches; a setting when a project's isn't
const NumbersRemote = "origin"

const (
	seqNamespace = "board"
	seqKey       = "seq"
	seqFile      = "numbers"
)

var prefixRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)

// Counter is the project's numbers, as refs/board/seq holds them.
type Counter struct {
	Prefix string         // CO2
	Start  int            // the first number the counter hands out
	ByID   map[string]int // the numbers it handed out, by git-bug id
}

// ParseCounter reads a numbers file.
func ParseCounter(data []byte) (Counter, error) {
	c := Counter{ByID: map[string]int{}}
	for i, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		key, value, _ := strings.Cut(strings.TrimSpace(line), " ")
		switch key {
		case "prefix":
			c.Prefix = value
		case "start":
			n, err := strconv.Atoi(value)
			if err != nil {
				return Counter{}, fmt.Errorf("numbers, line %d: start %q is not a number", i+1, value)
			}
			c.Start = n
		default:
			n, err := strconv.Atoi(key)
			if err != nil || value == "" {
				return Counter{}, fmt.Errorf("numbers, line %d: %q is not a number and an issue", i+1, line)
			}
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

// Next is the number the counter hands out next.
func (c Counter) Next() int {
	next := c.Start
	for _, n := range c.ByID {
		next = max(next, n+1)
	}
	return next
}

// NumberOf is an issue's number, 0 for none yet: its line, else its GitHub
// number below Start.
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

// Counter is the project's numbers as this clone last fetched them; ok is
// false while the project doesn't number its issues.
func (b *Backend) Counter() (c Counter, ok bool, err error) {
	h, err := b.repo.ResolveRef(seqNamespace, seqKey)
	if errors.Is(err, repository.ErrNotFound) {
		return Counter{}, false, nil
	}
	if err != nil {
		return Counter{}, false, err
	}
	commit, err := b.repo.ReadCommit(h)
	if err != nil {
		return Counter{}, false, fmt.Errorf("reading refs/board/seq: %w", err)
	}
	entries, err := b.repo.ReadTree(commit.TreeHash)
	if err != nil {
		return Counter{}, false, fmt.Errorf("reading refs/board/seq: %w", err)
	}
	for _, e := range entries {
		if e.Name != seqFile {
			continue
		}
		r, err := b.repo.ReadData(e.Hash)
		if err != nil {
			return Counter{}, false, fmt.Errorf("reading refs/board/seq: %w", err)
		}
		defer r.Close()
		data, err := io.ReadAll(r)
		if err != nil {
			return Counter{}, false, fmt.Errorf("reading refs/board/seq: %w", err)
		}
		c, err := ParseCounter(data)
		return c, err == nil, err
	}
	return Counter{}, false, fmt.Errorf("refs/board/seq holds no %s file", seqFile)
}

// counter is Counter for a listing: nil while the project doesn't number
// its issues.
func (b *Backend) counter() (*Counter, error) {
	c, ok, err := b.Counter()
	if !ok {
		return nil, err
	}
	return &c, nil
}

// FetchNumbers brings the remote's counter and makes it this clone's: a
// clone never holds numbers the remote hasn't, as numbering pushes at once.
// A remote without one is no error: the project doesn't number its issues.
func (b *Backend) FetchNumbers(remote string) error {
	if _, err := b.repo.FetchRefs(remote, seqNamespace); err != nil && !errors.Is(err, transport.ErrEmptyRemoteRepository) {
		return fmt.Errorf("fetching the numbers from %s: %w", remote, err)
	}
	theirs, err := b.repo.ResolveTrackingRef(remote, seqNamespace, seqKey)
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	ours, err := b.repo.ResolveRef(seqNamespace, seqKey)
	if errors.Is(err, repository.ErrNotFound) {
		ours = ""
	} else if err != nil {
		return err
	}
	if ours == theirs {
		return nil
	}
	return b.repo.UpdateRef(seqNamespace, seqKey, ours, theirs)
}

// InitNumbers makes the project number its issues, as prefix-N from start:
// the counter's first commit, pushed to remote. Pushing it is the switch.
func (b *Backend) InitNumbers(remote, prefix string, start int) error {
	if !prefixRe.MatchString(prefix) {
		return fmt.Errorf("a prefix is a letter, then letters or digits, like CO2; not %q", prefix)
	}
	if start < 1 {
		return fmt.Errorf("numbers start from 1 at least, not %d", start)
	}
	if err := b.FetchNumbers(remote); err != nil {
		return err
	}
	if c, ok, err := b.Counter(); err != nil || ok {
		if err == nil {
			err = fmt.Errorf("the project already numbers its issues: %s, from %d", c.Prefix, c.Start)
		}
		return err
	}
	_, err := b.pushCounter(remote, Counter{Prefix: prefix, Start: start, ByID: map[string]int{}}, "")
	return err
}

// NumberIssues gives the issues without a number the next ones, oldest
// first, and says how many it numbered. A project that doesn't number its
// issues, as this clone last fetched it, touches no remote.
func (b *Backend) NumberIssues(remote string) (int, error) {
	if _, ok, err := b.Counter(); !ok {
		return 0, err
	}
	// ponytail: five tries; more people numbering in the same second is
	// beyond a team of ten
	for range 5 {
		if err := b.FetchNumbers(remote); err != nil {
			return 0, err
		}
		n, rejected, err := b.numberOnce(remote)
		if !rejected {
			return n, err
		}
	}
	return 0, errors.New("the numbers kept moving: sync again")
}

// numberOnce numbers the issues without one on the counter as fetched, and
// pushes it; rejected: someone else pushed first.
func (b *Backend) numberOnce(remote string) (n int, rejected bool, err error) {
	c, _, err := b.Counter()
	if err != nil {
		return 0, false, err
	}
	head, err := b.repo.ResolveRef(seqNamespace, seqKey)
	if err != nil {
		return 0, false, err
	}
	var todo []*cache.BugExcerpt
	for _, id := range b.c.Bugs().AllIds() {
		e, err := b.c.Bugs().ResolveExcerpt(id)
		if err != nil {
			return 0, false, err
		}
		if forges, _ := forgesOf(e.CreateMetadata); c.NumberOf(id.String(), forges) == 0 {
			todo = append(todo, e)
		}
	}
	if len(todo) == 0 {
		return 0, false, nil
	}
	sort.Slice(todo, func(i, j int) bool {
		if todo[i].CreateUnixTime != todo[j].CreateUnixTime {
			return todo[i].CreateUnixTime < todo[j].CreateUnixTime
		}
		return todo[i].Id() < todo[j].Id()
	})
	next := c.Next()
	for i, e := range todo {
		c.ByID[e.Id().String()] = next + i
	}
	if rejected, err := b.pushCounter(remote, c, head); err != nil {
		return 0, rejected, err
	}
	return len(todo), false, nil
}

// pushCounter commits c on parent ("" for the first commit) and pushes it.
// A failed push leaves the counter as it was; rejected says git refused it,
// someone else having pushed first.
func (b *Backend) pushCounter(remote string, c Counter, parent repository.Hash) (rejected bool, err error) {
	data, err := b.repo.StoreData(c.Bytes())
	if err != nil {
		return false, err
	}
	tree, err := b.repo.StoreTree([]repository.TreeEntry{{ObjectType: repository.Blob, Hash: data, Name: seqFile}})
	if err != nil {
		return false, err
	}
	var parents []repository.Hash
	if parent != "" {
		parents = append(parents, parent)
	}
	commit, err := b.repo.StoreCommit(tree, parents...)
	if err != nil {
		return false, err
	}
	if err := b.repo.UpdateRef(seqNamespace, seqKey, parent, commit); err != nil {
		return false, err
	}
	if _, err := b.repo.PushRefs(remote, seqNamespace); err != nil {
		var undo error
		if parent == "" {
			undo = b.repo.RemoveRef(seqNamespace, seqKey)
		} else {
			undo = b.repo.UpdateRef(seqNamespace, seqKey, commit, parent)
		}
		return strings.Contains(err.Error(), "non-fast-forward"), errors.Join(fmt.Errorf("pushing the numbers to %s: %w", remote, err), undo)
	}
	return false, nil
}
