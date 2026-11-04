package mdstore_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/datastore"
	"github.com/guilbep/git-board/core/mdstore"
)

// git runs git in dir, and answers its output.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir, "-c", "commit.gpgsign=false"}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// checkout is a git checkout on main, with one commit, its board in
// issues/.
func checkout(t *testing.T) (string, *mdstore.Store) {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "start")
	return dir, open(t, filepath.Join(dir, "issues"))
}

// The counter is .board/numbers, written on the default branch alone:
// another branch reads it, and its write says why it waits (issue #84).
func TestCounterOnTheDefaultBranchOnly(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	dir, s := checkout(t)
	var c datastore.Counter = s
	v, err := c.SetCount(ctx, []byte("prefix GB\nstart 1\n"), "")
	if err != nil {
		t.Fatalf("on main: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "issues", mdstore.Board, "numbers")); err != nil || string(data) != "prefix GB\nstart 1\n" {
		t.Fatalf("numbers = %q, %v", data, err)
	}
	if err := s.CountWaits(ctx); err != nil {
		t.Errorf("on main, CountWaits = %v", err)
	}

	git(t, dir, "switch", "-q", "-c", "fix/x")
	if got, ver, err := c.Count(ctx); string(got) != "prefix GB\nstart 1\n" || ver != v || err != nil {
		t.Errorf("on fix/x, Count = %q, %q, %v: it reads", got, ver, err)
	}
	_, err = c.SetCount(ctx, []byte("prefix GB\nstart 1\n1 x\n"), v)
	if !errors.Is(err, datastore.ErrMissing) || !strings.Contains(err.Error(), "main") || !strings.Contains(err.Error(), "fix/x") {
		t.Fatalf("on fix/x, SetCount = %v; want ErrMissing naming main and fix/x", err)
	}
	if werr := s.CountWaits(ctx); werr == nil || werr.Error() != err.Error() {
		t.Errorf("CountWaits = %v, want %v", werr, err)
	}
	git(t, dir, "switch", "-q", "--detach")
	if _, err := c.SetCount(ctx, []byte("x"), v); !errors.Is(err, datastore.ErrMissing) {
		t.Errorf("detached: %v, want ErrMissing", err)
	}

	// origin's HEAD names the default branch, over main
	git(t, dir, "switch", "-q", "main")
	git(t, dir, "update-ref", "refs/remotes/origin/trunk", "HEAD")
	git(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")
	if _, err := c.SetCount(ctx, []byte("x"), v); !errors.Is(err, datastore.ErrMissing) || !strings.Contains(err.Error(), "trunk") {
		t.Errorf("on main, origin's HEAD trunk: %v, want ErrMissing naming trunk", err)
	}
	git(t, dir, "switch", "-q", "-c", "trunk")
	if _, err := c.SetCount(ctx, []byte("prefix GB\nstart 1\n1 x\n"), v); err != nil {
		t.Errorf("on trunk: %v", err)
	}
}

// master is the default branch when there is no main and origin names
// none; a folder outside any git checkout has no branch to wait for.
func TestCounterDefaultBranchFallbacks(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "master")
	s := open(t, filepath.Join(dir, "issues"))
	if _, err := s.SetCount(ctx, []byte("a"), ""); err != nil { // unborn master
		t.Fatalf("on an unborn master: %v", err)
	}
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "start")
	git(t, dir, "switch", "-q", "-c", "main-ish")
	if _, err := s.SetCount(ctx, []byte("b"), ""); !errors.Is(err, datastore.ErrMissing) || !strings.Contains(err.Error(), "master") {
		t.Errorf("on main-ish: %v, want ErrMissing naming master", err)
	}

	loose := open(t, t.TempDir())
	if _, err := loose.SetCount(ctx, []byte("a"), ""); err != nil {
		t.Errorf("outside a checkout: %v", err)
	}
}

// The counter file never merges line by line: .board/.gitattributes
// gives it git's binary driver, so a merge where both sides numbered
// keeps ours whole and says it conflicts, where union would keep one
// number twice and a text merge would write conflict markers in it.
func TestCounterIsNotUnionMerged(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	dir, s := checkout(t)
	v, err := s.SetCount(ctx, []byte("prefix GB\nstart 1\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	attrs, err := os.ReadFile(filepath.Join(dir, "issues", mdstore.Board, ".gitattributes"))
	if err != nil || !strings.Contains(string(attrs), "/numbers merge=binary\n") || !strings.Contains(string(attrs), "*.jsonl merge=union\n") {
		t.Fatalf(".gitattributes = %q, %v", attrs, err)
	}
	if _, err := s.SetCount(ctx, []byte("prefix GB\nstart 1\n"), v); err != nil { // a second write adds no second line
		t.Fatal(err)
	}
	if again, _ := os.ReadFile(filepath.Join(dir, "issues", mdstore.Board, ".gitattributes")); string(again) != string(attrs) {
		t.Fatalf(".gitattributes grew: %q", again)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "numbers")

	// a clone numbered 1 on main meanwhile: its main is "theirs"
	git(t, dir, "switch", "-q", "-c", "theirs")
	numbers := filepath.Join(dir, "issues", mdstore.Board, "numbers")
	if err := os.WriteFile(numbers, []byte("prefix GB\nstart 1\n1 theirs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "commit", "-q", "-am", "theirs")
	git(t, dir, "switch", "-q", "main")
	_, ver, _ := s.Count(ctx)
	if _, err := s.SetCount(ctx, []byte("prefix GB\nstart 1\n1 ours\n"), ver); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "commit", "-q", "-am", "ours")
	if out, err := exec.Command("git", "-C", dir, "merge", "-q", "theirs").CombinedOutput(); err == nil {
		t.Fatalf("both sides numbered 1, and the merge went through:\n%s", out)
	}
	if data, _ := os.ReadFile(numbers); string(data) != "prefix GB\nstart 1\n1 ours\n" {
		t.Errorf("after the merge, numbers = %q: ours, whole", data)
	}
}
