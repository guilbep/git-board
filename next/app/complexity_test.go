package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/store"
	"github.com/guilbep/git-board/next/tools/boardgen"
	"github.com/guilbep/git-board/next/tools/code"
)

// The complexity guards (docs/guide/performance.md, v2): on a generated
// board of guardIssues issues, each read and write counts its work
// (store.Counts: listings, entities loaded, git objects, git processes,
// view calls and rows) and the test holds it to a limit far below the
// board's size. They count work, not time: the same on a laptop and on a
// shared CI runner. A quadratic or an N+1 that comes back fails them by
// thousands, not by a few.

// guardIssues is the guards' board's size: big enough that one read per
// issue is thousands over any limit.
const guardIssues = 2000

var guard struct {
	once sync.Once
	dir  string // the board as made, copied for each test
	b    *boardgen.Board
	err  error
}

func TestMain(m *testing.M) {
	code := m.Run()
	if guard.dir != "" {
		os.RemoveAll(filepath.Dir(guard.dir))
	}
	os.Exit(code)
}

// guardBoard is a copy of the guards' board, made once for the package's
// run (1 s), and opened as the person who writes.
func guardBoard(t *testing.T) (*Board, *boardgen.Board) {
	t.Helper()
	guard.once.Do(func() {
		tmp, err := os.MkdirTemp("", "git-board-guards-")
		if err != nil {
			guard.err = err
			return
		}
		guard.dir = filepath.Join(tmp, "board")
		guard.b, guard.err = boardgen.Git(context.Background(), guard.dir, boardgen.Options{Issues: guardIssues, Seed: 1})
	})
	if guard.err != nil {
		t.Fatal(guard.err)
	}
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(guard.dir)); err != nil {
		t.Fatal(err)
	}
	b, err := Open(t.Context(), dir, Options{Who: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	return b, guard.b
}

// did is what one call did.
type did struct {
	store.Counts
	gits atomic.Int64 // git processes the code's reads started (code.Repo)
}

func (w *did) String() string {
	return fmt.Sprintf("%v, code's git processes %d", w.Snapshot(), w.gits.Load())
}

// counting runs f with a context that counts, and answers what it did.
func counting(t *testing.T, f func(ctx context.Context) error) *did {
	t.Helper()
	w := &did{}
	ctx := code.Counting(store.WithCounts(t.Context(), &w.Counts), &w.gits)
	if err := f(ctx); err != nil {
		t.Fatal(err)
	}
	return w
}

// limits are a call's most of each counter; a counter not named has no
// limit.
type limits map[string]int64

func (w *did) within(t *testing.T, what string, l limits) {
	t.Helper()
	got := w.Snapshot()
	got["git processes"] = got["processes"] + w.gits.Load()
	for k, most := range l {
		v, ok := got[k]
		if !ok {
			t.Fatalf("%s: no counter %q", what, k)
		}
		if v > most {
			t.Errorf("%s: %s %d, at most %d on %d issues (%v)", what, k, v, most, guardIssues, w)
		}
	}
	if testing.Verbose() {
		t.Logf("%s: %v", what, w)
	}
}

func list(b *Board) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		_, err := b.Issues.List(ctx, issues.ListReq{Filter: issues.DefaultFilter})
		return err
	}
}

// A first ls builds the view: every entity loaded once, in a few loads,
// not one each; then ls reads no entity and no git object, and a filter
// on fields or free text neither.
func TestGuardLs(t *testing.T) {
	t.Parallel()
	b, gb := guardBoard(t)
	entities := int64(len(gb.Issues) + len(gb.People))
	counting(t, list(b)).within(t, "first ls", limits{"entities": entities, "loads": 8, "listings": 4, "git processes": 8, "view calls": 32})
	warm := limits{"entities": 0, "objects": 0, "loads": 0, "listings": 4, "git processes": 2, "view calls": 16, "rows put": 0}
	counting(t, list(b)).within(t, "ls", warm)
	for _, filter := range []string{"status:ready assignee:dev3", "is:closed", "assignee:@me"} {
		counting(t, func(ctx context.Context) error {
			_, err := b.Issues.List(ctx, issues.ListReq{Filter: filter})
			return err
		}).within(t, "ls "+filter, warm)
	}
	// free text: its index built once, by the first query that asks
	counting(t, func(ctx context.Context) error {
		_, err := b.Issues.List(ctx, issues.ListReq{Filter: "export crash"})
		return err
	}).within(t, "first free-text ls", limits{"entities": entities, "loads": 8, "git processes": 8, "view calls": 32})
	counting(t, func(ctx context.Context) error {
		_, err := b.Issues.List(ctx, issues.ListReq{Filter: "export crash"})
		return err
	}).within(t, "free-text ls", warm)
	counting(t, func(ctx context.Context) error {
		_, err := b.Issues.LabelsIn(ctx, issues.ListReq{Filter: issues.DefaultFilter})
		return err
	}).within(t, "facets", warm)
}

