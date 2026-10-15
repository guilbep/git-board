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
