package httpapi

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/kernel"
)

// Insights replays the board's history: an issue moved through the
// status field and closed counts in the flow, the throughput and the
// cycle time; a deleted one doesn't. With sprint, the sprint's report.
func TestInsights(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	b := openKernel(t, dir)
	day := func(d int) int64 { return time.Now().AddDate(0, 0, -d).Unix() }
	ada := strings.TrimSpace(gitRun(t, dir, "config", "board.me"))
	e, err := issues.Kind.Create(ctx, b, "", op("create", day(20), kernel.ID(ada), `{"title":"Recent","body":""}`),
		op("change-labels", day(19), kernel.ID(ada), `{"add":["status:todo","sprint:s1"]}`),
		op("change-labels", day(10), kernel.ID(ada), `{"add":["status:doing"],"remove":["status:todo"]}`),
		op("set-status", day(4), kernel.ID(ada), `{"status":"closed"}`))
	if err != nil {
		t.Fatal(err)
	}
	b.Close()
	start := time.Now().AddDate(0, 0, -25).Format(time.DateOnly)
	toml := boardToml + fmt.Sprintf("\n[fields.sprint]\ntype = \"iteration\"\n\n[[sprints.list]]\nslug = \"s1\"\ntitle = \"Sprint 1\"\nstart = %s\n", start)
	if err := os.WriteFile(filepath.Join(dir, ".git-board.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newAPI(t, dir)

	type report struct {
		Field     string
		Values    []string
		Days      []string
		Open, All map[string][]int
		Weeks     []struct{ Opened, Closed int }
		Cycle     struct {
			From   string
			Days   []float64
			Median float64
		}
		Burnup *struct {
			Sprint string
			Scope  []int
		}
		Headlines map[string]string
	}
	r := want[report](t, a.do("GET", "/api/insights?days=30", nil), 200)
	if r.Field != "status" || len(r.Days) != 31 || r.All["done"] != nil {
		t.Fatalf("GET /api/insights = %+v", r)
	}
	if got := r.All["doing"]; got == nil || got[len(got)-1] != 1 || r.Open["doing"][len(got)-1] != 0 {
		t.Fatalf("the flow of doing = %v, open %v: want the closed issue counted, not open", got, r.Open["doing"])
	}
	closed := 0
	for _, w := range r.Weeks {
		closed += w.Closed
	}
	if closed != 1 || r.Cycle.From != "started" || len(r.Cycle.Days) != 1 || r.Cycle.Median < 5.9 || r.Cycle.Median > 6.1 {
		t.Fatalf("throughput %d closed, cycle %+v: want 1, 6 days from doing", closed, r.Cycle)
	}
	if r.Burnup == nil || r.Burnup.Sprint != "s1" || r.Headlines["flow"] == "" {
		t.Fatalf("burn-up %+v, headlines %v", r.Burnup, r.Headlines)
	}

	byPrio := want[report](t, a.do("GET", "/api/insights?days=30&field=prio&view=mine", nil), 200)
	if byPrio.Field != "prio" {
		t.Fatalf("field=prio = %+v", byPrio)
	}
	for q, field := range map[string]string{"days=900": "days", "view=nope": "view", "days=x": "days"} {
		if e := want[apiError](t, a.do("GET", "/api/insights?"+q, nil), 400); e.Code != "invalid" || e.Field != field {
			t.Errorf("?%s = %+v, want invalid on %s", q, e, field)
		}
	}

	type sprintReport struct{ Sprint, Title, Markdown string }
	sr := want[sprintReport](t, a.do("GET", "/api/insights?sprint=", nil), 200)
	if sr.Sprint != "s1" || !strings.Contains(sr.Markdown, "# Sprint 1") || !strings.Contains(sr.Markdown, "**1 of 1 done (100%)**") || !strings.Contains(sr.Markdown, "`"+string(e.ID[:7])+"` Recent") {
		t.Fatalf("the sprint's report = %+v", sr)
	}
	if e := want[apiError](t, a.do("GET", "/api/insights?sprint=s9", nil), 404); e.Code != "not-found" {
		t.Fatalf("an unknown sprint = %+v", e)
	}
}
