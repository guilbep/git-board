// Package gitbugiotest builds a real, throwaway git-bug repo cache for
// tests — via git-bug's own repository.CreateGoGitTestRepo test helper —
// instead of a hand-rolled fake. It's fast (single-digit milliseconds), so
// internal/gitbugio and internal/fields tests exercise the real
// cache.RepoCache API. Split into its own package (rather than a
// gitbugio_test.go helper) so other packages' tests can use it too.
package gitbugiotest

import (
	"context"
	"testing"
	"time"

	"github.com/git-bug/git-bug/bridge/core"
	"github.com/git-bug/git-bug/bridge/core/synctest"
	"github.com/git-bug/git-bug/cache"
	"github.com/git-bug/git-bug/repository/repotest"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// New spins up a repo cache in a t.TempDir() with a user identity
// configured, ready to create/query bugs.
func New(t testing.TB) *gitbugio.Backend {
	t.Helper()
	return backend(t, repotest.NewGoGitRepo(t, false))
}

// NewPair is New twice, two clones of one bare remote, origin: you and a
// colleague sharing a server.
func NewPair(t testing.TB) (mine, theirs *gitbugio.Backend) {
	t.Helper()
	remote := repotest.NewGoGitRepo(t, true)
	a, b := repotest.NewGoGitRepo(t, false), repotest.NewGoGitRepo(t, false)
	repotest.AddRemote(t, a, "origin", remote.GitDir)
	repotest.AddRemote(t, b, "origin", remote.GitDir)
	return backend(t, a), backend(t, b)
}

func backend(t testing.TB, repo *repotest.GoGitRepo) *gitbugio.Backend {
	t.Helper()
	b, err := gitbugio.NewForRepo(repo)
	if err != nil {
		t.Fatalf("NewForRepo: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })

	id, err := b.TestCache().Identities().NewFromGitUser()
	if err != nil {
		t.Fatalf("NewFromGitUser: %v", err)
	}
	if err := b.TestCache().SetUserIdentity(id); err != nil {
		t.Fatalf("SetUserIdentity: %v", err)
	}

	return b
}

// NewIssue creates a bug with the given title and returns its id.
func NewIssue(t testing.TB, b *gitbugio.Backend, title string) string {
	t.Helper()
	bug, _, err := b.TestCache().Bugs().New(title, "")
	if err != nil {
		t.Fatalf("Bugs().New: %v", err)
	}
	return bug.Id().String()
}

// NewImportedIssue creates a bug the same way NewIssue does, but stamps a
// "github-url" Create-metadata value on it — simulating a bug that came in
// via `git-bug bridge pull` from GitHub, so internal/ghbridge's
// issue-matching-by-URL has something real to match against.
func NewImportedIssue(t testing.TB, b *gitbugio.Backend, title, githubURL string) string {
	t.Helper()
	author, err := b.TestCache().GetUserIdentity()
	if err != nil {
		t.Fatalf("GetUserIdentity: %v", err)
	}
	bug, _, err := b.TestCache().Bugs().NewRaw(author, time.Now().Unix(), title, "", nil,
		map[string]string{"github-url": githubURL})
	if err != nil {
		t.Fatalf("Bugs().NewRaw: %v", err)
	}
	return bug.Id().String()
}

// CloseIssue closes the given bug (by id prefix) so tests can exercise
// status:closed filtering.
func CloseIssue(t testing.TB, b *gitbugio.Backend, idPrefix string) {
	t.Helper()
	bug, err := b.TestCache().Bugs().ResolvePrefix(idPrefix)
	if err != nil {
		t.Fatalf("ResolvePrefix: %v", err)
	}
	if _, err := bug.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := bug.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

// fakeForge is a bridge to git-bug's in-memory forge, synctest.Fake: the
// one of forges named by the bridge's project.
type fakeForge struct{}

var forges = map[string]*synctest.Fake{}

func (fakeForge) Target() string                          { return "fakeforge" }
func (fakeForge) LoginMetaKey() string                    { return "fake-login" }
func (fakeForge) NewImporter() core.Importer              { return nil }
func (fakeForge) NewExporter() core.Exporter              { return nil }
func (fakeForge) ValidParams() map[string]interface{}     { return nil }
func (fakeForge) ValidateConfig(core.Configuration) error { return nil }
func (fakeForge) Configure(*cache.RepoCache, core.BridgeParams, bool) (core.Configuration, error) {
	return nil, nil
}
func (fakeForge) NewRemote(_ context.Context, _ *cache.RepoCache, conf core.Configuration) (core.Remote, error) {
	return forges[conf["project"]], nil
}

func init() { core.Register(&fakeForge{}) }

// NewForge adds a sync bridge named name to a fake forge of its own, which
// it returns: its people act on it with UserAct, the bot posts for git-bug.
func NewForge(t testing.TB, b *gitbugio.Backend, name string) *synctest.Fake {
	t.Helper()
	bot := core.Account{User: core.User{ID: "1", Login: "bot", Name: "Bot"}}
	f := synctest.NewFake(name+".test/1", name+".test", synctest.Quirks{}, bot)
	forges[name] = f
	for k, v := range map[string]string{core.ConfigKeyTarget: "fakeforge", "project": name, core.ConfigKeyDefaultLogin: "bot", core.ConfigKeySync: "true"} {
		if err := core.SetBridgeConfig(b.TestCache(), name, k, v); err != nil {
			t.Fatal(err)
		}
	}
	return f
}
