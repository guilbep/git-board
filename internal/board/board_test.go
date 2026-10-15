package board

import (
	"slices"
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
)

func titles(issues []gitbugio.Issue) []string {
	out := make([]string, len(issues))
	for i, issue := range issues {
		out[i] = issue.Title
	}
	return out
}

func TestSortByFieldLexicalByDefault(t *testing.T) {
	issues := []gitbugio.Issue{
		{Title: "p2", Labels: []string{"prio:p2"}},
		{Title: "p0", Labels: []string{"prio:p0"}},
		{Title: "p1", Labels: []string{"prio:p1"}},
	}
	SortByField(issues, "prio", &config.Config{})
	if got := titles(issues); !slices.Equal(got, []string{"p0", "p1", "p2"}) {
		t.Errorf("order = %v, want lexical p0,p1,p2", got)
	}
}

func TestSortByFieldNumericOrdersByValueNotLexically(t *testing.T) {
	// Lexical order of these est values would be "1", "16", "2" -- the
	// whole reason a numeric field needs different handling.
	issues := []gitbugio.Issue{
		{Title: "sixteen", Labels: []string{"est:16"}},
		{Title: "two", Labels: []string{"est:2"}},
		{Title: "one", Labels: []string{"est:1"}},
	}
	cfg := &config.Config{Fields: map[string]config.Field{"est": {Numeric: true}}}

	SortByField(issues, "est", cfg)
	if got := titles(issues); !slices.Equal(got, []string{"one", "two", "sixteen"}) {
		t.Errorf("ascending order = %v, want one(1),two(2),sixteen(16)", got)
	}

	SortByField(issues, "est desc", cfg)
	if got := titles(issues); !slices.Equal(got, []string{"sixteen", "two", "one"}) {
		t.Errorf("descending order = %v, want sixteen(16),two(2),one(1)", got)
	}
}

func TestSortByFieldNumericMissingValueSortsLastBothDirections(t *testing.T) {
	issues := []gitbugio.Issue{
		{Title: "unset"},
		{Title: "two", Labels: []string{"est:2"}},
		{Title: "one", Labels: []string{"est:1"}},
	}
	cfg := &config.Config{Fields: map[string]config.Field{"est": {Numeric: true}}}

	SortByField(issues, "est", cfg)
	if got := titles(issues); got[len(got)-1] != "unset" {
		t.Errorf("ascending order = %v, want the unset issue last, not swapped to the front", got)
	}

	SortByField(issues, "est desc", cfg)
	if got := titles(issues); got[len(got)-1] != "unset" {
		t.Errorf("descending order = %v, want the unset issue still last, not swapped to the front", got)
	}
}

func TestSortByFieldEmptySpecIsNoOp(t *testing.T) {
	issues := []gitbugio.Issue{
		{Title: "b", Labels: []string{"prio:p2"}},
		{Title: "a", Labels: []string{"prio:p1"}},
	}
	SortByField(issues, "", &config.Config{})
	if got := titles(issues); !slices.Equal(got, []string{"b", "a"}) {
		t.Errorf("order = %v, want unchanged (empty spec is a no-op)", got)
	}
}

func TestSortByFieldNilConfigDoesNotPanic(t *testing.T) {
	issues := []gitbugio.Issue{{Title: "x", Labels: []string{"est:2"}}}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SortByField with a nil config panicked: %v", r)
		}
	}()
	SortByField(issues, "est", nil)
}

// A field with options sorts in their order (status follows the workflow,
// not the alphabet); values outside them after, missing values last.
func TestSortByFieldFollowsOptionOrder(t *testing.T) {
	cfg := &config.Config{Fields: map[string]config.Field{
		"status": {Single: true, Options: []string{"backlog", "ready", "in-progress"}},
	}}
	issues := func() []gitbugio.Issue {
		return []gitbugio.Issue{
			{Title: "none"},
			{Title: "in-progress", Labels: []string{"status:in-progress"}},
			{Title: "zzz", Labels: []string{"status:zzz"}},
			{Title: "backlog", Labels: []string{"status:backlog"}},
			{Title: "ready", Labels: []string{"status:ready"}},
		}
	}
	asc := issues()
	SortByField(asc, "status", cfg)
	if got := titles(asc); !slices.Equal(got, []string{"backlog", "ready", "in-progress", "zzz", "none"}) {
		t.Errorf("asc = %v", got)
	}
	desc := issues()
	SortByField(desc, "status desc", cfg)
	if got := titles(desc); !slices.Equal(got, []string{"zzz", "in-progress", "ready", "backlog", "none"}) {
		t.Errorf("desc = %v", got)
	}
}
