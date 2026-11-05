package cli

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Two clones of one origin sync: the first numbers through origin's
// counter, the second fetches, pulls and reads the number. They chdir
// (-C), so they run serially.
func TestRemoteCommands(t *testing.T) {
	src, _ := newBoard(t)
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", origin).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if out, err := exec.Command("git", "-C", src, "remote", "add", "origin", origin).CombinedOutput(); err != nil {
		t.Fatalf("git remote: %v: %s", err, out)
	}
	other := filepath.Join(root, "other")
	if out, err := exec.Command("git", "clone", "-q", origin, other).CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v: %s", err, out)
	}
	for _, c := range []struct {
		args []string
		code int
		has  string
	}{
		{[]string{"-C", src, "push", "nowhere"}, 1, ""},
		{[]string{"-C", src, "numbers", "init", "--prefix", "GB", "--start", "7"}, 0, "numbering as GB-N from 7, on origin"},
		{[]string{"-C", src, "sync", "--git"}, 0, ".: git pull from origin...\n.: git push to origin...\n.: git push to origin: numbered 2 issues\n.: synced\n"},
		{[]string{"-C", src, "ls", "", "--jq", ".[].number", "--json", "number"}, 0, "GB-7\nGB-8\n"},
		{[]string{"-C", other, "fetch"}, 0, "origin: issues 3 to receive, 0 to send; people 1 to receive, 0 to send\n"},
		{[]string{"-C", other, "pull"}, 0, "pulled from origin\n"},
		{[]string{"-C", other, "ls", "Export"}, 0, "GB-8 Export crashes"},
		{[]string{"-C", other, "push"}, 0, "pushed to origin\n"},
		{[]string{"-C", other, "fetch"}, 0, "issues 0 to receive, 0 to send"},
	} {
		out, errs, code := run(t, c.args...)
		if code != c.code || !strings.Contains(out, c.has) {
			t.Errorf("%q: exit %d, want %d; stdout %q, want %q; stderr %q", c.args, code, c.code, out, c.has, errs)
		}
	}
}
