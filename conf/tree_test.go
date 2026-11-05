package conf

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestSetValue(t *testing.T) {
	t.Parallel()
	const file = `# the board
[fields.status]
single = true # one
options = ["a", "b"]

[fields.size]
options = ["s", "m"]
colors = { s = "green" }

[fields.prio.colors]
p0 = "red"

[views]
x = {filter = "is:open"}

[[sprints.list]]
slug = "s1"
start = 2026-10-05

[cycle]
ready = "ready"
`
	for _, c := range []struct {
		name, key string
		v         any
		has, not  []string
	}{
		{"a table in place, its comment kept, a key dropped", "fields.status",
			map[string]any{"single": true, "type": "select"},
			[]string{"[fields.status]\nsingle = true # one\ntype = \"select\"\n"}, []string{`options = ["a", "b"]`}},
		{"an inline sub-table replaced on its line", "fields.size",
			map[string]any{"type": "issue"},
			[]string{"[fields.size]\ntype = \"issue\"\n"}, []string{`{ s = "green" }`, `"s", "m"`}},
		{"a table removed with its sub-tables", "fields.prio", nil,
			nil, []string{"[fields.prio", "p0"}},
		{"a whole top table removed", "cycle", nil, nil, []string{"[cycle]", "ready"}},
		{"an inline table in its parent", "views.x", map[string]any{"filter": "is:closed", "layout": "board"},
			[]string{`x = {filter = "is:closed", layout = "board"}`}, nil},
		{"an inline table removed, its empty parent too", "views.x", nil, nil, []string{"[views]"}},
		{"a new table", "views.y", map[string]any{"filter": "a"}, []string{"\n[views.y]\nfilter = \"a\"\n"}, nil},
		{"an array of tables, a date kept a date", "sprints.list",
			[]any{map[string]any{"slug": "s1", "title": "One", "start": "2026-10-05"}},
			[]string{"[[sprints.list]]\nslug = \"s1\"\nstart = 2026-10-05\ntitle = \"One\"\n"}, nil},
		{"an empty table", "release", map[string]any{}, []string{"\n[release]\n"}, nil},
	} {
		got, err := SetValue(file, c.key, c.v)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		var m map[string]any
		if _, err := toml.Decode(got, &m); err != nil {
			t.Errorf("%s: the file doesn't parse: %v\n%s", c.name, err, got)
		}
		for _, s := range c.has {
			if !strings.Contains(got, s) {
				t.Errorf("%s: no %q in\n%s", c.name, s, got)
			}
		}
		for _, s := range c.not {
			if strings.Contains(got, s) {
				t.Errorf("%s: %q still in\n%s", c.name, s, got)
			}
		}
		if !strings.HasPrefix(got, "# the board\n") {
			t.Errorf("%s: the head comment went:\n%s", c.name, got)
		}
	}
}
