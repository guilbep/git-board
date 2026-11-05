package httpapi

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// The Work page pages as the lists do: limit gives each group's first
// rows, its total and a cursor; group and cursor, a group's next page.
// The pages put together are the rows unpaged.
func TestWorkPages(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	on := boardToml + "\n[experimental]\nfeatures = [\"cycle\"]\n"
	if err := os.WriteFile(filepath.Join(dir, ".git-board.toml"), []byte(on), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newAPI(t, dir)
	for i := range 5 {
		want[map[string]any](t, a.do("POST", "/api/issues", map[string]any{"title": fmt.Sprintf("Work %d", i)}), 201)
	}
	type row struct{ Issue struct{ ID string } }
	type page struct {
		Needs, Waiting []row
		NeedsTotal     *int `json:"needs_total"`
		WaitingTotal   *int `json:"waiting_total"`
	}
	all := want[page](t, a.do("GET", "/api/cycle?who=team", nil), 200)
	if all.NeedsTotal != nil || all.WaitingTotal != nil || len(all.Needs) < 6 {
		t.Fatalf("unpaged = %+v: want every row, no total", all)
	}
	var got []string
	first := want[map[string]any](t, a.do("GET", "/api/cycle?who=team&limit=2", nil), 200)
	if first["needs_total"] != float64(len(all.Needs)) || len(first["needs"].([]any)) != 2 || first["needs_next"] == nil {
		t.Fatalf("limit=2 = %v", first)
	}
	for _, r := range first["needs"].([]any) {
		got = append(got, r.(map[string]any)["issue"].(map[string]any)["id"].(string))
	}
	next := first["needs_next"].(string)
	for next != "" {
		p := want[map[string]any](t, a.do("GET", "/api/cycle?who=team&limit=2&group=needs&cursor="+url.QueryEscape(next), nil), 200)
		if len(p["waiting"].([]any)) != 0 || p["waiting_total"] != nil {
			t.Fatalf("a page of needs = %v: want no waiting", p)
		}
		for _, r := range p["needs"].([]any) {
			got = append(got, r.(map[string]any)["issue"].(map[string]any)["id"].(string))
		}
		next, _ = p["needs_next"].(string)
	}
	if len(got) != len(all.Needs) {
		t.Fatalf("the pages hold %d rows, unpaged %d", len(got), len(all.Needs))
	}
	for i, r := range all.Needs {
		if got[i] != r.Issue.ID {
			t.Fatalf("row %d: pages %s, unpaged %s", i, got[i], r.Issue.ID)
		}
	}
	for _, c := range []struct{ query, field string }{
		{"limit=0", "limit"},
		{"limit=1001", "limit"},
		{"group=waiting", "limit"},
		{"limit=2&cursor=x", "group"},
		{"limit=2&group=all", "group"},
		{"limit=2&group=waiting&cursor=x", "cursor"},
	} {
		if e := want[apiError](t, a.do("GET", "/api/cycle?who=team&"+c.query, nil), 400); e.Field != c.field {
			t.Errorf("%s = %+v, want an error on %s", c.query, e, c.field)
		}
	}
}
