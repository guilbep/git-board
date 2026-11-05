package issues

// The filter's tests, from v1's core/filter, owned here with it.

import (
	"reflect"
	"slices"
	"testing"
	"time"
)

// From v1's core/filter/filter_test.go.

var today = time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC) // "delivery" is current, "post-delivery" is next

// co2 is internal/config/testdata/git-board.toml's schema.
var co2 = &Schema{
	Fields: map[string]Field{
		"status":   {Single: true, Options: []string{"backlog", "comite", "ready", "in-progress", "in-review", "ready-for-validation", "qa", "tests-metier", "validated"}},
		"prio":     {Single: true, Options: []string{"p0", "p1", "p2", "p3"}},
		"est":      {Single: true, Options: []string{"0.5", "1", "2", "3", "4", "6", "8", "16"}, Numeric: true},
		"assignee": {Single: false},
		"cat":      {Single: true, Options: []string{"bug", "feature", "chore"}},
	},
	Sprints: Sprints{LengthDays: 14, List: []Sprint{
		{Slug: "delivery", Title: "Delivery", Start: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)},
		{Slug: "post-delivery", Title: "Post-delivery", Start: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)},
	}},
}

func TestParseFieldCondition(t *testing.T) {
	t.Parallel()
	c := ParseFilter("status:ready")
	want := Condition{Kind: QField, Field: "status", Value: "ready"}
	if len(c) != 1 || !reflect.DeepEqual(c[0], want) {
		t.Fatalf("Parse = %+v, want [%+v]", c, want)
	}
}

func TestParseNegatedTitleCondition(t *testing.T) {
	t.Parallel()
	c := ParseFilter("-title:*spec*")
	want := Condition{Kind: QTitle, Value: "*spec*", Negated: true}
	if len(c) != 1 || !reflect.DeepEqual(c[0], want) {
		t.Fatalf("Parse = %+v, want [%+v]", c, want)
	}
}

func TestParseIsCondition(t *testing.T) {
	t.Parallel()
	c := ParseFilter("is:open")
	want := Condition{Kind: QIs, Value: "open"}
	if len(c) != 1 || !reflect.DeepEqual(c[0], want) {
		t.Fatalf("Parse = %+v, want [%+v]", c, want)
	}
}

func TestParseFreeText(t *testing.T) {
	t.Parallel()
	c := ParseFilter("bicycle")
	want := Condition{Kind: QText, Value: "bicycle"}
	if len(c) != 1 || !reflect.DeepEqual(c[0], want) {
		t.Fatalf("Parse = %+v, want [%+v]", c, want)
	}
}

func TestParseMultipleConditionsAreAnded(t *testing.T) {
	t.Parallel()
	c := ParseFilter("sprint:@current is:open -title:*spec*")
	if len(c) != 3 {
		t.Fatalf("len(ParseFilter(...)) = %d, want 3", len(c))
	}
	kinds := []Qualifier{c[0].Kind, c[1].Kind, c[2].Kind}
	if kinds[0] != QField || kinds[1] != QIs || kinds[2] != QTitle {
		t.Fatalf("kinds = %v", kinds)
	}
	if !c[2].Negated {
		t.Error("c[2].Negated = false, want true")
	}
}

func TestMatchesFieldIsAndTitleGlob(t *testing.T) {
	t.Parallel()
	cfg := co2
	issue := Issue{ID: "abc123", Title: "Fix invoice spec", Status: "open", Labels: []string{"status:ready"}}

	if !Matches(issue, ParseFilter("status:ready is:open title:*spec*"), cfg, "", today) {
		t.Error("expected match")
	}
	if Matches(issue, ParseFilter("status:backlog"), cfg, "", today) {
		t.Error("expected no match")
	}
}

func TestMatchesNegatedTitleGlobExcludes(t *testing.T) {
	t.Parallel()
	cfg := co2
	issue := Issue{ID: "abc123", Title: "Fix invoice spec", Status: "open"}

	if Matches(issue, ParseFilter("-title:*spec*"), cfg, "", today) {
		t.Error("expected exclusion")
	}
}

func TestMatchesFreeTextSubstring(t *testing.T) {
	t.Parallel()
	cfg := co2
	issue := Issue{ID: "abc123", Title: "Fix invoice bug", Status: "open"}

	if !Matches(issue, ParseFilter("invoice"), cfg, "", today) {
		t.Error("expected match")
	}
	if Matches(issue, ParseFilter("purchase"), cfg, "", today) {
		t.Error("expected no match")
	}
}

