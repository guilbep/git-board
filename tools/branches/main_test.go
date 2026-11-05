package branches

import (
	"os/exec"
	"testing"

	"github.com/guilbep/git-board/hermetic"
)

// TestMain keeps the tests off the network and the user's tokens.
func TestMain(m *testing.M) { hermetic.Run(m) }

// repo is a new git repository, its path.
func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "init", "-q")
	return dir
}

// run runs git in dir.
func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
