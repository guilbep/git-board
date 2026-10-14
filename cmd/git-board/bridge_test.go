package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/git-bug/git-bug/cache"
	"github.com/git-bug/git-bug/repository"
	"github.com/git-bug/gitconfig"

	"github.com/guilbep/git-board/internal/ghbridge"
)

// newBridgeTestRepo mirrors newTestRepo but seeds a bug that was
// bridged in from GitHub (github-url metadata set), matching the fixture
// item at internal/ghbridge/testdata/project-items.json.
func newBridgeTestRepo(t *testing.T) string {
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

	if _, _, err := c.Bugs().NewRaw(id, 1735689600, "cannot change equipment_class", "", nil,
		map[string]string{"github-url": "https://github.com/EPFL-ENAC/co2-calculator/issues/2605"}); err != nil {
		t.Fatalf("NewRaw: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("close cache: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, configPath), []byte(testConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Chdir(dir)
	return dir
}

// fixtureItemsPath is resolved to an absolute path at package init, before
// any test's t.Chdir into a throwaway repo dir makes the relative path
// meaningless.
var fixtureItemsPath = mustAbs("../../internal/ghbridge/testdata/project-items.json")

func mustAbs(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		panic(err)
	}
	return abs
}

func withFixtureFetch(t *testing.T) {
	t.Helper()
	data, err := os.ReadFile(fixtureItemsPath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	original := fetchGithubItems
	fetchGithubItems = func(owner string, project, limit int) ([]ghbridge.Item, error) {
		return ghbridge.ParseItems(data)
	}
	t.Cleanup(func() { fetchGithubItems = original })
}

func TestBridgeConfigureWritesConfig(t *testing.T) {
	newBridgeTestRepo(t)

	out, err := execute(t, []string{"bridge", "configure", "--owner", "EPFL-ENAC", "--project", "40"})
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read %s: %v", configPath, err)
	}
	if !strings.Contains(string(data), `owner = "EPFL-ENAC"`) || !strings.Contains(string(data), "number = 40") {
		t.Errorf("config missing [github-project] block:\n%s", data)
	}
	// The rest of the seeded config must survive untouched.
	if !strings.Contains(string(data), "[fields.status]") {
		t.Errorf("config lost existing [fields.status]:\n%s", data)
	}
}

func TestBridgePullDryRunWritesNothing(t *testing.T) {
	newBridgeTestRepo(t)
	withFixtureFetch(t)

	out, err := execute(t, []string{"bridge", "pull", "--owner", "EPFL-ENAC", "--project", "40"})
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "dry run") {
		t.Errorf("expected dry-run notice in output:\n%s", out)
	}
	if !strings.Contains(out, "#2605") {
		t.Errorf("expected #2605's planned change in output:\n%s", out)
	}

	// Confirm nothing was actually written: `get` on the bug shows no
	// bridged-in labels yet.
	getOut, err := execute(t, []string{"ls", "is:open", "--json"})
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	if strings.Contains(getOut, "prio:p0") {
		t.Errorf("dry-run must not write labels, but found prio:p0:\n%s", getOut)
	}
}

func TestBridgePullApplyWritesLabelsAndIsIdempotent(t *testing.T) {
	newBridgeTestRepo(t)
	withFixtureFetch(t)

	out, err := execute(t, []string{"bridge", "pull", "--owner", "EPFL-ENAC", "--project", "40", "--apply"})
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "applied.") {
		t.Errorf("expected apply confirmation in output:\n%s", out)
	}

	lsOut, err := execute(t, []string{"ls", "is:open"})
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	if !strings.Contains(lsOut, "prio:p0") {
		t.Errorf("expected prio:p0 written after --apply:\n%s", lsOut)
	}

	// Re-running must not error (git-bug's ChangeLabels rejects an all-no-op
	// call) and must leave the same labels in place. The summary must also
	// report 0 fields set the second time — the fixture bug still carries a
	// status warning (persists every run since it's never written), which
	// must not be miscounted as a "field set" once its diff is empty.
	rerunOut, err := execute(t, []string{"bridge", "pull", "--owner", "EPFL-ENAC", "--project", "40", "--apply"})
	if err != nil {
		t.Fatalf("second (idempotent) execute: %v\noutput:\n%s", err, rerunOut)
	}
	if !strings.Contains(rerunOut, "0 field(s) set") {
		t.Errorf("expected 0 field(s) set on idempotent re-run, got:\n%s", rerunOut)
	}
	lsOut2, err := execute(t, []string{"ls", "is:open"})
	if err != nil {
		t.Fatalf("ls after re-run: %v", err)
	}
	if lsOut != lsOut2 {
		t.Errorf("re-running bridge pull --apply changed output:\nfirst:\n%s\nsecond:\n%s", lsOut, lsOut2)
	}
}

func TestBridgePullRejectsApplyAndDryRunTogether(t *testing.T) {
	newBridgeTestRepo(t)
	withFixtureFetch(t)

	_, err := execute(t, []string{"bridge", "pull", "--owner", "EPFL-ENAC", "--project", "40", "--apply", "--dry-run"})
	if err == nil {
		t.Fatal("expected error when --apply and --dry-run are both set")
	}
}

func TestBridgePullRequiresProjectConfigured(t *testing.T) {
	newBridgeTestRepo(t)
	withFixtureFetch(t)

	_, err := execute(t, []string{"bridge", "pull"})
	if err == nil {
		t.Fatal("expected error when no --owner/--project and no [github-project] configured")
	}
}
