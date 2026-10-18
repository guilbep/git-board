package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/git-bug/git-bug/cache"
	"github.com/git-bug/git-bug/repository"
	"github.com/git-bug/gitconfig"

	"github.com/guilbep/git-board/internal/server"
)

const testConfig = `
[fields.status]
single = true
`

// newTestRepo initializes a real, on-disk git-bug repo with one open bug
// ("Fix invoice bug", status:ready) and one closed bug ("Write specs"),
// chdirs the test into it, and writes a minimal .git-board.toml. This
// exercises the exact repo-discovery path (repository.OpenDefault) the
// CLI uses in production, not a mock.
func newTestRepo(t *testing.T) {
	t.Helper()
	dir := t.TempDir()

	repo, err := repository.InitDefault(dir, "git-bug")
	if err != nil {
		t.Fatalf("InitGoGitRepo: %v", err)
	}
	if err := repo.Config().Update(func(f *gitconfig.File) error {
		return errors.Join(f.Set("user.name", "guilbep"), f.Set("user.email", "guilbep@example.com"))
	}); err != nil {
		t.Fatalf("set user: %v", err)
	}

	c, err := cache.NewRepoCacheNoEvents(repo, nil)
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

func TestLsListsOnlyOpenIssues(t *testing.T) {
	newTestRepo(t)

	out, err := execute(t, []string{"ls"})
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

// startBoard serves the test repository, as "git board serve" would, until
// the test ends; it answers the board's URL.
func startBoard(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	urls, done := make(chan string, 1), make(chan error, 1)
	go func() { done <- server.Serve(ctx, "127.0.0.1:0", "", func(url string) { urls <- url }) }()
	t.Cleanup(func() { cancel(); <-done })
	select {
	case url := <-urls:
		return url
	case err := <-done:
		t.Fatalf("serve: %v", err)
		return ""
	}
}

// opened records what the browser would open, instead of opening it.
func opened(t *testing.T) *[]string {
	t.Helper()
	var urls []string
	was := openURL
	openURL = func(url string) error { urls = append(urls, url); return nil }
	t.Cleanup(func() { openURL = was })
	return &urls
}

// A bare git board opens this repository's board where it already runs,
// without starting another.
func TestBareOpensThisRepositorysRunningBoard(t *testing.T) {
	newTestRepo(t)
	url := startBoard(t)
	urls := opened(t)

	out, err := execute(t, nil)
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, url) || len(*urls) != 1 || (*urls)[0] != url {
		t.Errorf("opened %v, printed:\n%s\nwant %s", *urls, out, url)
	}

	if out, err := execute(t, []string{"--no-open"}); err != nil || !strings.Contains(out, url) {
		t.Errorf("--no-open: %v, printed:\n%s", err, out)
	}
	if len(*urls) != 1 {
		t.Errorf("--no-open opened %v", (*urls)[1:])
	}
}

// Listing moved to ls: a bare git board takes no filter.
func TestBareTakesNoArguments(t *testing.T) {
	newTestRepo(t)
	opened(t)
	if out, err := execute(t, []string{"is:open"}); err == nil {
		t.Errorf("git board is:open ran:\n%s", out)
	}
}