func TestMatchesAtMe(t *testing.T) {
	t.Parallel()
	cfg := co2
	issue := Issue{ID: "abc123", Title: "x", Status: "open", Labels: []string{"assignee:guilbep"}}
	conditions := ParseFilter("assignee:@me")

	if !Matches(issue, conditions, cfg, "guilbep", today) {
		t.Error("expected match for me=guilbep")
	}
	if Matches(issue, conditions, cfg, "someone-else", today) {
		t.Error("expected no match for me=someone-else")
	}
}

func TestMatchesSprintCurrentAndNext(t *testing.T) {
	t.Parallel()
	cfg := co2
	current := Issue{ID: "1", Title: "x", Status: "open", Labels: []string{"sprint:delivery"}}
	next := Issue{ID: "2", Title: "x", Status: "open", Labels: []string{"sprint:post-delivery"}}

	if !Matches(current, ParseFilter("sprint:@current"), cfg, "", today) {
		t.Error("current issue should match sprint:@current")
	}
	if Matches(next, ParseFilter("sprint:@current"), cfg, "", today) {
		t.Error("next issue should not match sprint:@current")
	}
	if !Matches(next, ParseFilter("sprint:@next"), cfg, "", today) {
		t.Error("next issue should match sprint:@next")
	}
	if Matches(current, ParseFilter("sprint:@previous"), cfg, "", today) {
		t.Error("current issue should not match sprint:@previous (no previous sprint)")
	}
}

func TestRequiresMe(t *testing.T) {
	t.Parallel()
	if !RequiresMe(ParseFilter("assignee:@me is:open")) {
		t.Error("expected RequiresMe to be true")
	}
	if RequiresMe(ParseFilter("status:ready is:open")) {
		t.Error("expected RequiresMe to be false")
	}
}

// GitHub-style search: quoted phrases, author:, label:, no:, free text
// matched through the full-text hits a caller fills in.
func TestGitHubStyleSearch(t *testing.T) {
	t.Parallel()
	cfg := co2
	today := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	issue := Issue{ID: "abc", Title: "Train distances", Status: "open",
		Labels: []string{"assignee:guilbep", "status:ready", "bug"}, AuthorLogins: []string{"guilbep", "Pierre Guilbert"}}
	bare := Issue{ID: "def", Title: "Other", Status: "open"}

	conds := ParseFilter(`"wrong table" assignee:guilbep`)
	if len(conds) != 2 || conds[0].Kind != QText || conds[0].Value != "wrong table" {
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
		c := ParseFilter(expr)
		got := [2]bool{Matches(issue, c, cfg, "guilbep", today), Matches(bare, c, cfg, "guilbep", today)}
		if got != want {
			t.Errorf("%q: %v, want %v", expr, got, want)
		}
	}
	if !RequiresMe(ParseFilter("author:@me")) {
		t.Error("author:@me doesn't ask for @me")
	}

	// free text: the hits of the full-text index (title, description, comments)
	c := ParseFilter("distances")
	c[0].Hits = map[string]bool{"def": true}
	if Matches(issue, c, cfg, "", today) || !Matches(bare, c, cfg, "", today) {
		t.Error("free text didn't follow the search hits")
	}
}

// a,b is either, as in GitHub's search (label:bug,wip): what a facet with
// several values ticked saves into a view.
func TestMatchesCommaIsEither(t *testing.T) {
	t.Parallel()
	cfg := co2
	qa := Issue{ID: "a", Status: "open", Labels: []string{"status:qa", "bug", "assignee:guilbep"}}
	backlog := Issue{ID: "b", Status: "closed", Labels: []string{"status:backlog"}}
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
		conds := ParseFilter(c.filter)
		if got := Matches(qa, conds, cfg, "guilbep", today); got != c.qa {
			t.Errorf("%s on the qa issue: %v", c.filter, got)
		}
		if got := Matches(backlog, conds, cfg, "guilbep", today); got != c.back {
			t.Errorf("%s on the backlog issue: %v", c.filter, got)
		}
	}
	if !RequiresMe(ParseFilter("assignee:someone,@me")) {
		t.Error("@me inside a list isn't seen")
	}
}

