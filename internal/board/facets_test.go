package board_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
	"github.com/guilbep/git-board/internal/inbox"
)

// The filters panel: each field's values counted against the rest of the
// filter, the values the filter picks, and the filter without the field's
// own terms, which a tick writes a new term after.
func TestFacetsCountAgainstTheRestOfTheFilter(t *testing.T) {
	b := gitbugiotest.New(t)
	b.NewIssue("a", "", []string{"status:ready", "assignee:guilbep", "nice to have"})
	b.NewIssue("b", "", []string{"status:qa", "sprint:s1"})
	closed, _ := b.NewIssue("c", "", []string{"status:ready"})
	gitbugiotest.CloseIssue(t, b, closed)
	b.NewIssue("d", "", nil)
	cfg := &config.Config{
		Fields:  map[string]config.Field{"status": {Options: []string{"ready", "qa", "validated"}}},
		Sprints: config.SprintsConfig{LengthDays: 14, List: []config.Sprint{{Slug: "s1", Start: time.Now().AddDate(0, 0, -20)}, {Slug: "s2", Start: time.Now().AddDate(0, 0, -6)}}},
	}

	facets := func(filter string) map[string]board.Facet {
		t.Helper()
		fs, err := board.Facets(b, cfg, filter, "guilbep")
		if err != nil {
			t.Fatalf("%q: %v", filter, err)
		}
		m := map[string]board.Facet{}
		for _, f := range fs {
			m[f.Key] = f
		}
		return m
	}
	counts := func(f board.Facet) map[string]int {
		m := map[string]int{}
		for _, o := range f.Options {
			m[o.Value] = o.Count
		}
		return m
	}

	f := facets("is:open")
	if got, want := counts(f["is"]), map[string]int{"open": 3, "closed": 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("is: %v, want %v", got, want)
	}
	if !reflect.DeepEqual(f["is"].Selected, []string{"open"}) || f["is"].Rest != "" {
		t.Errorf("is: selected %v, rest %q", f["is"].Selected, f["is"].Rest)
	}
	if got, want := counts(f["status"]), map[string]int{"ready": 1, "qa": 1, "(none)": 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("status: %v, want %v", got, want)
	}
	if len(f["status"].Selected) != 0 || f["status"].Rest != "is:open" {
		t.Errorf("status: selected %v, rest %q", f["status"].Selected, f["status"].Rest)
	}

	// several terms on one field; a quoted value kept as typed in the rest
	f = facets(`is:open -status:validated status:ready,qa label:"nice to have"`)
	if got := f["status"]; got.Rest != `is:open label:"nice to have"` || !reflect.DeepEqual(counts(got), map[string]int{"ready": 1, "qa": 0, "validated": 0}) {
		t.Errorf("status: rest %q, counts %v", got.Rest, counts(got))
	}
	if !sameSet(f["status"].Selected, "ready", "qa") {
		t.Errorf("status selected: %v", f["status"].Selected)
	}
	if got := f["label"]; got.Rest != "is:open -status:validated status:ready,qa" || !reflect.DeepEqual(got.Selected, []string{"nice to have"}) {
		t.Errorf("label: rest %q, selected %v", got.Rest, got.Selected)
	}

	// all but one, not set, @me
	f = facets("-status:qa")
	if !sameSet(f["status"].Selected, "ready", "(none)") {
		t.Errorf("-status:qa selected %v", f["status"].Selected)
	}
	f = facets("no:status")
	if !sameSet(f["status"].Selected, "(none)") {
		t.Errorf("no:status selected %v", f["status"].Selected)
	}
	// an alias names its value: today's sprint, ticked even with no issue in it
	f = facets("sprint:@current")
	if !sameSet(f["sprint"].Selected, "s2") || counts(f["sprint"])["s2"] != 0 {
		t.Errorf("sprint:@current: selected %v, counts %v", f["sprint"].Selected, counts(f["sprint"]))
	}
	f = facets("assignee:@me")
	if !sameSet(f["assignee"].Selected, "guilbep") || f["assignee"].Rest != "" {
		t.Errorf("assignee:@me: selected %v, rest %q", f["assignee"].Selected, f["assignee"].Rest)
	}
}

func sameSet(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for _, w := range want {
		found := false
		for _, g := range got {
			found = found || g == w
		}
		if !found {
			return false
		}
	}
	return true
}

// The filters panel counts follows:@me as the list does: an issue you
// unfollowed is left out of the counts too.
func TestFacetsHonourFollows(t *testing.T) {
	b := gitbugiotest.New(t)
	if _, err := b.NewIssue("Kept", "", []string{"assignee:guilbep", "status:ready"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.NewIssue("Also kept", "", []string{"assignee:guilbep", "status:qa"}); err != nil {
		t.Fatal(err)
	}
	dropped, _ := b.NewIssue("Dropped", "", []string{"assignee:guilbep", "status:blocked"})
	me, _, _ := b.Me()
	if err := inbox.Save(b.Storage(), me.ID, inbox.State{Unfollow: []string{dropped}}); err != nil {
		t.Fatal(err)
	}
	facets, err := board.Facets(b, &config.Config{}, "follows:@me", "guilbep")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range facets {
		if f.Key != "status" {
			continue
		}
		for _, o := range f.Options {
			if o.Value == "blocked" {
				t.Errorf("status:blocked counted %d, but you unfollowed its only issue", o.Count)
			}
		}
		return
	}
	t.Fatalf("no status facet in %+v", facets)
}
