package board_test

import (
	"slices"
	"testing"

	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

// Issues take the project's roles unless they set their own: filters,
// groups and facets see the inherited value, the PM's board pm:@me too.
func TestRolesInherit(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	b.NewIssue("inherits", "", nil)
	b.NewIssue("overrides", "", []string{"lead-dev:dan"})
	cfg := &config.Config{Roles: map[string]string{"lead-dev": "bob", "pm": "claire"}}
	titles := func(filter, me string) []string {
		t.Helper()
		issues, err := board.LoadMatching(b, cfg, filter, me)
		if err != nil {
			t.Fatalf("%q: %v", filter, err)
		}
		var out []string
		for _, is := range issues {
			out = append(out, is.Title)
		}
		slices.Sort(out)
		return out
	}
	for filter, want := range map[string][]string{
		"lead-dev:@me":  {"inherits"},
		"lead-dev:dan":  {"overrides"},
		"-lead-dev:bob": {"overrides"},
		"pm:@claire":    nil,
		"no:lead-dev":   nil,
	} {
		if got := titles(filter, "bob"); !slices.Equal(got, want) {
			t.Errorf("%q: %v, want %v", filter, got, want)
		}
	}
	if got := titles("pm:@me", "claire"); !slices.Equal(got, []string{"inherits", "overrides"}) {
		t.Errorf("the PM's board, pm:@me: %v", got)
	}

	all, _ := board.LoadMatching(b, cfg, "", "bob")
	groups := map[string]int{}
	for _, bk := range board.BucketBy(all, "lead-dev") {
		groups[bk.Group] = len(bk.Issues)
	}
	if groups["bob"] != 1 || groups["dan"] != 1 {
		t.Errorf("grouped by lead-dev: %v", groups)
	}

	facets, err := board.Facets(b, cfg, "", "bob")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range facets {
		if f.Key == "lead-dev" {
			counts := map[string]int{}
			for _, o := range f.Options {
				counts[o.Value] = o.Count
			}
			if counts["bob"] != 1 || counts["dan"] != 1 {
				t.Errorf("lead-dev facet: %v", counts)
			}
			return
		}
	}
	t.Error("no lead-dev facet")
}