// show loads its issue alone; a write adds one pack, loads nothing and
// puts one row; a refresh after another process moved one issue loads
// and puts that one.
func TestGuardShowAndWrite(t *testing.T) {
	t.Parallel()
	b, gb := guardBoard(t)
	counting(t, list(b)) // the view built
	id := string(gb.Issues[7])
	counting(t, func(ctx context.Context) error {
		_, err := b.Issues.Show(ctx, id)
		return err
	}).within(t, "show", limits{"entities": 1, "loads": 1, "objects": 40, "listings": 4, "git processes": 4, "view calls": 16, "rows put": 0})
	counting(t, func(ctx context.Context) error {
		_, _, err := b.Issues.Comment(ctx, id, "a comment the guards write")
		return err
	}).within(t, "comment", limits{"adds": 1, "entities": 1, "loads": 1, "objects": 40, "listings": 4, "git processes": 6, "view calls": 16, "rows put": 1})
	counting(t, func(ctx context.Context) error {
		_, err := b.Issues.Set(ctx, issues.SetReq{Name: id, Assign: []string{"prio=p0"}})
		return err
	}).within(t, "set", limits{"adds": 1, "entities": 2, "loads": 2, "objects": 80, "listings": 4, "git processes": 6, "view calls": 16, "rows put": 1})

	// a pull brings one issue another clone moved
	clone := t.TempDir()
	if err := os.CopyFS(clone, os.DirFS(b.Dir)); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(filepath.Join(clone, ".git", "board")) // its own view
	other, err := Open(t.Context(), clone, Options{Who: true})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, _, err := other.Issues.Comment(t.Context(), string(gb.Issues[9]), "from another clone"); err != nil {
		t.Fatal(err)
	}
	gitT(t, b.Dir, "fetch", "-q", clone, "+refs/board/*:refs/board/*")
	counting(t, list(b)).within(t, "ls after a pull that moved one issue", limits{"entities": 1, "loads": 1, "listings": 4, "git processes": 4, "view calls": 16, "rows put": 1})
}

// The Work page reads every open issue's thread in a few loads, and its
// plan files and branches in a few git processes, not one each; again,
// nothing; after a comment, that issue's thread alone.
func TestGuardWork(t *testing.T) {
	t.Parallel()
	b, gb := guardBoard(t)
	counting(t, list(b))
	counting(t, func(ctx context.Context) error {
		_, err := b.Work(ctx, "", true)
		return err
	}).within(t, "first Work page", limits{"entities": int64(len(gb.Issues)), "loads": 4, "listings": 6, "git processes": 16, "view calls": 32})
	again := limits{"entities": 0, "objects": 0, "loads": 0, "listings": 6, "git processes": 8, "view calls": 16}
	counting(t, func(ctx context.Context) error {
		_, err := b.Work(ctx, "", true)
		return err
	}).within(t, "Work page", again)
	open, err := b.Issues.List(t.Context(), issues.ListReq{Filter: issues.DefaultFilter})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.Issues.Comment(t.Context(), open[3].ID, "Is it done?"); err != nil {
		t.Fatal(err)
	}
	counting(t, func(ctx context.Context) error {
		_, err := b.Work(ctx, "", true)
		return err
	}).within(t, "Work page after a comment", limits{"entities": 1, "loads": 1, "listings": 6, "git processes": 8, "view calls": 16})
}
