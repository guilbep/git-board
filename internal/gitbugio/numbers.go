package gitbugio

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/git-bug/git-bug/cache"
	bugent "github.com/git-bug/git-bug/entities/bug"
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

// ParseCounter reads a numbers file. Blank lines, and keys a later
// git-board may write, are skipped; a number twice is refused.
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
		case notNumber != nil:
			// a later git-board's
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

// Next is the number the counter hands out next.
func (c Counter) Next() int {
	next := c.Start
	for _, n := range c.ByID {
		next = max(next, n+1)
	}
	return next
}

// NumberOf is an issue's number, 0 for none yet: its line, else its GitHub
// number below Start, from a GitHub issue it is still on: an issue linked
// to that one since has it, and an unlinked one waits for a line.
func (c Counter) NumberOf(id string, forges []Forge) int {
	if n, ok := c.ByID[id]; ok {
		return n
	}
	for _, f := range forges {
		if n, err := strconv.Atoi(f.Number); f.Name == "GitHub" && f.Gone == "" && err == nil && n < c.Start {
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
// its issues, and when this git-board can't read its counter, which is
// logged: the issues still list, without numbers; NumberIssues refuses.
func (b *Backend) counter() *Counter {
	c, ok, err := b.Counter()
	if err != nil {
		log.Printf("refs/board/seq: %v; the issues show without their numbers", err)
	}
	if !ok {
		return nil
	}
	return &c
}

// FetchNumbers brings the remote's counter and makes it this clone's: a
// clone never holds numbers the remote hasn't, as numbering pushes at once.
// A remote without one is no error: the project doesn't number its issues.
func (b *Backend) FetchNumbers(remote string) error {
	theirs, err := b.remoteSeq(remote)
	if err != nil || theirs == "" {
		return err
	}
	return b.setSeq(theirs)
}

// remoteSeq fetches the remote's counter and says where it is: "" for none.
func (b *Backend) remoteSeq(remote string) (repository.Hash, error) {
	if _, err := b.repo.FetchRefs(remote, seqNamespace); err != nil && !errors.Is(err, transport.ErrEmptyRemoteRepository) {
		return "", fmt.Errorf("fetching the numbers from %s: %w", remote, err)
	}
	theirs, err := b.repo.ResolveTrackingRef(remote, seqNamespace, seqKey)
	if errors.Is(err, repository.ErrNotFound) {
		return "", nil
	}
	return theirs, err
}

// setSeq points this clone's counter at h; "" removes it.
func (b *Backend) setSeq(h repository.Hash) error {
	ours, err := b.repo.ResolveRef(seqNamespace, seqKey)
	if errors.Is(err, repository.ErrNotFound) {
		ours = ""
	} else if err != nil {
		return err
	}
	switch {
	case ours == h:
		return nil
	case h == "":
		return b.repo.RemoveRef(seqNamespace, seqKey)
	}
	return b.repo.UpdateRef(seqNamespace, seqKey, ours, h)
}

// ErrNumbered: the project numbers its issues already. It starts once.
var ErrNumbered = errors.New("the project already numbers its issues")

// ErrNumbersMoved: other clones kept numbering first, try after try.
var ErrNumbersMoved = errors.New("the numbers kept moving")

// CheckNumbers says what's wrong with a prefix and a start, nil when
// InitNumbers takes them.
func CheckNumbers(prefix string, start int) error {
	if !prefixRe.MatchString(prefix) {
		return fmt.Errorf("a prefix is a letter, then letters or digits, like CO2; not %q", prefix)
	}
	if start < 1 {
		return fmt.Errorf("numbers start from 1 at least, not %d", start)
	}
	return nil
}

// InitNumbers makes the project number its issues, as prefix-N from start:
// the counter's first commit, pushed to remote. Pushing it is the switch.
// When the remote has one already, or another clone pushed one first, it
// says so (ErrNumbered) and this clone takes that one.
func (b *Backend) InitNumbers(remote, prefix string, start int) error {
	if err := CheckNumbers(prefix, start); err != nil {
		return err
	}
	if err := b.FetchNumbers(remote); err != nil {
		return err
	}
	numbered := func() error {
		c, ok, err := b.Counter()
		if err == nil && ok {
			err = fmt.Errorf("%w: %s, from %d", ErrNumbered, c.Prefix, c.Start)
		}
		return err
	}
	if err := numbered(); err != nil {
		return err
	}
	rejected, err := b.pushCounter(remote, Counter{Prefix: prefix, Start: start, ByID: map[string]int{}}, "")
	if rejected {
		if e := numbered(); e != nil {
			return e
		}
	}
	return err
}

// NumberIssues gives the issues origin has and that have no number the
// next ones, oldest first, and says how many it numbered; one made here
// waits for its push. A project that doesn't number its
// issues, as this clone last fetched it, touches no remote.
func (b *Backend) NumberIssues(remote string) (int, error) {
	if _, ok, err := b.Counter(); !ok {
		return 0, err
	}
	// ponytail: five tries; more people numbering in the same second is
	// beyond a team of ten
	var last error
	for range 5 {
		if err := b.FetchNumbers(remote); err != nil {
			return 0, err
		}
		n, rejected, err := b.numberOnce(remote)
		if !rejected {
			return n, err
		}
		last = err
	}
	return 0, fmt.Errorf("%w, sync again: %w", ErrNumbersMoved, last)
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
	todo, _, err := b.survey(remote, c)
	if err != nil || len(todo) == 0 {
		return 0, false, err
	}
	next := c.Next()
	for i, e := range todo {
		c.ByID[e.Id().String()] = next + i
	}
	if rejected, err := b.pushCounter(remote, c, head); err != nil {
		return 0, rejected, err
	}
	return len(todo), false, nil
}

// Numbering is how the issues here stand with the project's numbers, as
// this clone last fetched the counter and the remote's issues.
type Numbering struct {
	Numbered int `json:"numbered"` // with a number: their line, or their GitHub one below start
	Waiting  int `json:"waiting"`  // on the remote without one: NumberIssues numbers them
	Unpushed int `json:"unpushed"` // without one, not on the remote yet: their push numbers them
	Highest  int `json:"highest"`  // the highest forge number of an issue here, 0 for none
	// GitHub is each issue on the remote by its GitHub number (its lowest,
	// 0 for none): what a start would keep of them, before the switch
	GitHub []int `json:"github_numbers,omitempty"`
}

// Numbering says how the issues here stand; before the switch, all those
// on the remote wait.
func (b *Backend) Numbering(remote string) (Numbering, error) {
	c, _, err := b.Counter()
	if err != nil {
		return Numbering{}, err
	}
	_, n, err := b.survey(remote, c)
	return n, err
}

// survey goes through the issues here with c. todo are those numbering
// numbers now, oldest first: on the remote, as last pushed or fetched (a
// number other clones can't find an issue for is a gap), without one.
func (b *Backend) survey(remote string, c Counter) (todo []*cache.BugExcerpt, n Numbering, err error) {
	onRemote, err := b.repo.ListTrackingRefs(remote, bugent.Namespace)
	if err != nil {
		return nil, n, err
	}
	n.GitHub = []int{}
	for _, id := range b.c.Bugs().AllIds() {
		e, err := b.c.Bugs().ResolveExcerpt(id)
		if err != nil {
			return nil, n, err
		}
		forges, _ := forgesOf(e.CreateMetadata)
		github := 0
		for _, f := range forges {
			num, _ := strconv.Atoi(f.Number)
			n.Highest = max(n.Highest, num) // a gone one's too: still taken there
			if f.Name == "GitHub" && f.Gone == "" && num > 0 && (github == 0 || num < github) {
				github = num
			}
		}
		_, there := onRemote[id.String()]
		switch {
		case c.NumberOf(id.String(), forges) > 0:
			n.Numbered++
		case there:
			n.Waiting++
			todo = append(todo, e)
		default:
			n.Unpushed++
		}
		if there {
			n.GitHub = append(n.GitHub, github)
		}
	}
	sort.Slice(todo, func(i, j int) bool {
		if todo[i].CreateUnixTime != todo[j].CreateUnixTime {
			return todo[i].CreateUnixTime < todo[j].CreateUnixTime
		}
		return todo[i].Id() < todo[j].Id()
	})
	return todo, n, nil
}

// pushCounter commits c on parent ("" for the first commit) and pushes it.
// When the push fails, the remote's counter says what happened, since what
// git says differs by server and by race: ours (it landed; a mirror among
// the remote's push URLs refused it), someone else's (rejected: they took
// the number), or still parent (a real failure, the counter as it was).
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
		err = fmt.Errorf("pushing the numbers to %s: %w", remote, err)
		theirs, ferr := b.remoteSeq(remote)
		switch {
		case ferr != nil:
			return false, errors.Join(err, ferr, b.setSeq(parent))
		case theirs == commit:
			return false, nil
		case theirs == parent:
			return false, errors.Join(err, b.setSeq(parent))
		}
		return true, errors.Join(err, b.setSeq(theirs))
	}
	return false, nil
}
