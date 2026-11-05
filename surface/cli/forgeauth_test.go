package cli

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/guilbep/git-board/app"
	"github.com/guilbep/git-board/forge"
)

// forge status and forge login, on a board whose forge is found in
// origin's URL, the forges' doors faked: no network, no credential
// helper. They chdir (-C), set the environment and swap app's doors, so
// they run serially.
func TestForgeTokens(t *testing.T) {
	dir, _ := newBoard(t)
	if out, err := exec.Command("git", "-C", dir, "remote", "add", "origin", "git@github.com:o/r.git").CombinedOutput(); err != nil {
		t.Fatalf("git remote: %v: %s", err, out)
	}
	t.Chdir(dir) // and back after: -C leaves the process there
	t.Setenv("GITHUB_TOKEN", "")
	kept := map[string]string{} // login -> token
	var stored []string
	find, owner, approve := app.FindToken, app.ForgeOwner, app.ApproveToken
	t.Cleanup(func() { app.FindToken, app.ForgeOwner, app.ApproveToken = find, owner, approve })
	app.FindToken = func(_ context.Context, _ forge.Table, login string) (string, error) { return kept[login], nil }
	app.ForgeOwner = func(_ context.Context, _ forge.Table, login string) (string, error) { return "ada", nil }
	app.ApproveToken = func(_ context.Context, baseURL, login, token string) error {
		stored = append(stored, baseURL+" "+login+" "+token)
		kept[login] = token
		return nil
	}
	stdin := func(s string) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		w.WriteString(s)
		w.Close()
		old := os.Stdin
		os.Stdin = r
		t.Cleanup(func() { os.Stdin = old; r.Close() })
	}

	for _, c := range []struct {
		args  []string
		stdin string
		code  int
		out   string
	}{
		{[]string{"forge", "status"}, "", 0, "github: github github.com/o/r (sync on, found in the git remotes)\n" +
			"  posting token for any login: none found: git board forge login github\n" +
			"  you token for ada: none found: git board forge login github --login ada\n"},
		{[]string{"forge", "login", "nope"}, "", 3, ""},
		{[]string{"forge", "login", "github", "--login", "bob"}, "\n", 2, ""}, // no token read
		{[]string{"forge", "login", "github", "--login", "bob"}, "  the-token \n", 0, "stored a token for bob on github.com in git's credential helper; git board forge status --check asks github.com whose it is\n"},
		// by default, for your own account on the host
		{[]string{"forge", "login", "github"}, "ada-token\n", 0, "stored a token for ada on github.com in git's credential helper; git board forge status --check asks github.com whose it is\n"},
	} {
		if c.stdin != "" {
			stdin(c.stdin)
		}
		out, errs, code := run(t, append([]string{"-C", dir}, c.args...)...)
		if code != c.code || (c.out != "" && out != c.out) {
			t.Errorf("%q: exit %d, want %d; stdout %q, want %q; stderr %q", c.args, code, c.code, out, c.out, errs)
		}
	}
	if len(stored) != 2 || stored[0] != "https://github.com bob the-token" || stored[1] != "https://github.com ada ada-token" {
		t.Errorf("stored: %q", stored)
	}
	// a token for any login, then --check asks whose it is
	kept[""] = "t"
	out, _, _ := run(t, "-C", dir, "forge", "status", "--check")
	if want := "  posting token for any login: found (git's credential helper), it is ada's\n  you token for ada: found (git's credential helper), it is ada's\n"; !strings.HasSuffix(out, want) {
		t.Errorf("status --check: %q, want it to end %q", out, want)
	}
	out, _, _ = run(t, "-C", dir, "forge", "status", "--json", "name,tokens", "--jq", ".[0].tokens[0].found")
	if out != "true\n" {
		t.Errorf("status --json: %q", out)
	}
}
