package httpapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/next/app"
	"github.com/guilbep/git-board/next/domain/releases"
	"github.com/guilbep/git-board/next/tools/ship"
	"github.com/guilbep/git-board/next/tools/ship/shiptest"
)

type releasesBody struct {
	Enabled      bool
	Trunk        string
	Environments []struct{ Name, Version string }
	Releases     []struct {
		Tag struct {
			Name string
		}
		Record *struct {
			Notes, Yanked string
			Comments      []struct{ Message string }
		}
	}
	Job *Job
}

// The releases' routes on shiptest's app: what runs where, a cut as a
// job, its page, its notes, a comment and a yank on its record, the next
// cut's draft, and what a deploy refuses. Serial: the fixture sets the
// git environment.
func TestReleases(t *testing.T) {
	_, tools, fx := shiptest.NewRunner(t)
	if err := os.WriteFile(filepath.Join(fx.App, ".git-board.toml"), []byte(fx.Config(t)), 0o644); err != nil {
		t.Fatal(err)
	}
	old := app.ShipTools
	app.ShipTools = func(ship.Publisher) ship.Tools { return tools }
	t.Cleanup(func() { app.ShipTools = old })
	c := shiptest.Git(t, fx.App, "rev-list", "--max-parents=0", "HEAD")
	fx.Tag(t, c, "v1.0.0")
	for i := range fx.ConfigBare {
		for _, env := range []string{"stage", "prod"} {
			fx.SetOverlay(t, i, env, releases.Deployed{Commit: c, Version: "v1.0.0"})
		}
	}
	m := fx.Merge(t, "fix/12-a", "")
	for i := range fx.ConfigBare {
		fx.SetOverlay(t, i, "dev", releases.Deployed{Commit: m, Version: "1.0.1-dev.1.g" + m[:7]})
	}
	a := newAPI(t, fx.App)

	local := want[releasesBody](t, a.do("GET", "/api/releases?local=1", nil), 200)
	if !local.Enabled || local.Trunk != "dev" || len(local.Environments) != 3 || len(local.Releases) != 1 || local.Releases[0].Tag.Name != "v1.0.0" || local.Job != nil {
		t.Fatalf("GET /api/releases?local = %+v", local)
	}
	draft := want[struct{ Tag string }](t, a.do("GET", "/api/releases/next", nil), 200)
	if draft.Tag != "v1.0.1-rc.1" {
		t.Fatalf("the draft = %+v, want v1.0.1-rc.1", draft)
	}

	job := want[Job](t, a.do("POST", "/api/releases", map[string]any{"notes": "Fixes the login."}), 202)
	if job.Kind != "cut" || job.State != "running" {
		t.Fatalf("POST /api/releases = %+v", job)
	}
	var all releasesBody
	for deadline := time.Now().Add(20 * time.Second); ; {
		all = want[releasesBody](t, a.do("GET", "/api/releases", nil), 200)
		if all.Job != nil && all.Job.State != "running" || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if all.Job == nil || all.Job.State != "done" || all.Job.Subject != "v1.0.1-rc.1" || len(all.Job.Steps) == 0 {
		t.Fatalf("the cut's job = %+v", all.Job)
	}
	if all.Releases[0].Tag.Name != "v1.0.1-rc.1" || all.Releases[0].Record == nil || all.Releases[0].Record.Notes != "Fixes the login." {
		t.Fatalf("the releases after the cut = %+v", all.Releases)
	}

	type page struct {
		Row struct {
			Tag struct{ Name string }
		}
		Issues []struct{ Number, Title string }
	}
	p := want[page](t, a.do("GET", "/api/releases/v1.0.1-rc.1", nil), 200)
	if p.Row.Tag.Name != "v1.0.1-rc.1" || p.Issues == nil {
		t.Fatalf("GET /api/releases/{tag} = %+v", p)
	}
	if e := want[apiError](t, a.do("GET", "/api/releases/v9.9.9", nil), 404); e.Code != "not-found" {
		t.Fatalf("an unknown tag = %+v", e)
	}

	want[struct{}](t, a.do("PATCH", "/api/releases/v1.0.1-rc.1", map[string]any{"notes": "Fixes the login, for real."}), 200)
	want[struct{}](t, a.do("POST", "/api/releases/v1.0.1-rc.1/comments", map[string]any{"body": "Looks good on stage."}), 200)
	if e := want[apiError](t, a.do("PATCH", "/api/releases/v1.0.1-rc.1", map[string]any{"state": "yanked"}), 400); e.Field != "reason" {
		t.Fatalf("a yank without a reason = %+v", e)
	}
	want[struct{}](t, a.do("PATCH", "/api/releases/v1.0.1-rc.1", map[string]any{"state": "yanked", "reason": "breaks SSO"}), 200)
	rec := want[releasesBody](t, a.do("GET", "/api/releases?local=1", nil), 200).Releases[0].Record
	if rec == nil || rec.Notes != "Fixes the login, for real." || rec.Yanked != "breaks SSO" || len(rec.Comments) != 1 || !strings.Contains(rec.Comments[0].Message, "Looks good") {
		t.Fatalf("the record after its writes = %+v", rec)
	}
	if e := want[apiError](t, a.do("PATCH", "/api/releases/v1.0.0", map[string]any{"notes": "x"}), 404); e.Code != "not-found" {
		t.Fatalf("notes on a tag without a record = %+v", e)
	}

	for _, c := range []struct {
		path   string
		body   map[string]any
		status int
		field  string
	}{
		{"/api/environments/qa/deploy", map[string]any{"tag": "v1.0.0"}, 404, ""},
		{"/api/environments/prod/deploy", map[string]any{}, 400, "tag"},
		{"/api/environments/prod/deploy", map[string]any{"tag": "nope"}, 400, "tag"},
		{"/api/environments/prod/deploy", map[string]any{"tag": "v8.0.0"}, 404, ""},
	} {
		if e := want[apiError](t, a.do("POST", c.path, c.body), c.status); e.Field != c.field {
			t.Errorf("POST %s %v = %+v, want on %q", c.path, c.body, e, c.field)
		}
	}
}

// Without [release], the list answers its shape, empty; a release's
// page, not found.
func TestReleasesOff(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	a := newAPI(t, dir)
	r := want[releasesBody](t, a.do("GET", "/api/releases", nil), 200)
	if r.Enabled || r.Releases == nil || r.Environments == nil {
		t.Fatalf("GET /api/releases without [release] = %+v", r)
	}
	if e := want[apiError](t, a.do("GET", "/api/releases/v1.0.0", nil), 404); e.Code != "not-found" {
		t.Fatalf("a release without [release] = %+v", e)
	}
}
