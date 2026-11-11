package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guilbep/git-board/app"
	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/fake"
	"github.com/guilbep/git-board/tools/ghproject/projectfake"
)

// The first sync, chosen (docs/prd/config-fit.md, Part 2), end to end on
// the fake forge and the fake Project: the whole check counts each part
// with its issues and checks the Project in the same run; a sync of one
// part leaves the others for the next, which a second check still
// counts; a Project check and a Project sync stop on DELETE /api/sync,
// nothing written to the Project. It swaps app.OpenForge and
// app.GitHubAPI and sets GITHUB_TOKEN: serial.
func TestTheChosenSync(t *testing.T) {
	dir, _ := newBoard(t)
	toml := boardToml + "\n[forges.gh]\nkind = \"github\"\nproject = \"o/r\"\nsync = true\n\n[github-project]\nowner = \"o\"\nnumber = 1\nwrite = true\n"
	if err := os.WriteFile(filepath.Join(dir, ".git-board.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}

	// the forge: One and Two pulled once, then Two renamed there and
	// Three opened there
	f := fake.New("github.com/o/r")
	for _, title := range []string{"One", "Two"} {
		f.Add(forge.Issue{Author: forge.Account{ID: "7", Login: "ann"}, Title: title})
	}
	b, err := app.Open(ctx, dir, app.Options{Who: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.SyncForges(ctx, app.WayPull, app.ForgeRun{Forges: []app.NamedForge{{Name: "gh", Forge: f}}}); err != nil {
		t.Fatal(err)
	}
	b.Close()
	f.Edit("2", func(is *forge.Issue) { is.Title = "Two, renamed" })
	f.Add(forge.Issue{Author: forge.Account{ID: "7", Login: "ann"}, Title: "Three"})
	was := app.OpenForge
	app.OpenForge = func(context.Context, forge.Table) (forge.Forge, error) { return f, nil }
	t.Cleanup(func() { app.OpenForge = was })

	// the Project, behind a door that holds its answers while hold is on,
	// until the request's context ends, or the test does: a body left
	// unread, the server may not hear the client go
	p := projectfake.New("o", 1)
	p.AddField(projectfake.Field{ID: "F-status", Name: "Status", DataType: "SINGLE_SELECT", Choices: []projectfake.Choice{
		{ID: "o-todo", Name: "Todo"}, {ID: "o-doing", Name: "Doing"}, {ID: "o-done", Name: "Done"}}})
	p.Add(projectfake.Item{Number: 1, URL: "https://github.com/o/r/issues/1", Title: "One", Values: map[string]string{"Status": "Doing"}})
	p.Add(projectfake.Item{Number: 2, URL: "https://github.com/o/r/issues/2", Title: "Two", Values: map[string]string{"Status": "Todo"}})
	target, err := url.Parse(p.Server(t).URL)
	if err != nil {
		t.Fatal(err)
	}
	var hold atomic.Bool
	held, ended := make(chan struct{}, 1), make(chan struct{})
	door := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hold.Load() {
			select {
			case held <- struct{}{}:
			default:
			}
			select {
			case <-r.Context().Done():
			case <-ended:
			}
			return
		}
		httputil.NewSingleHostReverseProxy(target).ServeHTTP(w, r)
	}))
	t.Cleanup(func() { close(ended); door.Close() })
	api := app.GitHubAPI
	app.GitHubAPI = door.URL
	t.Cleanup(func() { app.GitHubAPI = api })
	t.Setenv("GITHUB_TOKEN", "t")
	a := newAPI(t, dir)

	// One changed here: a change to send
	one := want[issuePage](t, a.do("GET", "/api/issues?filter=One", nil), 200)
	if len(one.Items) != 1 {
		t.Fatalf("One: %+v", one.Items)
	}
	want[map[string]any](t, a.do("PATCH", "/api/issues/"+one.Items[0].ID, map[string]any{"title": "One, here"}), 200)

	counts := func(r Run) map[string]int {
		n := map[string]int{}
		for part, issues := range r.Parts {
			n[part] = len(issues)
		}
		return n
	}
	wantCounts := func(what string, r Run, want map[string]int) {
		t.Helper()
		got := counts(r)
		for _, part := range app.Parts {
			if got[string(part)] != want[string(part)] {
				t.Fatalf("%s: parts %v, want %v; steps %+v", what, got, want, r.Steps)
			}
		}
	}

	// the check: each part counted, its issues named, the Project read in
	// the same run, nothing written anywhere
	ck := ran(t, a, map[string]any{"scope": "all", "dry_run": true})
	if ck.State != "done" {
		t.Fatalf("the check: %s %s %+v", ck.State, ck.Error, ck.Steps)
	}
	wantCounts("the check", ck, map[string]int{"pull-changes": 1, "pull-new": 1, "create": 2, "send": 1})
	if n := ck.Parts["pull-new"][0].Title; n != "#3 Three" {
		t.Errorf("the new issue is named %q", n)
	}
	st := want[projectBody](t, a.do("GET", "/api/github-project", nil), 200)
	if st.Last == nil || st.Last.Applied || st.Last.Counts.Matched != 2 {
		t.Fatalf("the Project in the check: %+v", st.Last)
	}
	if len(f.Writes())+len(p.Writes()) != 0 {
		t.Fatalf("the check wrote %v %v", f.Writes(), p.Writes())
	}

	// a sync of the new issues alone: Three comes, nothing goes
	if sy := ran(t, a, map[string]any{"scope": "all", "only": []string{"pull-new"}}); sy.State != "done" || len(sy.Only) != 1 {
		t.Fatalf("the sync of pull-new: %s %s %v", sy.State, sy.Error, sy.Only)
	}
	if got := want[issuePage](t, a.do("GET", "/api/issues?filter=Three", nil), 200); len(got.Items) != 1 {
		t.Errorf("Three not pulled: %+v", got.Items)
	}
	if len(f.Writes())+len(p.Writes()) != 0 {
		t.Fatalf("a sync of pull-new wrote %v %v", f.Writes(), p.Writes())
	}

	// the parts left out wait for the next sync: a check counts them still
	wantCounts("the second check", ran(t, a, map[string]any{"scope": "all", "dry_run": true}),
		map[string]int{"pull-changes": 1, "create": 2, "send": 1})

	// a Project check, then a Project sync, stopped while the Project reads
	for _, dry := range []bool{true, false} {
		hold.Store(true)
		want[Run](t, a.do("POST", "/api/sync", map[string]any{"scope": "all", "dry_run": dry}), 202)
		select {
		case <-held:
		case <-time.After(10 * time.Second):
			t.Fatalf("dry run %v: the Project was never read", dry)
		}
		want[map[string]any](t, a.do("DELETE", "/api/sync", nil), 202)
		var r Run
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
			if s := want[syncRes](t, a.do("GET", "/api/sync", nil), 200); s.Run != nil && s.Run.State != "running" {
				r = *s.Run
				break
			}
		}
		hold.Store(false)
		if r.State != "stopped" {
			t.Fatalf("dry run %v, stopped: %s %s", dry, r.State, r.Error)
		}
	}
	if w := p.Writes(); len(w) != 0 {
		t.Errorf("a stopped Project sync wrote %q", w)
	}
}
