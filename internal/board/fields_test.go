package board_test

import (
	"testing"
	"time"

	"github.com/guilbep/git-board/core/fields"
	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// fields.Apply on the store, with the person git-board writes as: @me is
// theirs, and the board finds it.

var today = time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)

func mustLoadConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load("../config/testdata/git-board.toml")
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}

// @me with no --me and no one chosen to write as means nobody.
func TestApplyMeWithoutMeConfiguredErrors(t *testing.T) {
	cfg := mustLoadConfig(t)
	dir := storetest.Repo(t, false)
	storetest.Init(t, dir)
	b := storetest.Open(t, dir)
	id := storetest.NewIssue(t, b, "x")
	storetest.Git(t, dir, "config", "--unset", "board.me")

	if _, err := fields.Apply(b, cfg.IssueSchema(), id, "assignee", "@me", "", today); err == nil {
		t.Error("expected error when @me used without a configured user")
	}
}

// Without --me, @me is the person you write as: their name while they
// have no forge account (issue #87), their login once they have one.
func TestApplyMeFallsBackToTheIdentitysName(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")
	me, _, err := b.Me()
	if err != nil {
		t.Fatal(err)
	}
	label, err := fields.Apply(b, cfg.IssueSchema(), id, "assignee", "@me", "", today)
	if err != nil || label != "assignee:"+me.Name {
		t.Errorf("Apply @me = %q, %v; want assignee:%s", label, err, me.Name)
	}
	for _, f := range []string{"assignee:@me", "involves:@me"} {
		if issues, err := board.LoadMatching(b, cfg, f, ""); err != nil || len(issues) != 1 {
			t.Errorf("LoadMatching %s = %d issues, %v; want the one", f, len(issues), err)
		}
	}
}

// Without --me, @me is the forge login of the person you write as.
func TestApplyMeFallsBackToTheIdentitysLogin(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")
	storetest.SetMyLogin(t, b, "guilbep")
	label, err := fields.Apply(b, cfg.IssueSchema(), id, "assignee", "@me", "", today)
	if err != nil || label != "assignee:guilbep" {
		t.Errorf("Apply @me = %q, %v; want assignee:guilbep", label, err)
	}
	issues, err := board.LoadMatching(b, cfg, "assignee:@me", "")
	if err != nil || len(issues) != 1 {
		t.Errorf("LoadMatching assignee:@me = %d issues, %v; want the one", len(issues), err)
	}
}

// assignee=+@me adds you, -@me removes you: the sign is split off before
// @me resolves (review: fields.go:108).
func TestApplySignedMeResolves(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")
	storetest.SetMyLogin(t, b, "guilbep")
	if label, err := fields.Apply(b, cfg.IssueSchema(), id, "assignee", "+@me", "", today); err != nil || label != "assignee:guilbep" {
		t.Fatalf("Apply +@me = %q, %v; want assignee:guilbep", label, err)
	}
	if _, err := fields.Apply(b, cfg.IssueSchema(), id, "assignee", "-@me", "", today); err != nil {
		t.Fatalf("Apply -@me: %v", err)
	}
	if got, _ := b.GetLabels(id); len(got) != 0 {
		t.Errorf("labels = %v, want none after -@me", got)
	}
}
