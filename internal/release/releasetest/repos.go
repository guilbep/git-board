// Package releasetest builds local git repositories for tests: no network, no forge.
package releasetest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Git runs git in dir with a fixed identity and no user or system config.
func Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=dev", "GIT_AUTHOR_EMAIL=dev@example.com",
		"GIT_COMMITTER_NAME=dev", "GIT_COMMITTER_EMAIL=dev@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// NewRemote returns a bare repository and a clone of it with one commit on dev, already pushed.
func NewRemote(t *testing.T) (work, bare string) {
	t.Helper()
	root := t.TempDir()
	bare = filepath.Join(root, "bare.git")
	work = filepath.Join(root, "work")
	Git(t, root, "init", "--bare", "-b", "dev", bare)
	Git(t, root, "clone", bare, work)
	Git(t, work, "checkout", "-B", "dev")
	Commit(t, work, "first")
	Git(t, work, "push", "origin", "dev")
	return work, bare
}

// Commit makes an empty commit and returns its full sha.
func Commit(t *testing.T, dir, msg string) string {
	t.Helper()
	Git(t, dir, "commit", "--allow-empty", "-m", msg)
	return Git(t, dir, "rev-parse", "HEAD")
}

// NewConfigRepo returns a bare repository whose default branch is main, holding files.
func NewConfigRepo(t *testing.T, files map[string]string) (bare string) {
	t.Helper()
	root := t.TempDir()
	bare = filepath.Join(root, "config.git")
	work := filepath.Join(root, "work")
	Git(t, root, "init", "--bare", "-b", "main", bare)
	Git(t, root, "clone", bare, work)
	Git(t, work, "checkout", "-B", "main")
	for p, c := range files {
		full := filepath.Join(work, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	Git(t, work, "add", "-A")
	Git(t, work, "commit", "-m", "init")
	Git(t, work, "push", "origin", "main")
	return bare
}
