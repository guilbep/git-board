package gitbugio_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/gitbugio"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=dev", "-c", "user.email=dev@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func commitFile(t *testing.T, dir, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, path)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, path), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", path)
	git(t, dir, "commit", "-m", "add "+path)
	return git(t, dir, "rev-parse", "HEAD")
}

func TestGitReaders(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	git(t, dir, "init", "-b", "dev")
	commitFile(t, dir, "docs/plans/42-x.md", "---\nissue: 42\nstatus: draft\n---\n# Plan\n")
	git(t, dir, "checkout", "-b", "fix/42-y")
	first := commitFile(t, dir, "a_test.go", "package a\n")
	commitFile(t, dir, "b.go", "package a\n")

	b, err := gitbugio.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })

	if got, err := b.ChangedFiles("dev", "fix/42-y"); err != nil || !slices.Equal(got, []string{"a_test.go", "b.go"}) {
		t.Errorf("ChangedFiles = %v, %v", got, err)
	}
	if got := b.FirstCommitAdding("dev", "fix/42-y", "a_test.go"); got != first {
		t.Errorf("FirstCommitAdding = %q, want %q", got, first)
	}
	if got := b.FirstCommitAdding("dev", "fix/42-y", "nope.go"); got != "" {
		t.Errorf("FirstCommitAdding(nope.go) = %q", got)
	}
	if got, err := b.GrepFiles("dev", "docs/plans", "^issue: 42$"); err != nil || !slices.Equal(got, []string{"docs/plans/42-x.md"}) {
		t.Errorf("GrepFiles(42) = %v, %v", got, err)
	}
	if got, err := b.GrepFiles("dev", "docs/plans", "^issue: 7$"); err != nil || len(got) != 0 {
		t.Errorf("GrepFiles(7) = %v, %v", got, err)
	}
	if got, err := b.GrepFiles("dev", "no/such/dir", "^issue: 42$"); err != nil || len(got) != 0 {
		t.Errorf("GrepFiles(no dir) = %v, %v", got, err)
	}
	if got, err := b.ReadFile("dev", "docs/plans/42-x.md"); err != nil || !bytes.HasPrefix(got, []byte("---")) {
		t.Errorf("ReadFile = %q, %v", got, err)
	}
}

// An SSH-signed commit (gpg.format ssh) carries a gpgsig header git-bug's
// ReadCommit decodes as OpenPGP armor, which fails with EOF: the branch's
// ancestry, and the Development section and the cycle with it, failed on a
// repository whose commits are signed with SSH.
func TestAncestorsOfAnSSHSignedCommit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	git(t, dir, "init", "-b", "dev")
	parent := commitFile(t, dir, "a.go", "package a\n")
	tree := git(t, dir, "rev-parse", "HEAD^{tree}")
	obj := "tree " + tree + "\nparent " + parent + "\n" +
		"author dev <dev@example.com> 1700000000 +0000\n" +
		"committer dev <dev@example.com> 1700000000 +0000\n" +
		"gpgsig -----BEGIN SSH SIGNATURE-----\n U1NIU0lHAAAAAQ==\n -----END SSH SIGNATURE-----\n" +
		"\nsigned with ssh\n"
	cmd := exec.Command("git", "-C", dir, "hash-object", "-t", "commit", "-w", "--stdin")
	cmd.Stdin = strings.NewReader(obj)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	signed := strings.TrimSpace(string(out))
	git(t, dir, "update-ref", "refs/heads/dev", signed)

	b, err := gitbugio.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })

	got, err := b.Ancestors(signed)
	if err != nil || !got[signed] || !got[parent] || len(got) != 2 {
		t.Errorf("Ancestors = %v, %v; want the signed commit and its parent", got, err)
	}
}

func TestWorktrees(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	git(t, dir, "init", "-b", "dev")
	commitFile(t, dir, "a.txt", "a\n")
	b, err := gitbugio.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if got, err := b.Worktrees(); err != nil || len(got) != 0 {
		t.Fatalf("no linked worktree: %v, %v", got, err)
	}
	wt, d := filepath.Join(t.TempDir(), "wt"), filepath.Join(t.TempDir(), "d")
	git(t, dir, "worktree", "add", "-b", "fix/1-x", wt)
	git(t, dir, "worktree", "add", "--detach", d)
	head := git(t, dir, "rev-parse", "HEAD")
	got, err := b.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	want := []gitbugio.Worktree{{Path: wt, Head: head, Branch: "fix/1-x"}, {Path: d, Head: head}}
	for i := range got {
		got[i].Path, _ = filepath.EvalSymlinks(got[i].Path)
	}
	for i := range want {
		want[i].Path, _ = filepath.EvalSymlinks(want[i].Path)
	}
	if !slices.Equal(got, want) {
		t.Errorf("Worktrees = %+v, want %+v", got, want)
	}
}
