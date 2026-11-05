package httpapi

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The cycle's reads: off, experimental; on, an issue's step, and the
// Work page's groups, as the person who writes.
func TestCycle(t *testing.T) {
	t.Parallel()
	dir, ids := newBoard(t)
	a := newAPI(t, dir)
	id := string(ids[1])
	for _, path := range []string{"/api/issues/" + id + "/cycle", "/api/cycle"} {
		if e := want[apiError](t, a.do("GET", path, nil), 404); e.Code != "experimental" {
			t.Fatalf("GET %s, the cycle off = %+v", path, e)
		}
	}

	on := boardToml + "\n[experimental]\nfeatures = [\"cycle\"]\n"
	if err := os.WriteFile(filepath.Join(dir, ".git-board.toml"), []byte(on), 0o644); err != nil {
		t.Fatal(err)
	}
	b := newAPI(t, dir)
	type cycleBody struct {
		Step     int
		Of       int
		Name     string
		Done     bool
		Waits    struct{ What string }
		Evidence struct{ Status string }
	}
	c := want[cycleBody](t, b.do("GET", "/api/issues/"+id+"/cycle", nil), 200)
	if c.Step == 0 || c.Of == 0 || c.Name == "" || c.Done {
		t.Fatalf("GET /api/issues/{id}/cycle = %+v", c)
	}
	type groups struct {
		Needs, Waiting []struct {
			Issue struct{ ID string }
			Step  int
			Name  string
		}
	}
	mine := want[groups](t, b.do("GET", "/api/cycle", nil), 200)
	team := want[groups](t, b.do("GET", "/api/cycle?who=team", nil), 200)
	if mine.Needs == nil || mine.Waiting == nil || len(team.Needs)+len(team.Waiting) != 1 {
		t.Fatalf("GET /api/cycle = %+v, team %+v: want the one open issue in team's", mine, team)
	}
	if e := want[apiError](t, b.do("GET", "/api/cycle?who=all", nil), 400); e.Field != "who" {
		t.Fatalf("who=all = %+v", e)
	}
}

// Route 50: an action from the version read (If-Match), 412 from
// another; the cycle after it with its version as ETag; an unknown
// action not found, a wrong value invalid, a step not the viewer's
// forbidden; off, experimental.
func TestCycleActions(t *testing.T) {
	t.Parallel()
	dir, ids := newBoard(t)
	id := string(ids[1])
	off := newAPI(t, dir)
	if e := want[apiError](t, off.do("POST", "/api/issues/"+id+"/cycle/set-risk", map[string]string{"risk": "low"}), 404); e.Code != "experimental" {
		t.Fatalf("off: %+v", e)
	}
	on := boardToml + "\n[experimental]\nfeatures = [\"cycle\"]\n\n[cycle]\nready = \"todo\"\n"
	if err := os.WriteFile(filepath.Join(dir, ".git-board.toml"), []byte(on), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newAPI(t, dir)
	want[map[string]any](t, a.do("PATCH", "/api/issues/"+id, map[string]any{"set": []string{"status=todo"}, "body": "## Done when\n- it works\n"}), 200)
	type view struct {
		Step    int
		Risk    string
		Actions []string
		Version string
	}
	res := a.do("GET", "/api/issues/"+id+"/cycle", nil)
	etag := res.Header.Get("ETag")
	v := want[view](t, res, 200)
	if v.Step != 2 || etag != `"`+v.Version+`"` || !slices.Equal(v.Actions, []string{"set-risk"}) {
		t.Fatalf("GET: %+v, ETag %s", v, etag)
	}
	if e := want[apiError](t, a.do("POST", "/api/issues/"+id+"/cycle/set-risk", map[string]string{"risk": "low"}, "If-Match", `"0000"`), 412); e.Code != "stale" {
		t.Fatalf("a stale version: %+v", e)
	}
	if e := want[apiError](t, a.do("POST", "/api/issues/"+id+"/cycle/set-risk", map[string]string{"risk": "huge"}), 400); e.Code != "invalid" {
		t.Fatalf("a wrong risk: %+v", e)
	}
	if e := want[apiError](t, a.do("POST", "/api/issues/"+id+"/cycle/fly", nil), 404); e.Code != "not-found" {
		t.Fatalf("no such action: %+v", e)
	}
	if e := want[apiError](t, a.do("POST", "/api/issues/"+id+"/cycle/ready-for-review", nil), 403); e.Code != "forbidden" {
		t.Fatalf("not the viewer's step: %+v", e)
	}
	res = a.do("POST", "/api/issues/"+id+"/cycle/set-risk", map[string]string{"risk": "low"}, "If-Match", etag)
	after := res.Header.Get("ETag")
	v = want[view](t, res, 200)
	if v.Step != 3 || v.Risk != "low" || after == etag || after != `"`+v.Version+`"` {
		t.Fatalf("set-risk: %+v, ETag %s after %s", v, after, etag)
	}
	if e := want[apiError](t, a.do("POST", "/api/issues/"+id+"/cycle/start", nil, "If-Match", etag), 412); e.Code != "stale" {
		t.Fatalf("from the version before: %+v", e)
	}
	if v = want[view](t, a.do("POST", "/api/issues/"+id+"/cycle/start", nil, "If-Match", after), 200); v.Step != 4 {
		t.Fatalf("start: %+v", v)
	}
}
