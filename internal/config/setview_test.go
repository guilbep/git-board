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
