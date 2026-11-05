package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Settings through .git-board.toml's keys: compare-and-swap on the
// file's version, a dry run that writes nothing, a key git-board doesn't
// read refused, comments kept.
func TestConfig(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	a := newAPI(t, dir)
	path := filepath.Join(dir, ".git-board.toml")
	res := a.do("GET", "/api/config", nil)
	c := want[configRes](t, res, 200)
	if c.SchemaVersion != 1 || c.Text != boardToml || etag(res) != `"`+c.Version+`"` || len(c.Keys) != 2 || len(c.Problems) != 0 {
		t.Fatalf("GET /api/config = %+v", c)
	}
	// a dry run: checked, answered, not written
	dry := want[configRes](t, a.do("PATCH", "/api/config?dry_run=1", map[string]any{"set": map[string]any{"views.mine.group": "status"}}), 200)
	if !strings.Contains(dry.Text, `group = "status"`) || read(t, path) != boardToml {
		t.Fatalf("a dry run wrote, or didn't answer: %s", dry.Text)
	}
	res = a.do("PATCH", "/api/config", map[string]any{"set": map[string]any{"fields.status.options": []string{"todo", "done"}, "views.mine.group": "status"}}, "If-Match", etag(res))
	c2 := want[configRes](t, res, 200)
	if got := read(t, path); !strings.Contains(got, "# the test board") || !strings.Contains(got, `options = ["todo", "done"]`) || got != c2.Text {
		t.Fatalf("PATCH wrote:\n%s", got)
	}
	// from the first version again: stale, nothing written
	e := want[apiError](t, a.do("PATCH", "/api/config", map[string]any{"set": map[string]any{"views.mine.sort": "updated"}}, "If-Match", `"`+c.Version+`"`), 412)
	if e.Code != "stale" || strings.Contains(read(t, path), "updated") {
		t.Fatalf("a stale PATCH = %+v", e)
	}
	e = want[apiError](t, a.do("PATCH", "/api/config", map[string]any{"set": map[string]any{"nope.key": 1}}), 400)
	if e.Code != "invalid" {
		t.Fatalf("a key git-board doesn't read = %+v", e)
	}
	// null removes a key
	c3 := want[configRes](t, a.do("PATCH", "/api/config", map[string]any{"set": map[string]any{"views.mine.group": nil}}), 200)
	if strings.Contains(c3.Text, "group") {
		t.Fatalf("null didn't remove the key:\n%s", c3.Text)
	}
	// the whole text
	e = want[apiError](t, a.do("PUT", "/api/config", map[string]any{"text": "[fields\n"}), 400)
	if e.Field != "text" {
		t.Fatalf("a broken text = %+v", e)
	}
	c4 := want[configRes](t, a.do("PUT", "/api/config", map[string]any{"text": "[views.all]\nfilter = \"\"\n"}, "If-Match", `"`+c3.Version+`"`), 200)
	if read(t, path) != "[views.all]\nfilter = \"\"\n" || c4.Version == c3.Version {
		t.Fatalf("PUT = %+v", c4)
	}
	// the board opened again: its views are the file's
	if e := want[apiError](t, a.do("PUT", "/api/inbox/settings", map[string]any{"views": []string{"mine"}}), 400); e.Field != "views" {
		t.Fatalf("an old view = %+v", e)
	}
	want[map[string]any](t, a.do("PUT", "/api/inbox/settings", map[string]any{"views": []string{"all"}}), http.StatusOK)
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// The forge-mapping migration's route: its preview per declared forge
// (none here), a field nothing configures refused.
func TestMigrateFieldRoute(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	a := newAPI(t, dir)
	m := want[migrateRes](t, a.do("POST", "/api/fields/status/migrate?dry_run=1", map[string]any{}), 200)
	if m.Plans == nil || len(m.Plans) != 0 || m.Error != "" {
		t.Fatalf("preview = %+v", m)
	}
	if e := want[apiError](t, a.do("POST", "/api/fields/nosuch/migrate?dry_run=1", map[string]any{}), 400); e.Code != "invalid" {
		t.Fatalf("no such field = %+v", e)
	}
}
