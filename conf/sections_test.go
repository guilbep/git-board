package conf

import "testing"

// A new table joins its family; an existing one is SetKeys's.
func TestSetKeysAfterJoinsTheFamily(t *testing.T) {
	t.Parallel()
	text := "[fields.a]\nsingle = true\n\n[fields.a.colors]\nx = \"red\"\n\n# views\n[views.all]\nlayout = \"list\"\n"
	got := SetKeysAfter(text, "fields.b", LastTable(text, "fields"), []string{"single", "options"}, map[string]string{"single": "true", "options": List([]string{"p", "q"})})
	want := "[fields.a]\nsingle = true\n\n[fields.a.colors]\nx = \"red\"\n\n[fields.b]\nsingle = true\noptions = [\"p\", \"q\"]\n\n# views\n[views.all]\nlayout = \"list\"\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if got := SetKeysAfter(want, "fields.a", "fields.b", []string{"single"}, map[string]string{"single": ""}); got != SetKeys(want, "fields.a", []string{"single"}, map[string]string{"single": ""}) {
		t.Errorf("an existing table: %s", got)
	}
	if got := SetKeysAfter("", "fields.a", "", []string{"single"}, map[string]string{"single": "true"}); got != "[fields.a]\nsingle = true\n" {
		t.Errorf("no anchor: %q", got)
	}
}

// A sub-table is written whole, after its parent; an inline one goes.
func TestSetSubTable(t *testing.T) {
	t.Parallel()
	text := "[fields.m]\noptions = [\"Wont\"]\nproject = { Wont = \"Won't\" }\n\n[views.all]\n"
	got := SetSubTable(text, "fields.m", "project", map[string]string{"Wont": "Won't", "a b": "x"})
	want := "[fields.m]\noptions = [\"Wont\"]\n\n[fields.m.project]\nWont = \"Won't\"\n\"a b\" = \"x\"\n\n[views.all]\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if got := SetSubTable(got, "fields.m", "project", nil); got != "[fields.m]\noptions = [\"Wont\"]\n\n[views.all]\n" {
		t.Errorf("emptied: %q", got)
	}
}

// A table is replaced whole, the next table's comments kept; a missing
// one appended.
func TestReplaceTable(t *testing.T) {
	t.Parallel()
	text := "a = 1\n\n[github-project]\nowner = \"x\"\nnumber = 1\nwrite = true\n\n# uploads\n[uploads]\n"
	block := "[github-project]\nowner = \"y\"\nnumber = 2\n"
	if got, want := ReplaceTable(text, "github-project", block), "a = 1\n\n"+block+"\n# uploads\n[uploads]\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := ReplaceTable("a = 1\n", "github-project", block), "a = 1\n\n"+block; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
