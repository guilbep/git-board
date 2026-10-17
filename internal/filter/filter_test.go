package filter

import (
	"reflect"
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
	if len(c) != 1 || !reflect.DeepEqual(c[0], want) {
		t.Fatalf("Parse = %+v, want [%+v]", c, want)
	}
}

func TestParseNegatedTitleCondition(t *testing.T) {
	c := Parse("-title:*spec*")
	want := Condition{Kind: KindTitle, Value: "*spec*", Negated: true}
	if len(c) != 1 || !reflect.DeepEqual(c[0], want) {
		t.Fatalf("Parse = %+v, want [%+v]", c, want)
	}
}

func TestParseIsCondition(t *testing.T) {
	c := Parse("is:open")
	want := Condition{Kind: KindIs, Value: "open"}
	if len(c) != 1 || !reflect.DeepEqual(c[0], want) {
		t.Fatalf("Parse = %+v, want [%+v]", c, want)
	}
}

func TestParseFreeText(t *testing.T) {
	c := Parse("bicycle")
	want := Condition{Kind: KindText, Value: "bicycle"}
	if len(c) != 1 || !reflect.DeepEqual(c[0], want) {
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

// GitHub-style search: quoted phrases, author:, label:, no:, free text
// matched through the full-text hits a caller fills in.
func TestGitHubStyleSearch(t *testing.T) {
	cfg := mustLoad(t)
	today := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	issue := gitbugio.Issue{ID: "abc", Title: "Train distances", Status: "open",
		Labels: []string{"assignee:guilbep", "status:ready", "bug"}, AuthorLogins: []string{"guilbep", "Pierre Guilbert"}}
	bare := gitbugio.Issue{ID: "def", Title: "Other", Status: "open"}

	conds := Parse(`"wrong table" assignee:guilbep`)
	if len(conds) != 2 || conds[0].Kind != KindText || conds[0].Value != "wrong table" {
		t.Fatalf("quoted phrase: %+v", conds)
	}
	for expr, want := range map[string][2]bool{ // [issue, bare]
		"author:guilbep":           {true, false},
		"author:@me":               {true, false},
		"label:bug":                {true, false},
		"label:status:ready":       {true, false},
		"-label:bug":               {false, true},
		"no:assignee":              {false, true},
		"no:label":                 {false, true},
		"no:prio":                  {true, true},
		"assignee:guilbep is:open": {true, false},
	} {
		c := Parse(expr)
		got := [2]bool{Matches(issue, c, cfg, "guilbep", today), Matches(bare, c, cfg, "guilbep", today)}
		if got != want {
			t.Errorf("%q: %v, want %v", expr, got, want)
		}
	}
	if !RequiresMe(Parse("author:@me")) {
		t.Error("author:@me doesn't ask for @me")
	}

	// free text: the hits of the full-text index (title, description, comments)
	c := Parse("distances")
	c[0].Hits = map[string]bool{"def": true}
	if Matches(issue, c, cfg, "", today) || !Matches(bare, c, cfg, "", today) {
		t.Error("free text didn't follow the search hits")
	}
}

// a,b is either, as in GitHub's search (label:bug,wip): what a facet with
// several values ticked saves into a view.
func TestMatchesCommaIsEither(t *testing.T) {
	cfg := mustLoad(t)
	qa := gitbugio.Issue{ID: "a", Status: "open", Labels: []string{"status:qa", "bug", "assignee:guilbep"}}
	backlog := gitbugio.Issue{ID: "b", Status: "closed", Labels: []string{"status:backlog"}}
	for _, c := range []struct {
		filter   string
		qa, back bool
	}{
		{"status:ready,qa", true, false},
		{"-status:ready,qa", false, true},
		{"status:qa,backlog", true, true},
		{"is:open,closed", true, true},
		{"label:wip,bug", true, false},
		{"assignee:@me,someone", true, false},
	} {
		conds := Parse(c.filter)
		if got := Matches(qa, conds, cfg, "guilbep", today); got != c.qa {
			t.Errorf("%s on the qa issue: %v", c.filter, got)
		}
		if got := Matches(backlog, conds, cfg, "guilbep", today); got != c.back {
			t.Errorf("%s on the backlog issue: %v", c.filter, got)
		}
	}
	if !RequiresMe(Parse("assignee:someone,@me")) {
		t.Error("@me inside a list isn't seen")
	}
}

// An issue by its number (#2528) or the start of its id, as the command
// palette jumps to it: #2528 is the number only; a bare number or 7 and
// more hex characters also look in the text.
func TestMatchesNumberAndIDPrefix(t *testing.T) {
	cfg := mustLoad(t)
	n2528 := gitbugio.Issue{ID: "b9de9e14aa", Title: "Fix CSV upload", Status: "open", Forges: []gitbugio.Forge{{Number: "2528"}}}
	mentions := gitbugio.Issue{ID: "5a3b1c22bb", Title: "Follow-up of 2528", Status: "open", Forges: []gitbugio.Forge{{Number: "2115"}}}
	if c := Parse("#2528"); len(c) != 1 || c[0].Kind != KindNumber || c[0].Value != "2528" {
		t.Fatalf("Parse(#2528) = %+v", c)
	}
	for expr, want := range map[string][2]bool{ // [n2528, mentions]
		"#2528":    {true, false},
		"-#2528":   {false, true},
		"2528":     {true, true}, // the number, or the text
		"-2528":    {false, false},
		"#25":      {false, false},
		"b9de9e1":  {true, false},
		"B9DE9E1":  {true, false},
		"b9de9e":   {false, false}, // under 7: a word only
		"5a3b1c22": {false, true},
	} {
		c := Parse(expr)
		got := [2]bool{Matches(n2528, c, cfg, "", today), Matches(mentions, c, cfg, "", today)}
		if got != want {
			t.Errorf("%q: %v, want %v", expr, got, want)
		}
	}
	// with the full-text index's hits: still the number
	c := Parse("2528")
	c[0].Hits = map[string]bool{mentions.ID: true}
	if !Matches(n2528, c, cfg, "", today) || !Matches(mentions, c, cfg, "", today) {
		t.Error("2528 with hits: the number or the hits")
	}
}

// Without drops terms as typed, quotes kept: a facet rewrites its own
// terms after the rest.
func TestWithoutKeepsTheOtherTermsAsTyped(t *testing.T) {
	isStatus := func(c Condition) bool {
		return c.Kind == KindField && c.Field == "status" || c.Kind == KindNo && c.Value == "status"
	}
	for text, want := range map[string]string{
		`is:open status:ready label:"nice to have" "car sharing"`: `is:open label:"nice to have" "car sharing"`,
		`-status:validated   no:status prio:p1`:                   `prio:p1`,
		`status:ready`:                                            ``,
		``:                                                        ``,
	} {
		if got := Without(text, isStatus); got != want {
			t.Errorf("Without(%q) = %q, want %q", text, got, want)
		}
	}
	if got := Parse(`label:"nice to have"`); len(got) != 1 || got[0].Value != "nice to have" {
		t.Errorf("quoted value: %+v", got)
	}
}
