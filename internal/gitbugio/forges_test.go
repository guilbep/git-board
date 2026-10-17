package gitbugio_test

import (
	"context"
	"testing"

	"github.com/git-bug/git-bug/bridge/core"
	"github.com/git-bug/git-bug/bridge/core/synctest"
	"github.com/git-bug/git-bug/cache"

	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

// fakeForge is a bridge to git-bug's fake forge, synctest.Fake.
type fakeForge struct{}

var theForge *synctest.Fake

func (fakeForge) Target() string                          { return "fakeforge" }
func (fakeForge) LoginMetaKey() string                    { return "fake-login" }
func (fakeForge) NewImporter() core.Importer              { return nil }
func (fakeForge) NewExporter() core.Exporter              { return nil }
func (fakeForge) ValidParams() map[string]interface{}     { return nil }
func (fakeForge) ValidateConfig(core.Configuration) error { return nil }
func (fakeForge) Configure(*cache.RepoCache, core.BridgeParams, bool) (core.Configuration, error) {
	return nil, nil
}
func (fakeForge) NewRemote(context.Context, *cache.RepoCache, core.Configuration) (core.Remote, error) {
	return theForge, nil
}

func init() { core.Register(&fakeForge{}) }

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
	b := gitbugiotest.New(t)
	bot := core.Account{User: core.User{ID: "1", Login: "bot", Name: "Bot"}}
	theForge = synctest.NewFake("forge.test/1", "forge.test", synctest.Quirks{}, bot)
	for k, v := range map[string]string{core.ConfigKeyTarget: "fakeforge", core.ConfigKeyDefaultLogin: "bot", core.ConfigKeySync: "true"} {
		if err := core.SetBridgeConfig(b.TestCache(), "fk", k, v); err != nil {
			t.Fatal(err)
		}
	}
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
