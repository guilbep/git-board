package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/git-bug/git-bug/cache"
	"github.com/git-bug/git-bug/repository"
)

const testConfig = `
[fields.status]
single = true
`

// newTestRepo initializes a real, on-disk git-bug repo with one open bug
// ("Fix invoice bug", status:ready) and one closed bug ("Write specs"),
// chdirs the test into it, and writes a minimal .git-board.toml. This
// exercises the exact repo-discovery path (repository.OpenGoGitRepo) the
// CLI uses in production, not a mock.
func newTestRepo(t *testing.T) {
	t.Helper()
	dir := t.TempDir()

	repo, err := repository.InitGoGitRepo(dir, "git-bug")
	if err != nil {
		t.Fatalf("InitGoGitRepo: %v", err)
	}
	if err := repo.LocalConfig().StoreString("user.name", "guilbep"); err != nil {
		t.Fatalf("set user.name: %v", err)
	}
	if err := repo.LocalConfig().StoreString("user.email", "guilbep@example.com"); err != nil {
		t.Fatalf("set user.email: %v", err)
	}

	c, err := cache.NewRepoCacheNoEvents(repo)
	if err != nil {
		t.Fatalf("NewRepoCacheNoEvents: %v", err)
	}
	id, err := c.Identities().NewFromGitUser()
	if err != nil {
		t.Fatalf("NewFromGitUser: %v", err)
	}
	if err := c.SetUserIdentity(id); err != nil {
		t.Fatalf("SetUserIdentity: %v", err)
	}

	open, _, err := c.Bugs().New("Fix invoice bug", "")
	if err != nil {
		t.Fatalf("new open bug: %v", err)
	}
	if _, _, err := open.ChangeLabels([]string{"status:ready"}, nil); err != nil {
		t.Fatalf("label open bug: %v", err)
	}
	if err := open.Commit(); err != nil {
		t.Fatalf("commit open bug: %v", err)
	}

	closed, _, err := c.Bugs().New("Write specs", "")
	if err != nil {
		t.Fatalf("new closed bug: %v", err)
	}
	if _, err := closed.Close(); err != nil {
		t.Fatalf("close bug: %v", err)
	}
	if err := closed.Commit(); err != nil {
		t.Fatalf("commit closed bug: %v", err)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("close cache: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, configPath), []byte(testConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Chdir(dir)
}

func execute(t *testing.T, args []string) (string, error) {
	t.Helper()
	cmd := newRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestDefaultNoSubcommandListsOnlyOpenIssues(t *testing.T) {
	newTestRepo(t)

	out, err := execute(t, nil)
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "Fix invoice bug") {
		t.Errorf("expected open issue in output:\n%s", out)
	}
	if strings.Contains(out, "Write specs") {
		t.Errorf("expected closed issue to be excluded from default view:\n%s", out)
	}
}

func TestLsWithFilter(t *testing.T) {
	newTestRepo(t)

	out, err := execute(t, []string{"ls", "status:ready"})
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "Fix invoice bug") {
		t.Errorf("expected status:ready issue in output:\n%s", out)
	}
}

func TestSetAndGet(t *testing.T) {
	newTestRepo(t)

	out, err := execute(t, []string{"ls", "is:open", "--json"})
	if err != nil {
		t.Fatalf("execute ls: %v\noutput:\n%s", err, out)
	}
	// Grab the open bug's id out of the JSON so `set`/`get` can target it.
	idx := strings.Index(out, `"id": "`)
	if idx < 0 {
		t.Fatalf("no id found in JSON output:\n%s", out)
	}
	rest := out[idx+len(`"id": "`):]
	id := rest[:strings.Index(rest, `"`)]

	if _, err := execute(t, []string{"set", id, "prio=p1"}); err != nil {
		t.Fatalf("set: %v", err)
	}

	got, err := execute(t, []string{"get", id})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !strings.Contains(got, "prio:p1") {
		t.Errorf("expected prio:p1 in `get` output:\n%s", got)
	}
}
