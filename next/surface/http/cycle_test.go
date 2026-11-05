package httpapi

import (
	"os"
	"path/filepath"
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
