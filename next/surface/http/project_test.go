package httpapi

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/next/app"
	"github.com/guilbep/git-board/next/forge"
	"github.com/guilbep/git-board/next/forge/fake"
	"github.com/guilbep/git-board/next/forge/github"
	"github.com/guilbep/git-board/next/tools/ghproject"
	"github.com/guilbep/git-board/next/tools/ghproject/projectfake"
)

// projectAPI is the test board, its issues 1 and 2 pulled from
// github.com/o/r, its .git-board.toml naming o's Project 1, which
// app.OpenProjectV2 opens on a fake for the test: it swaps a package
// variable, so its tests are serial.
func projectAPI(t *testing.T, write bool) (*api, *projectfake.Fake) {
	t.Helper()
	dir, _ := newBoard(t)
	toml := boardToml + "\n[github-project]\nowner = \"o\"\nnumber = 1\n"
	if write {
		toml += "write = true\n"
	}
	if err := os.WriteFile(filepath.Join(dir, ".git-board.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := app.Open(ctx, dir, app.Options{Who: true})
	if err != nil {
		t.Fatal(err)
	}
	f := fake.New("github.com/o/r")
	for _, title := range []string{"One", "Two"} {
		f.Add(forge.Issue{Author: forge.Account{ID: "7", Login: "ann"}, Title: title})
	}
	if _, err := b.SyncForges(ctx, app.WayPull, app.ForgeRun{Forges: []app.NamedForge{{Name: "gh", Forge: f}}}); err != nil {
		t.Fatal(err)
	}
	b.Close()

	p := projectfake.New("o", 1)
	p.AddField(projectfake.Field{ID: "F-status", Name: "Status", DataType: "SINGLE_SELECT", Choices: []projectfake.Choice{
		{ID: "o-todo", Name: "Todo"}, {ID: "o-doing", Name: "Doing"}, {ID: "o-done", Name: "Done"}}})
	p.Add(projectfake.Item{Number: 1, URL: "https://github.com/o/r/issues/1", Title: "One", Values: map[string]string{"Status": "Doing"}})
	p.Add(projectfake.Item{Number: 2, URL: "https://github.com/o/r/issues/2", Title: "Two", Values: map[string]string{"Status": "Todo"}})
	srv := p.Server(t)
	was := app.OpenProjectV2
	app.OpenProjectV2 = func(ctx context.Context, _, _, owner string, number int) (ghproject.Project, error) {
		return github.OpenProject(ctx, github.Options{API: srv.URL, Token: func(context.Context, forge.Account) (string, error) { return "t", nil }}, owner, number)
	}
	t.Cleanup(func() { app.OpenProjectV2 = was })
	return newAPI(t, dir), p
}

type projectBody struct {
	Project string `json:"project"`
	Write   bool   `json:"write"`
	Kept    string `json:"kept"`
	Last    *struct {
		Title   string `json:"title"`
		Applied bool   `json:"applied"`
		Counts  struct {
			Matched       int `json:"matched"`
			LabelChanges  int `json:"label_changes"`
			ProjectWrites int `json:"project_writes"`
			Conflicts     int `json:"conflicts"`
		} `json:"counts"`
		Conflicts []ghproject.Conflict `json:"conflicts"`
		Changes   []struct {
			Number int      `json:"number"`
			Added  []string `json:"added"`
			Writes []string `json:"writes"`
		} `json:"changes"`
	} `json:"last"`
	Places []struct {
		Field     string `json:"field"`
		Forge     string `json:"forge"`
		To        string `json:"to"`
		Column    string `json:"column"`
		OnProject *bool  `json:"on_project"`
	} `json:"places"`
}

// The bridge's routes: the status before any check, a check, an apply,
// a conflict and its resolution, and where each field lives.
func TestGithubProjectRoutes(t *testing.T) {
	a, p := projectAPI(t, true)
	st := want[projectBody](t, a.do("GET", "/api/github-project", nil), 200)
	if st.Project != "o/1" || !st.Write || st.Last != nil {
		t.Fatalf("status before a check: %+v", st)
	}
	var status *struct {
		To, Column string
		OnProject  *bool
	}
	for _, pl := range st.Places {
		if pl.Field == "status" && pl.Forge == "gh" {
			status = &struct {
				To, Column string
				OnProject  *bool
			}{pl.To, pl.Column, pl.OnProject}
		}
	}
	if status == nil || status.To != "project" || status.Column != "Status" || status.OnProject != nil {
		t.Errorf("status's place: %+v in %+v", status, st.Places)
	}

	ck := want[projectBody](t, a.do("POST", "/api/github-project/check", map[string]any{}), 200)
	if ck.Last == nil || ck.Last.Applied || ck.Last.Counts.Matched != 2 || ck.Last.Counts.LabelChanges != 2 || ck.Last.Title != "o's Project" {
		t.Fatalf("check: %+v", ck.Last)
	}
	if len(p.Writes()) != 0 {
		t.Errorf("a check wrote %q", p.Writes())
	}
	for _, pl := range ck.Places {
		if pl.Field == "status" && (pl.OnProject == nil || !*pl.OnProject) {
			t.Errorf("status after a check: %+v", pl)
		}
		if pl.Field == "prio" && pl.OnProject != nil && *pl.OnProject {
			t.Errorf("prio, which the Project hasn't: %+v", pl)
		}
	}
	ap := want[projectBody](t, a.do("POST", "/api/github-project/apply", map[string]any{"side": "labels"}), 200)
	if !ap.Last.Applied || len(ap.Last.Changes) != 2 || ap.Last.Changes[0].Added[0] != "status:doing" {
		t.Fatalf("apply: %+v", ap.Last)
	}
	if got := want[projectBody](t, a.do("GET", "/api/github-project", nil), 200); got.Last == nil || !got.Last.Applied {
		t.Errorf("the last run kept: %+v", got.Last)
	}

	// both sides move #1: a conflict, kept on the Project's side
	list := want[issuePage](t, a.do("GET", "/api/issues?filter=One", nil), 200)
	if len(list.Items) != 1 {
		t.Fatalf("issues %+v", list.Items)
	}
	want[map[string]any](t, a.do("PATCH", "/api/issues/"+list.Items[0].ID, map[string]any{"labels": map[string]any{"add": []string{"status:done"}, "remove": []string{"status:doing"}}}), 200)
	p.Edit("PVTI_1", func(it *projectfake.Item) { it.Values["Status"] = "Todo" })
	ck = want[projectBody](t, a.do("POST", "/api/github-project/check", nil), 200)
	if len(ck.Last.Conflicts) != 1 || ck.Last.Conflicts[0].Local != "done" || ck.Last.Conflicts[0].Project != "todo" {
		t.Fatalf("conflicts %+v", ck.Last.Conflicts)
	}
	rs := want[projectBody](t, a.do("POST", "/api/github-project/resolve", map[string]any{"issue": "o/r#1", "field": "status", "keep": "git-board"}), 200)
	if rs.Kept != "done" || len(rs.Last.Conflicts) != 0 {
		t.Errorf("resolve: %q, %+v", rs.Kept, rs.Last)
	}
	if w := p.Writes(); len(w) != 1 || w[0] != "#1 Status = Done" {
		t.Errorf("writes %q", w)
	}

	for _, c := range []struct {
		path string
		body map[string]any
		code int
	}{
		{"/api/github-project/resolve", map[string]any{"issue": "1", "field": "status", "keep": "git-board"}, 404},
		{"/api/github-project/resolve", map[string]any{"issue": "1", "field": "status", "keep": "mine"}, 400},
		{"/api/github-project/apply", map[string]any{"side": "left"}, 400},
		{"/api/github-project/nope", map[string]any{}, 404},
	} {
		if res := a.do("POST", c.path, c.body); res.StatusCode != c.code {
			t.Errorf("%s %v: %d, want %d", c.path, c.body, res.StatusCode, c.code)
		}
	}
}

// Without [github-project], the status says so and an action is a
// usage error.
func TestGithubProjectNone(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	a := newAPI(t, dir)
	st := want[projectBody](t, a.do("GET", "/api/github-project", nil), 200)
	if st.Project != "" || st.Last != nil {
		t.Errorf("status %+v", st)
	}
	if e := want[apiError](t, a.do("POST", "/api/github-project/check", nil), 400); !strings.Contains(e.Message, "github-project") {
		t.Errorf("check: %+v", e)
	}
}
