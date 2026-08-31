package filter

import (
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
)

const fixture = "../config/testdata/git-board.toml"

var today = time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC) // "delivery" is current, "post-delivery" is next

func mustLoad(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load(fixture)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}

func TestParseFieldCondition(t *testing.T) {
	c := Parse("status:ready")
	want := Condition{Kind: KindField, Field: "status", Value: "ready"}
	if len(c) != 1 || c[0] != want {
		t.Fatalf("Parse = %+v, want [%+v]", c, want)
	}
}

func TestParseNegatedTitleCondition(t *testing.T) {
	c := Parse("-title:*spec*")
	want := Condition{Kind: KindTitle, Value: "*spec*", Negated: true}
	if len(c) != 1 || c[0] != want {
		t.Fatalf("Parse = %+v, want [%+v]", c, want)
	}
}

func TestParseIsCondition(t *testing.T) {
	c := Parse("is:open")
	want := Condition{Kind: KindIs, Value: "open"}
	if len(c) != 1 || c[0] != want {
		t.Fatalf("Parse = %+v, want [%+v]", c, want)
	}
}

func TestParseFreeText(t *testing.T) {
	c := Parse("bicycle")
	want := Condition{Kind: KindText, Value: "bicycle"}
	if len(c) != 1 || c[0] != want {
		t.Fatalf("Parse = %+v, want [%+v]", c, want)
	}
}

func TestParseMultipleConditionsAreAnded(t *testing.T) {
	c := Parse("sprint:@current is:open -title:*spec*")
	if len(c) != 3 {
		t.Fatalf("len(Parse(...)) = %d, want 3", len(c))
	}
	kinds := []Kind{c[0].Kind, c[1].Kind, c[2].Kind}
	if kinds[0] != KindField || kinds[1] != KindIs || kinds[2] != KindTitle {
		t.Fatalf("kinds = %v", kinds)
	}
	if !c[2].Negated {
		t.Error("c[2].Negated = false, want true")
	}
}

func TestMatchesFieldIsAndTitleGlob(t *testing.T) {
	cfg := mustLoad(t)
	issue := gitbugio.Issue{ID: "abc123", Title: "Fix invoice spec", Status: "open", Labels: []string{"status:ready"}}

	if !Matches(issue, Parse("status:ready is:open title:*spec*"), cfg, "", today) {
		t.Error("expected match")
	}
	if Matches(issue, Parse("status:backlog"), cfg, "", today) {
		t.Error("expected no match")
	}
}

func TestMatchesNegatedTitleGlobExcludes(t *testing.T) {
	cfg := mustLoad(t)
	issue := gitbugio.Issue{ID: "abc123", Title: "Fix invoice spec", Status: "open"}

	if Matches(issue, Parse("-title:*spec*"), cfg, "", today) {
		t.Error("expected exclusion")
	}
}

func TestMatchesFreeTextSubstring(t *testing.T) {
	cfg := mustLoad(t)
	issue := gitbugio.Issue{ID: "abc123", Title: "Fix invoice bug", Status: "open"}

	if !Matches(issue, Parse("invoice"), cfg, "", today) {
		t.Error("expected match")
	}
	if Matches(issue, Parse("purchase"), cfg, "", today) {
		t.Error("expected no match")
	}
}

func TestMatchesAtMe(t *testing.T) {
	cfg := mustLoad(t)
	issue := gitbugio.Issue{ID: "abc123", Title: "x", Status: "open", Labels: []string{"assignee:guilbep"}}
	conditions := Parse("assignee:@me")

	if !Matches(issue, conditions, cfg, "guilbep", today) {
		t.Error("expected match for me=guilbep")
	}
	if Matches(issue, conditions, cfg, "someone-else", today) {
		t.Error("expected no match for me=someone-else")
	}
}

func TestMatchesSprintCurrentAndNext(t *testing.T) {
	cfg := mustLoad(t)
	current := gitbugio.Issue{ID: "1", Title: "x", Status: "open", Labels: []string{"sprint:delivery"}}
	next := gitbugio.Issue{ID: "2", Title: "x", Status: "open", Labels: []string{"sprint:post-delivery"}}

	if !Matches(current, Parse("sprint:@current"), cfg, "", today) {
		t.Error("current issue should match sprint:@current")
	}
	if Matches(next, Parse("sprint:@current"), cfg, "", today) {
		t.Error("next issue should not match sprint:@current")
	}
	if !Matches(next, Parse("sprint:@next"), cfg, "", today) {
		t.Error("next issue should match sprint:@next")
	}
	if Matches(current, Parse("sprint:@previous"), cfg, "", today) {
		t.Error("current issue should not match sprint:@previous (no previous sprint)")
	}
}

func TestRequiresMe(t *testing.T) {
	if !RequiresMe(Parse("assignee:@me is:open")) {
		t.Error("expected RequiresMe to be true")
	}
	if RequiresMe(Parse("status:ready is:open")) {
		t.Error("expected RequiresMe to be false")
	}
}
