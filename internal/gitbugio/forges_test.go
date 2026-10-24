package gitbugio_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/git-bug/git-bug/bridge/core"
	"github.com/git-bug/git-bug/bridge/core/auth"
	"github.com/git-bug/git-bug/cache"
	"github.com/git-bug/gitconfig"

	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

func drain(t *testing.T, results <-chan core.SyncResult, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	for r := range results {
		if r.IsError() {
			t.Error(r.String())
		}
	}
}

// Get brings the forge's issues in and sends nothing; Send sends and gets
// nothing.
func TestGetAndSendAreOneWay(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	theForge := gitbugiotest.NewForge(t, b, "fk")
	theForge.UserAct("alice").CreateIssue("from the forge", "body")
	gitbugiotest.NewIssue(t, b, "from git-board")

	results, err := b.GetForges(context.Background(), nil)
	drain(t, results, err)
	if issues, _ := b.ListIssues(); len(issues) != 2 || theForge.Writes() != 0 {
		t.Fatalf("after Get: %d issues here, %d writes to the forge; want 2 and none", len(issues), theForge.Writes())
	}
	if cfg, _ := b.TestCache().Config().Read(); cfg.Value("git-bug.bridge.fk.lastSyncTime", "") == "" {
		t.Error("Get didn't move the sync's pull time on, as a sync does")
	}

	theForge.UserAct("alice").CreateIssue("later on the forge", "body")
	results, err = b.SendForges(context.Background(), []string{"fk"})
	drain(t, results, err)
	if theForge.Issue("3") == nil || theForge.Issue("3").Title != "from git-board" {
		t.Errorf("Send didn't create the issue on the forge")
	}
	if issues, _ := b.ListIssues(); len(issues) != 2 {
		t.Errorf("Send got %d issues, want the 2 it had", len(issues))
	}

	if _, err := b.GetForges(context.Background(), []string{"nope"}); err == nil {
		t.Error("Get on an unknown bridge: no error")
	}
}

// The token git-board acts on GitHub with is the one of whoever writes
// from it: its identity's GitHub login, as the bridge maps an author to an
// account, never another person's token.
func TestGitHubToken(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	for login, target := range map[string]string{"alice": "github", "bob": "github", "carol": "gitlab"} {
		tok := auth.NewToken(target, "tok-"+login)
		tok.SetMetadata(auth.MetaKeyLogin, login)
		if target == "gitlab" {
			tok.SetMetadata(auth.MetaKeyBaseURL, "https://gitlab.example.com/")
		}
		if err := auth.Store(b.TestCache(), tok); err != nil {
			t.Fatal(err)
		}
	}
	as := func(login string, meta map[string]string) *cache.IdentityCache {
		t.Helper()
		i, err := b.TestCache().Identities().NewRaw("Someone", "", login, "", nil, meta)
		if err != nil {
			t.Fatal(err)
		}
		if err := b.TestCache().SetUserIdentity(t.Context(), i); err != nil {
			t.Fatal(err)
		}
		return i
	}

	if _, _, err := b.GitHubToken(); err == nil || !strings.Contains(err.Error(), "no GitHub login") {
		t.Errorf("no login anywhere: err %v, want no GitHub login", err)
	}
	as("gl-user", map[string]string{"github-login": "Bob"})
	if login, tok, err := b.GitHubToken(); err != nil || login != "Bob" || tok != "tok-bob" {
		t.Errorf("github-login Bob: %q %q %v, want Bob's token", login, tok, err)
	}
	as("carol", nil)
	if _, tok, err := b.GitHubToken(); err == nil || err.Error() != "no GitHub token for carol" || tok != "" {
		t.Errorf("carol, a GitLab token only: %q %v, want no GitHub token for carol", tok, err)
	}
	me := as("gl-user", nil)
	if err := b.TestCache().Config().Update(func(f *gitconfig.File) error {
		return errors.Join(
			f.Set("git-bug.bridge.gl.target", "gitlab"),
			f.Set("git-bug.bridge.gl.login-"+me.Id().String(), "gl-user"),
			f.Set("git-bug.bridge.gh.target", "github"),
			f.Set("git-bug.bridge.gh.login-"+me.Id().String(), "alice"),
		)
	}); err != nil {
		t.Fatal(err)
	}
	if login, tok, err := b.GitHubToken(); err != nil || login != "alice" || tok != "tok-alice" {
		t.Errorf("the GitHub bridge's link alice: %q %q %v, want alice's token", login, tok, err)
	}
}
