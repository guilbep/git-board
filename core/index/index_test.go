package index

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func openStore(t *testing.T, dir string) *gitstore.Store {
	t.Helper()
	s, err := gitstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newRepo(t *testing.T) (string, *gitstore.Store) {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	return dir, openStore(t, dir)
}

func openIndex(t *testing.T, s *gitstore.Store) *Index {
	t.Helper()
	ix, err := Open(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ix.Close() })
	return ix
}

func at(op oplog.Op, t int64) oplog.Op {
	op.Time = t
	return op
}

func mkIssue(t *testing.T, s *gitstore.Store, ops ...oplog.Op) *issue.Issue {
	t.Helper()
	is, err := issue.Create(t.Context(), s, ops...)
	if err != nil {
		t.Fatal(err)
	}
	return is
}

// ids answers q and gives the ids, in order.
func ids(t *testing.T, ix *Index, q Query) []string {
	t.Helper()
	got, err := ix.Issues(t.Context(), q)
	if err != nil {
		t.Fatalf("issues %+v: %v", q, err)
	}
	out := []string{}
	for _, s := range got {
		out = append(out, s.ID)
	}
	return out
}

func sorted(ids ...string) []string {
	slices.Sort(ids)
	return ids
}

func TestIndexSeesOtherWrites(t *testing.T) {
	t.Parallel()
	dir, s := newRepo(t)
	ix := openIndex(t, s)
	other := openStore(t, dir)
	is := mkIssue(t, other, issue.NewCreate("ann", "First", ""))
	got, err := ix.Issues(t.Context(), Query{})
	if err != nil || len(got) != 1 || got[0].ID != is.ID || got[0].Title != "First" {
		t.Fatalf("issues = %+v, %v", got, err)
	}
	if _, err := issue.Do(t.Context(), other, is.ID, issue.SetTitle("ann", "Renamed")); err != nil {
		t.Fatal(err)
	}
	got, err = ix.Issues(t.Context(), Query{})
	if err != nil || len(got) != 1 || got[0].Title != "Renamed" {
		t.Fatalf("after set-title: %+v, %v", got, err)
	}
	gitRun(t, dir, "update-ref", "-d", oplog.RefName("issue", is.ID))
	if got := ids(t, ix, Query{}); len(got) != 0 {
		t.Fatalf("after its ref is gone: %v", got)
	}
}

func TestIndexRebuilds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		spoil func(t *testing.T, path string)
	}{
		{"missing", func(t *testing.T, path string) {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}},
		{"garbage", func(t *testing.T, path string) {
			if err := os.WriteFile(path, bytes.Repeat([]byte("garbage!"), 512), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"version", func(t *testing.T, path string) {
			// Without a rebuild, the stored heads would hide the lost rows.
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.Exec("DELETE FROM issues; PRAGMA user_version = 99"); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, s := newRepo(t)
			x := mkIssue(t, s, issue.NewCreate("ann", "X", ""))
			ix, err := Open(t.Context(), s)
			if err != nil {
				t.Fatal(err)
			}
			if got := ids(t, ix, Query{}); !slices.Equal(got, []string{x.ID}) {
				t.Fatalf("before: %v", got)
			}
			if err := ix.Close(); err != nil {
				t.Fatal(err)
			}
			tc.spoil(t, filepath.Join(s.CommonDir(), "board", "index.db"))
			y := mkIssue(t, s, issue.NewCreate("ann", "Y", ""))
			ix = openIndex(t, s)
			if got := sorted(ids(t, ix, Query{})...); !slices.Equal(got, sorted(x.ID, y.ID)) {
				t.Fatalf("after: %v, want %v and %v", got, x.ID, y.ID)
			}
			var v int
			if err := ix.db.QueryRow("PRAGMA user_version").Scan(&v); err != nil || v != 1 {
				t.Fatalf("user_version = %d, %v", v, err)
			}
		})
	}
}

func TestIndexTwoProcesses(t *testing.T) {
	t.Parallel()
	dir, s := newRepo(t)
	ixs := []*Index{openIndex(t, s), openIndex(t, openStore(t, dir))}
	var wg sync.WaitGroup
	for i := range 4 {
		ix := ixs[i%2]
		wg.Go(func() {
			for range 20 {
				if err := ix.Refresh(t.Context()); err != nil {
					t.Error(err)
					return
				}
				if _, err := ix.Issues(t.Context(), Query{Text: "bug"}); err != nil {
					t.Error(err)
					return
				}
				if _, _, err := ix.ForAccount(t.Context(), "github.com", "1"); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	wg.Go(func() {
		for i := range 10 {
			if _, err := issue.Create(t.Context(), s, issue.NewCreate("ann", fmt.Sprintf("bug %d", i), "")); err != nil {
				t.Error(err)
				return
			}
		}
	})
	wg.Wait()
	for _, ix := range ixs {
		if got := ids(t, ix, Query{Text: "bug"}); len(got) != 10 {
			t.Fatalf("issues = %v, want 10", got)
		}
	}
}

func TestIndexSkipsBroken(t *testing.T) {
	t.Parallel()
	dir, s := newRepo(t)
	good := mkIssue(t, s, issue.NewCreate("ann", "Good", ""))
	bad, err := oplog.Create(t.Context(), s, "issue", "", []oplog.Op{{Kind: "bogus", Author: "ann", Time: 1}})
	if err != nil {
		t.Fatal(err)
	}
	// A ref whose name isn't an id isn't an entity.
	gitRun(t, dir, "update-ref", "refs/board/issue/not-an-id", bad.Head.String())
	ix := openIndex(t, s)
	if got := ids(t, ix, Query{}); !slices.Equal(got, []string{good.ID}) {
		t.Fatalf("issues = %v, want %v", got, good.ID)
	}
	br, err := ix.Broken(t.Context())
	if err != nil || len(br) != 1 || br[0].Kind != "issue" || br[0].ID != bad.ID || !strings.Contains(br[0].Error, "unknown operation") {
		t.Fatalf("broken = %+v, %v", br, err)
	}
	fix := mkIssue(t, s, issue.NewCreate("ann", "Fixed", ""))
	head, err := s.Ref(oplog.RefName("issue", fix.ID))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateRef(t.Context(), oplog.RefName("issue", bad.ID), head, bad.Head); err != nil {
		t.Fatal(err)
	}
	if br, err := ix.Broken(t.Context()); err != nil || len(br) != 0 {
		t.Fatalf("broken after the fix = %+v, %v", br, err)
	}
	if got := sorted(ids(t, ix, Query{Text: "fixed"})...); !slices.Equal(got, sorted(bad.ID, fix.ID)) {
		t.Fatalf("after the fix: %v, want %v and %v", got, bad.ID, fix.ID)
	}
}

func TestIndexPathIsEscaped(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "C# 100%?x")
	if err := os.Mkdir(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "init", "-q")
	s := openStore(t, dir)
	x := mkIssue(t, s, issue.NewCreate("ann", "X", ""))
	if got := ids(t, openIndex(t, s), Query{}); !slices.Equal(got, []string{x.ID}) {
		t.Fatalf("issues = %v", got)
	}
	if _, err := os.Stat(filepath.Join(s.CommonDir(), "board", "index.db")); err != nil {
		t.Fatal(err)
	}
}
