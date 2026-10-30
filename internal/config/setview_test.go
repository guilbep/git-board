package config

import (
	"strings"
	"testing"
)

func TestSetViewTextKeepsTheRestOfTheFile(t *testing.T) {
	src := `# our board
[views.backlog]
layout = "table"   # the long list
filter = "is:open"
sort = "prio asc"

[views.mine]
filter = "assignee:@me"
`
	got, err := SetViewText(src, "backlog", View{Layout: "table", Filter: `is:open -status:validated "car sharing"`, Group: "sprint"})
	if err != nil {
		t.Fatal(err)
	}
	want := `# our board
[views.backlog]
layout = "table"   # the long list
filter = "is:open -status:validated \"car sharing\""
group = "sprint"

[views.mine]
filter = "assignee:@me"
`
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	cfg, _, err := Check([]byte(got))
	if err != nil || cfg.Views["backlog"].Filter != `is:open -status:validated "car sharing"` || cfg.Views["backlog"].Sort != "" {
		t.Errorf("reparsed: %+v %v", cfg.Views["backlog"], err)
	}
}

func TestSetViewTextAddsANewView(t *testing.T) {
	got, err := SetViewText("[fields.status]\nsingle = true\n", "in-review", View{Layout: "board", Filter: "status:in-review,qa", Group: "assignee"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "\n\n[views.in-review]\nlayout = \"board\"\nfilter = \"status:in-review,qa\"\ngroup = \"assignee\"\n") {
		t.Errorf("got\n%s", got)
	}
	if _, err := SetViewText("", "my view", View{}); err == nil {
		t.Error("a name with a space was taken")
	}
}

func TestSetViewTextWritesColumns(t *testing.T) {
	src := "[views.backlog]\nlayout = \"table\"\nfilter = \"is:open\"\n"
	got, err := SetViewText(src, "backlog", View{Layout: "table", Filter: "is:open", Columns: []string{"id", "title", "status", "links"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "columns = [\"id\", \"title\", \"status\", \"links\"]\n") {
		t.Errorf("got\n%s", got)
	}
	cfg, _, err := Check([]byte(got))
	if err != nil || strings.Join(cfg.Views["backlog"].Columns, ",") != "id,title,status,links" {
		t.Fatalf("reparsed %+v %v", cfg.Views["backlog"], err)
	}
	// rewritten in place, then removed when back to the default
	again, _ := SetViewText(got, "backlog", View{Layout: "table", Filter: "is:open", Columns: []string{"title", "prio"}})
	if strings.Count(again, "columns =") != 1 || !strings.Contains(again, "columns = [\"title\", \"prio\"]") {
		t.Errorf("rewrite\n%s", again)
	}
	none, _ := SetViewText(again, "backlog", View{Layout: "table", Filter: "is:open"})
	if strings.Contains(none, "columns") {
		t.Errorf("default columns kept a line\n%s", none)
	}
}

// A board per role: the lanes it shows, in order, and their limits.
func TestSetViewTextWritesLanesAndLimits(t *testing.T) {
	src := "[views.dev]\nlayout = \"board\" # devs\ngroup = \"status\"\n"
	v := View{Layout: "board", Group: "status", Filter: "is:open",
		Lanes:  []string{"ready", "in-progress", "in-review", "ready-for-validation"},
		Limits: map[string]int{"in-progress": 5, "in review": 3}}
	got, err := SetViewText(src, "dev", v)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`layout = "board" # devs`,
		`lanes = ["ready", "in-progress", "in-review", "ready-for-validation"]`,
		`limits = { "in review" = 3, in-progress = 5 }`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in\n%s", want, got)
		}
	}
	cfg, _, err := Check([]byte(got))
	if err != nil {
		t.Fatal(err)
	}
	d := cfg.Views["dev"]
	if strings.Join(d.Lanes, ",") != "ready,in-progress,in-review,ready-for-validation" || d.Limits["in-progress"] != 5 || d.Limits["in review"] != 3 {
		t.Errorf("reparsed %+v", d)
	}
	// limits rewritten in place, then gone with the lanes
	again, _ := SetViewText(got, "dev", View{Layout: "board", Group: "status", Filter: "is:open", Lanes: []string{"ready"}, Limits: map[string]int{"ready": 2}})
	if strings.Count(again, "limits =") != 1 || !strings.Contains(again, "limits = { ready = 2 }") {
		t.Errorf("rewrite\n%s", again)
	}
	none, _ := SetViewText(again, "dev", View{Layout: "board", Group: "status", Filter: "is:open"})
	if strings.Contains(none, "lanes") || strings.Contains(none, "limits") {
		t.Errorf("no lanes kept a line\n%s", none)
	}
}

func TestSetLabelsTable(t *testing.T) {
	t.Parallel()
	text := "[labels]\nlist = [\"needs-design\"]\n\n[views.board]\nlayout = \"board\"\n"
	got := SetLabelsTable(text, "colors", map[string]string{"needs-design": "#d73a4a"})
	want := "[labels]\nlist = [\"needs-design\"]\n\n[labels.colors]\nneeds-design = \"#d73a4a\"\n\n[views.board]\nlayout = \"board\"\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if again := SetLabelsTable(got, "colors", nil); again != text {
		t.Errorf("removed: got\n%q\nwant\n%q", again, text)
	}
}