// An issue by its number (#2528) or the start of its id, as the command
// palette jumps to it: #2528 is the number only; a bare number or 7 and
// more hex characters also look in the text.
func TestMatchesNumberAndIDPrefix(t *testing.T) {
	t.Parallel()
	cfg := co2
	n2528 := Issue{ID: "b9de9e14aa", Title: "Fix CSV upload", Status: "open", Forges: []Forge{{Number: "2528"}}}
	mentions := Issue{ID: "5a3b1c22bb", Title: "Follow-up of 2528", Status: "open", Forges: []Forge{{Number: "2115"}}}
	if c := ParseFilter("#2528"); len(c) != 1 || c[0].Kind != QNumber || c[0].Value != "2528" {
		t.Fatalf("ParseFilter(#2528) = %+v", c)
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
		c := ParseFilter(expr)
		got := [2]bool{Matches(n2528, c, cfg, "", today), Matches(mentions, c, cfg, "", today)}
		if got != want {
			t.Errorf("%q: %v, want %v", expr, got, want)
		}
	}
	// with the full-text index's hits: still the number
	c := ParseFilter("2528")
	c[0].Hits = map[string]bool{mentions.ID: true}
	if !Matches(n2528, c, cfg, "", today) || !Matches(mentions, c, cfg, "", today) {
		t.Error("2528 with hits: the number or the hits")
	}
}

// Without drops terms as typed, quotes kept: a facet rewrites its own
// terms after the rest.
func TestWithoutKeepsTheOtherTermsAsTyped(t *testing.T) {
	t.Parallel()
	isStatus := func(c Condition) bool {
		return c.Kind == QField && c.Field == "status" || c.Kind == QNo && c.Value == "status"
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
	if got := ParseFilter(`label:"nice to have"`); len(got) != 1 || got[0].Value != "nice to have" {
		t.Errorf("quoted value: %+v", got)
	}
}

// Once the project numbers its issues, a bare number and CO2-3060 are the
// project's number, #3060 GitHub's: X is CO2-3060 (GitHub #3071), Y is
// GitHub #3060 imported after the switch (CO2-3075).
func TestMatchesProjectNumbers(t *testing.T) {
	t.Parallel()
	cfg := co2
	x := Issue{ID: "aaaaaaaaaa", Title: "Bouncer pool", Status: "open", Number: "CO2-3060", Counted: true, Forges: []Forge{{Name: "GitHub", Number: "3071"}}}
	y := Issue{ID: "bbbbbbbbbb", Title: "CSV export", Status: "open", Number: "CO2-3075", Counted: true, Forges: []Forge{{Name: "GitHub", Number: "3060"}}}
	for expr, want := range map[string][2]bool{ // [x, y]
		"3060":     {true, false},
		"CO2-3060": {true, false},
		"co2-3060": {true, false},
		"#3060":    {false, true},
		"#3071":    {true, false},
		"3071":     {false, false}, // a bare number is the project's now
		"CO2-30":   {false, false},
	} {
		c := ParseFilter(expr)
		if got := [2]bool{Matches(x, c, cfg, "", today), Matches(y, c, cfg, "", today)}; got != want {
			t.Errorf("%q: %v, want %v", expr, got, want)
		}
	}
}

// A title glob's * crosses /, and a [ that closes no class is a [: the
// README's -title:*spec* excludes "update ci/cd spec", and -title:*[wip*
// excludes "[wip" titles instead of every title failing a broken pattern.
func TestTitleGlobCrossesSlashesAndReadsALoneBracket(t *testing.T) {
	t.Parallel()
	cfg := &Schema{}
	for _, tt := range []struct {
		title, glob string
		want        bool
	}{
		{"update ci/cd spec", "*spec*", true},
		{"update ci/cd spec", "update*spec", true},
		{"[WIP draft", "*[wip*", true},
		{"done", "*[wip*", false},
		{"Fix bug 7", "fix*?", true},
		{"Fix bug 7", "*[0-9]", true},
		{"Fix bug x", "*[!a-w]", true},
		{"Fix bug b", "*[!a-w]", false},
		{"a.b", "a?b", true},
		{"axb", "a.b", false},
	} {
		got := Matches(Issue{Title: tt.title}, ParseFilter("title:"+tt.glob), cfg, "", today)
		if got != tt.want {
			t.Errorf("title:%s on %q = %v, want %v", tt.glob, tt.title, got, tt.want)
		}
	}
}

// From v1's core/filter/ranges_test.go.

func TestParseQualifiers(t *testing.T) {
	t.Parallel()
	for text, want := range map[string]Condition{
		"created:>2026-10-01": {Kind: QDate, Field: "created", Value: ">2026-10-01"},
		"-updated:@today":     {Kind: QDate, Field: "updated", Value: "@today", Negated: true},
		"closed:a..b":         {Kind: QDate, Field: "closed", Value: "a..b"},
		"comments:>5":         {Kind: QComments, Field: "comments", Value: ">5"},
		"in:title,body":       {Kind: QIn, Field: "in", Value: "title,body"},
	} {
		if c := ParseFilter(text); len(c) != 1 || c[0].Kind != want.Kind || c[0].Field != want.Field || c[0].Value != want.Value || c[0].Negated != want.Negated {
			t.Errorf("ParseFilter(%q) = %+v, want %+v", text, c, want)
		}
	}
}

// A date is a day in today's time zone: 23:30 in Zurich is that day there,
// the day before in UTC.
func TestDateRangeIsTodaysZone(t *testing.T) {
	t.Parallel()
	zurich, err := time.LoadLocation("Europe/Zurich")
	if err != nil {
		t.Skip(err)
	}
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, zurich)
	late := time.Date(2026, 10, 1, 23, 30, 0, 0, zurich).Unix()
	for v, want := range map[string]bool{
		"2026-10-01": true, "2026-10-02": false, ">2026-09-30": true, "<=2026-10-01": true, "<2026-10-01": false,
		"2026-10-01T23:30": true, "2026-10-01T23:31": false, "2026-10-01T21:30:00Z": true,
		"@today-14d": true, "@today-2w": true, "@today-13d": false, ">=@today-1w": false, ">=@today-1m": true, "<@today-0d": true, "2026-09-01..2026-10-01": true,
		"*..*": true, "@today+1d..*": false,
	} {
		r, err := DateRange(v, now)
		if err != nil {
			t.Errorf("%q: %v", v, err)
			continue
		}
		if r.Has(late) != want {
			t.Errorf("%q has 2026-10-01 23:30 Zurich: %v, want %v", v, !want, want)
		}
	}
	for _, v := range []string{"", ">", "2026-10", "10/01/2026", "@today-7", "@yesterday", "2026-10-01..", "2026-10-01T25:00"} {
		if _, err := DateRange(v, now); err == nil {
			t.Errorf("%q: no error", v)
		}
	}
}

