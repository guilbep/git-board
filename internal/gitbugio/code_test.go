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
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
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

	// no git directory: every reader answers empty
	m := gitbugiotest.New(t)
	files, err1 := m.ChangedFiles("dev", "fix/42-y")
	grep, err2 := m.GrepFiles("dev", "docs", "x")
	data, err3 := m.ReadFile("dev", "a")
	if files != nil || grep != nil || data != nil || err1 != nil || err2 != nil || err3 != nil || m.FirstCommitAdding("dev", "x", "a") != "" {
		t.Errorf("no git dir: %v %v %v %v %v %v", files, grep, data, err1, err2, err3)
	}
}
