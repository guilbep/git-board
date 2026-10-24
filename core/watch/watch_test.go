package watch

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/oplog"
)

const every = 10 * time.Millisecond

func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
}

func open(t *testing.T, dir string) *gitstore.Store {
	t.Helper()
	s, err := gitstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func create(t *testing.T, s *gitstore.Store, kind string) *oplog.Log {
	t.Helper()
	l, err := oplog.Create(t.Context(), s, kind, "", []oplog.Op{{Kind: "create", Time: 1}})
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// next waits up to 5 intervals for a batch.
func next(t *testing.T, ch <-chan []Change) []Change {
	t.Helper()
	select {
	case b := <-ch:
		return b
	case <-time.After(5 * every):
		t.Fatal("no change within 5 intervals")
		return nil
	}
}

func quiet(t *testing.T, ch <-chan []Change) {
	t.Helper()
	select {
	case b := <-ch:
		t.Fatalf("unexpected changes %v", b)
	case <-time.After(5 * every):
	}
}

func TestWatchSeesOtherStore(t *testing.T) {
	t.Parallel()
	dir := newRepo(t)
	ch := Watch(t.Context(), open(t, dir), every)
	l := create(t, open(t, dir), "issue")
	if got, want := next(t, ch), []Change{{"issue", l.ID, l.Head}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
	quiet(t, ch)
}

func TestWatchQuiet(t *testing.T) {
	t.Parallel()
	s := open(t, newRepo(t))
	create(t, s, "issue") // in the baseline
	quiet(t, Watch(t.Context(), s, every))
}

func TestWatchRemoval(t *testing.T) {
	t.Parallel()
	dir := newRepo(t)
	s := open(t, dir)
	l := create(t, s, "person")
	ch := Watch(t.Context(), s, every)
	if out, err := exec.Command("git", "-C", dir, "update-ref", "-d", oplog.RefName("person", l.ID)).CombinedOutput(); err != nil {
		t.Fatalf("git update-ref -d: %v\n%s", err, out)
	}
	if got, want := next(t, ch), []Change{{"person", l.ID, gitstore.ZeroHash}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
}

func TestWatchStops(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	ch := Watch(ctx, open(t, newRepo(t)), every)
	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("a batch after the cancel")
		}
	case <-time.After(5 * every):
		t.Fatal("the channel is still open")
	}
}

// world is what a poll lists; fail is the prefix whose listing fails.
type world struct {
	refs map[string]gitstore.Hash
	fail string
}

// worlds lists each world in turn, one per poll, then the last one again.
func worlds(ws ...world) lister {
	n := -1
	return func(prefix string) (map[string]gitstore.Hash, error) {
		if prefix == oplog.RefName(gitstore.Kinds[0], "") && n < len(ws)-1 {
			n++ // a new poll
		}
		if prefix == ws[n].fail {
			return nil, errors.New("listing failed")
		}
		out := map[string]gitstore.Hash{}
		for name, h := range ws[n].refs {
			if strings.HasPrefix(name, prefix) {
				out[name] = h
			}
		}
		return out, nil
	}
}

// A failed poll is skipped whole: a failed baseline makes the next poll the
// baseline, and a later poll compares with the last that succeeded.
func TestWatchSkipsFailedPolls(t *testing.T) {
	t.Parallel()
	a, b, c := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)
	issue := func(id string) string { return oplog.RefName("issue", id) }
	h1, h2 := gitstore.Hash{1}, gitstore.Hash{2}
	now := map[string]gitstore.Hash{
		oplog.RefName("person", c): h1,
		issue(b):                   h1,
		issue(a):                   h2,
		issue("notes"):             h1, // not an id
	}
	list := worlds(
		world{fail: issue("")},
		world{refs: map[string]gitstore.Hash{issue(a): h1, oplog.RefName("sync", c): h1}},
		world{refs: now, fail: oplog.RefName("person", "")},
		world{refs: now},
	)
	ch := watch(t.Context(), list, time.Millisecond)
	want := []Change{{"issue", a, h2}, {"issue", b, h1}, {"person", c, h1}, {"sync", c, gitstore.ZeroHash}}
	if got := next(t, ch); !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
	quiet(t, ch)
}