func TestCountRange(t *testing.T) {
	t.Parallel()
	for v, want := range map[string][]int64{
		"0": {0}, "5": {5}, ">5": {6, 7}, ">=5": {5, 6, 7}, "<2": {0, 1}, "<=2": {0, 1, 2}, "2..4": {2, 3, 4}, "3..*": {3, 4, 5, 6, 7}, "*..1": {0, 1},
	} {
		r, err := CountRange(v)
		if err != nil {
			t.Fatalf("%q: %v", v, err)
		}
		var got []int64
		for n := range int64(8) {
			if r.Has(n) {
				got = append(got, n)
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("%q: %v, want %v", v, got, want)
		}
	}
	for _, v := range []string{"", "x", "-1", ">=", "1.5", "1..x"} {
		if _, err := CountRange(v); err == nil {
			t.Errorf("%q: no error", v)
		}
	}
}

func TestInNarrowsFreeText(t *testing.T) {
	t.Parallel()
	for text, want := range map[string][]string{
		"crash":                   nil,
		"in:title":                {"title"},
		"in:comments in:title":    {"title", "comments"},
		"in:body,title":           {"title", "body"},
		"-in:title":               {"body", "comments"},
		"-in:title,body,comments": {},
		"in:title -in:title":      {"title", "body", "comments"},
	} {
		if got := TextsIn(ParseFilter(text)); !slices.Equal(got, want) || (got == nil) != (want == nil) {
			t.Errorf("TextsIn(%q) = %#v, want %#v", text, got, want)
		}
	}
	is := Issue{Title: "Docs", Body: "a café", CommentBodies: []string{"crash here", "and there"}}
	for text, want := range map[string]bool{
		"cafe": true, "cafe in:title": false, "cafe in:body": true, "crash-here": true, "here and": true, "there-crash": false,
		"cras in:comments": true, "rash": false, "docs -in:title": false,
	} {
		if got := Matches(is, ParseFilter(text), nil, "", today); got != want {
			t.Errorf("%q: %v, want %v", text, got, want)
		}
	}
}

// From v1's core/filter/involves_test.go.

func TestInvolvement(t *testing.T) {
	t.Parallel()
	cfg := &Schema{}
	in := Involved{
		AuthorLogins:    []string{"Ann Example", "ann"},
		Labels:          []string{"assignee:guilbep", "reviewer:GUILBEP", "status:guilbep", "bug"},
		CommenterLogins: []string{"srossi", "guilbep"},
		Mentioned:       true,
	}
	// status isn't a person field: a value that happens to be a login
	if got, want := Involvement(in, "guilbep", "", cfg), []string{"assignee", "reviewer", "commented", "mentioned"}; !slices.Equal(got, want) {
		t.Errorf("guilbep: %v, want %v", got, want)
	}
	if got := Involvement(Involved{AuthorLogins: in.AuthorLogins}, "ANN", "", cfg); !slices.Equal(got, []string{"author"}) {
		t.Errorf("ann: %v, want [author]", got)
	}
	nobody := in
	nobody.Mentioned = false // Mentioned is the asked login's: nothing mentions nobody
	if got := Involvement(nobody, "nobody", "", cfg); got != nil {
		t.Errorf("nobody: %v, want none", got)
	}
	if got := Involvement(in, "", "", cfg); got != nil {
		t.Errorf("no login: %v, want none", got)
	}
	if got := Involvement(in, "guilbep", "", nil); len(got) == 0 {
		t.Error("a nil config still knows assignee by its name")
	}
}

func TestInvolvesTerm(t *testing.T) {
	t.Parallel()
	cfg, now := &Schema{}, time.Now()
	assigned := Issue{ID: "a1", Labels: []string{"assignee:guilbep"}}
	mentioned := Issue{ID: "b2"}
	other := Issue{ID: "c3", AuthorLogins: []string{"srossi"}}

	conds := ParseFilter("involves:@me")
	conds[0].Mentions = map[string]bool{"b2": true}
	for _, tc := range []struct {
		issue Issue
		want  bool
	}{{assigned, true}, {mentioned, true}, {other, false}} {
		if got := Matches(tc.issue, conds, cfg, "guilbep", now); got != tc.want {
			t.Errorf("involves:@me on %s = %v, want %v", tc.issue.ID, got, tc.want)
		}
	}
	if !Matches(other, ParseFilter("involves:srossi"), cfg, "guilbep", now) {
		t.Error("involves:srossi misses srossi's issue")
	}
	if Matches(assigned, ParseFilter("-involves:@me"), cfg, "guilbep", now) {
		t.Error("-involves:@me keeps an issue you're assigned")
	}
	if !RequiresMe(ParseFilter("involves:@me")) {
		t.Error("involves:@me needs to know who you are")
	}
}
func TestFollowsTerm(t *testing.T) {
	t.Parallel()
	cfg, now := &Schema{}, time.Now()
	assigned := Issue{ID: "a1", Labels: []string{"assignee:guilbep"}}
	unfollowed := Issue{ID: "b2", Labels: []string{"assignee:guilbep"}}
	followed := Issue{ID: "c3"}
	inView := Issue{ID: "d4"}
	nothing := Issue{ID: "e5"}

	conds := ParseFilter("follows:@me")
	conds[0].Followed = &Followed{
		Follow:   map[string]bool{"c3": true},
		Unfollow: map[string]bool{"b2": true},
		InViews:  map[string]bool{"d4": true},
	}
	for _, tc := range []struct {
		issue Issue
		want  bool
	}{{assigned, true}, {unfollowed, false}, {followed, true}, {inView, true}, {nothing, false}} {
		if got := Matches(tc.issue, conds, cfg, "guilbep", now); got != tc.want {
			t.Errorf("follows:@me on %s = %v, want %v", tc.issue.ID, got, tc.want)
		}
	}
	if Matches(assigned, ParseFilter("follows:ann"), cfg, "guilbep", now) {
		t.Error("follows: is yours only: follows:ann matches nothing")
	}
	if !RequiresMe(ParseFilter("follows:@me")) {
		t.Error("follows:@me needs to know who you are")
	}
}

// A role the issue takes from the project (internal/config's roles) isn't
// involvement: the project's lead-dev isn't in every issue's discussion.
// pm:@me still finds it, as a field.
func TestInvolvesLeavesOutProjectRoles(t *testing.T) {
	t.Parallel()
	cfg, now := &Schema{Fields: map[string]Field{"pm": {Type: TypePerson}}}, time.Now()
	inherited := Issue{ID: "a1", Inherited: []string{"pm:guilbep"}}
	if Matches(inherited, ParseFilter("involves:@me"), cfg, "guilbep", now) {
		t.Error("involves:@me took a project role for involvement")
	}
	if !Matches(inherited, ParseFilter("pm:@me"), cfg, "guilbep", now) {
		t.Error("pm:@me misses the project's pm")
	}
	own := Issue{ID: "b2", Labels: []string{"pm:guilbep"}}
	if !Matches(own, ParseFilter("involves:@me"), cfg, "guilbep", now) {
		t.Error("involves:@me misses a pm set on the issue itself")
	}
}
