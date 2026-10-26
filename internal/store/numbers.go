package store

import (
	"cmp"
	"errors"
	"fmt"
	"log"
	"maps"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/issue"
)

// The project's own numbers (CO2-3075): refs/board/seq, a chain of commits
// whose tree holds one file, numbers:
//
//	prefix CO2
//	start 3052
//	3052 <issue id>
//	3053 <issue id>
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
	seqRef  = "refs/board/seq"
	seqFile = "numbers"
)

// seqCopy is where a remote's counter is fetched to, beside its entities.
func seqCopy(remote string) string { return "refs/board-remotes/" + remote + "/seq" }

var prefixRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)

// Counter is the project's numbers, as refs/board/seq holds them.
type Counter struct {
	Prefix string         // CO2
	Start  int            // the first number the counter hands out
	ByID   map[string]int // the numbers it handed out, by issue id
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
func (b *Backend) Counter() (c Counter, ok bool, err error) { return readCounter(b.s) }

func readCounter(s *gitstore.Store) (c Counter, ok bool, err error) {
	h, err := s.Ref(seqRef)
	if err != nil || h == gitstore.ZeroHash {
		return Counter{}, false, err
	}
	commit, err := s.ReadCommit(h)
	if err != nil {
		return Counter{}, false, fmt.Errorf("reading %s: %w", seqRef, err)
	}
	entries, err := s.ReadTree(commit.Tree)
	if err != nil {
		return Counter{}, false, fmt.Errorf("reading %s: %w", seqRef, err)
	}
	for _, e := range entries {
		if e.Name != seqFile {
			continue
		}
		data, err := s.ReadBlob(e.Blob)
		if err != nil {
			return Counter{}, false, fmt.Errorf("reading %s: %w", seqRef, err)
		}
		c, err := ParseCounter(data)
		return c, err == nil, err
	}
	return Counter{}, false, fmt.Errorf("%s holds no %s file", seqRef, seqFile)
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

// ErrNumbersAhead: this clone's counter holds numbers the remote hasn't,
// which taking the remote's would drop: switch --apply's, until
// refs/board/seq is pushed, or a numbering killed before its push.
var ErrNumbersAhead = errors.New("refs/board/seq has numbers the remote hasn't")

// FetchNumbers brings the remote's counter and makes it this clone's, when
// this clone's is behind it or the same: numbering pushes at once. It
// refuses one ahead, or apart (ErrNumbersAhead). A remote without one is no
// error: the project doesn't number its issues.
func (b *Backend) FetchNumbers(remote string) error {
	theirs, err := b.numbersOf(remote)
	if err != nil || theirs == gitstore.ZeroHash {
		return err
	}
	return b.setSeq(theirs)
}

// numbersBehind is FetchNumbers' refusal alone, for a check: the remote's
// counter fetched to its copy here, this clone's left as it is.
func (b *Backend) numbersBehind(remote string) error {
	_, err := b.numbersOf(remote)
	return err
}

// numbersOf fetches the remote's counter and answers it, ZeroHash for
// none; ErrNumbersAhead when this clone's isn't it or behind it.
func (b *Backend) numbersOf(remote string) (gitstore.Hash, error) {
	theirs, err := b.remoteSeq(remote)
	if err != nil || theirs == gitstore.ZeroHash {
		return theirs, err
	}
	ours, err := b.s.Ref(seqRef)
	if err != nil {
		return gitstore.ZeroHash, err
	}
	if ok, err := b.behind(ours, theirs); err != nil || !ok {
		return gitstore.ZeroHash, cmp.Or(err, fmt.Errorf("%w: git push %s %s first; or, to drop them, git update-ref %s %s",
			ErrNumbersAhead, remote, seqRef, seqRef, seqCopy(remote)))
	}
	return theirs, nil
}

// NumbersAhead says this clone's counter holds numbers the remote hasn't
// (ErrNumbersAhead's case): its push waits, and syncing and numbering
// refuse. It asks the remote only when its copy here says so.
func (b *Backend) NumbersAhead(remote string) (bool, error) {
	ours, err := b.s.Ref(seqRef)
	if err != nil || ours == gitstore.ZeroHash {
		return false, err
	}
	if has, err := b.hasRemote(remote); err != nil || !has {
		return false, err
	}
	if theirs, err := b.s.Ref(seqCopy(remote)); err == nil && theirs != gitstore.ZeroHash {
		if ok, err := b.behind(ours, theirs); err == nil && ok {
			return false, nil
		}
	}
	_, err = b.numbersOf(remote)
	if errors.Is(err, ErrNumbersAhead) {
		return true, nil
	}
	return false, err
}

// behind says whether ours is theirs or one of its ancestors: taking
// theirs then drops nothing.
func (b *Backend) behind(ours, theirs gitstore.Hash) (bool, error) {
	if ours == gitstore.ZeroHash || ours == theirs {
		return true, nil
	}
	_, err := b.run(nil, "merge-base", "--is-ancestor", ours.String(), theirs.String())
	if ee := (*exec.ExitError)(nil); errors.As(err, &ee) && ee.ExitCode() == 1 {
		return false, nil
	}
	return err == nil, err
}

// remoteSeq fetches the remote's counter and says where it is: ZeroHash
// for none.
func (b *Backend) remoteSeq(remote string) (gitstore.Hash, error) {
	if err := b.checkRemote(remote); err != nil {
		return gitstore.ZeroHash, err
	}
	if _, err := b.run(nil, "fetch", "--no-write-fetch-head", remote, "+"+seqRef+":"+seqCopy(remote)); err != nil {
		if strings.Contains(err.Error(), "couldn't find remote ref") {
			return gitstore.ZeroHash, nil
		}
		return gitstore.ZeroHash, fmt.Errorf("fetching the numbers from %s: %w", remote, err)
	}
	return b.s.Ref(seqCopy(remote))
}

// setSeq points this clone's counter at h; ZeroHash removes it.
func (b *Backend) setSeq(h gitstore.Hash) error {
	ours, err := b.s.Ref(seqRef)
	switch {
	case err != nil:
		return err
	case ours == h:
		return nil
	case h == gitstore.ZeroHash:
		_, err := b.run(nil, "update-ref", "-d", seqRef, ours.String())
		return err
	}
	return b.s.UpdateRef(ctx, seqRef, h, ours)
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
	rejected, err := b.pushCounter(remote, Counter{Prefix: prefix, Start: start, ByID: map[string]int{}}, gitstore.ZeroHash)
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
	head, err := b.s.Ref(seqRef)
	if err != nil {
		return 0, false, err
	}
	todo, _, err := b.survey(remote, c)
	if err != nil || len(todo) == 0 {
		return 0, false, err
	}
	next := c.Next()
	for i, is := range todo {
		c.ByID[is.ID] = next + i
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
func (b *Backend) survey(remote string, c Counter) (todo []*issue.Issue, n Numbering, err error) {
	if err := validRemote(remote); err != nil {
		return nil, n, err
	}
	prefix := "refs/board-remotes/" + remote + "/issue/"
	onRemote, err := b.s.Refs(prefix)
	if err != nil {
		return nil, n, err
	}
	issues, err := b.allIssues()
	if err != nil {
		return nil, n, err
	}
	v, err := b.view()
	if err != nil {
		return nil, n, err
	}
	n.GitHub = []int{}
	for _, id := range slices.Sorted(maps.Keys(issues)) {
		forges := forgesOf(v.records[id])
		github := 0
		for _, f := range forges {
			num, _ := strconv.Atoi(f.Number)
			n.Highest = max(n.Highest, num)
			if f.Name == "GitHub" && num > 0 && (github == 0 || num < github) {
				github = num
			}
		}
		_, there := onRemote[prefix+id]
		switch {
		case c.NumberOf(id, forges) > 0:
			n.Numbered++
		case there:
			n.Waiting++
			todo = append(todo, issues[id])
		default:
			n.Unpushed++
		}
		if there {
			n.GitHub = append(n.GitHub, github)
		}
	}
	slices.SortFunc(todo, func(x, y *issue.Issue) int { return cmp.Or(cmp.Compare(x.Created, y.Created), cmp.Compare(x.ID, y.ID)) })
	return todo, n, nil
}

// pushCounter commits c on parent (ZeroHash for the first commit) and
// pushes it. When the push fails, the remote's counter says what happened,
// since what git says differs by server and by race: ours (it landed; a
// mirror among the remote's push URLs refused it), someone else's
// (rejected: they took the number), or still parent (a real failure, the
// counter as it was).
func (b *Backend) pushCounter(remote string, c Counter, parent gitstore.Hash) (rejected bool, err error) {
	commit, err := commitCounter(b.s, c, parent)
	if err != nil {
		return false, err
	}
	if _, err := b.run(nil, "push", remote, seqRef+":"+seqRef); err != nil {
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

// commitCounter commits c on parent (ZeroHash for the first commit) as
// refs/board/seq, and answers the commit.
func commitCounter(s *gitstore.Store, c Counter, parent gitstore.Hash) (gitstore.Hash, error) {
	data, err := s.WriteBlob(c.Bytes())
	if err != nil {
		return gitstore.ZeroHash, err
	}
	tree, err := s.WriteTree([]gitstore.TreeEntry{{Name: seqFile, Blob: data}})
	if err != nil {
		return gitstore.ZeroHash, err
	}
	var parents []gitstore.Hash
	if parent != gitstore.ZeroHash {
		parents = append(parents, parent)
	}
	commit, err := s.WriteCommit(gitstore.Commit{Tree: tree, Parents: parents})
	if err != nil {
		return gitstore.ZeroHash, err
	}
	return commit, s.UpdateRef(ctx, seqRef, commit, parent)
}

// numbersToSend says whether this clone's counter is strictly ahead of
// the remote's: the remote's is one of its ancestors, so sending ours
// drops nothing there. A remote without a counter numbers nothing.
func (b *Backend) numbersToSend(remote string) (bool, error) {
	theirs, err := b.remoteSeq(remote)
	if err != nil || theirs == gitstore.ZeroHash {
		return false, err
	}
	ours, err := b.s.Ref(seqRef)
	if err != nil || ours == gitstore.ZeroHash || ours == theirs {
		return false, err
	}
	return b.behind(theirs, ours)
}
