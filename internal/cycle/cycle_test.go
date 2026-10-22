package cycle

import (
	"slices"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghpr"
)

var co2Options = []string{"backlog", "comite", "ready", "in-progress", "in-review", "ready-for-validation", "qa", "tests-metier", "validated"}

var co2 = (&config.Config{Fields: map[string]config.Field{"status": {Options: co2Options}}}).Stages()

var people = map[string]string{
	"risk": "alice", "plan": "bob", "review": "carol",
	"signoff:ready-for-validation": "bob", "signoff:qa": "alice", "signoff:tests-metier": "",
}

// noValidation is git-board's own board: Verify happens in in-review.
var noValidation = (&config.Config{Fields: map[string]config.Field{"status": {Options: []string{"draft", "backlog", "ready", "in-progress", "in-review", "done"}}}}).Stages()

var two = []string{"Exports a CSV", "Keeps the filters"}

// input is an issue at status with two criteria, author dan, assignee
// erin, viewed by alice; mod changes the rest.
func input(status string, mod func(*Input)) Input {
	in := Input{Stages: co2, Status: status, Criteria: two, Author: "dan", Assignees: []string{"erin"}, People: people, Me: "alice"}
	if mod != nil {
		mod(&in)
	}
	return in
}

func risk(r string) func(*Input) { return func(in *Input) { in.Risk = r } }

func approved(r string) func(*Input) {
	return func(in *Input) { in.Risk = r; in.PlanApproval = &Approval{By: "bob", At: time.Unix(1, 0)} }
}

func TestWalk(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		in      Input
		step    int
		login   string
		what    string
		mine    bool
		note    string
		gate    string
		actions []string
		skipped []int
		done    bool
	}{
		{name: "backlog", in: input("backlog", func(in *Input) { in.Criteria = nil }), step: 1, login: "dan", what: "add acceptance criteria", gate: "No issue, no agent"},
		{name: "backlog, criteria", in: input("backlog", func(in *Input) { in.Me = "dan" }), step: 1, login: "dan", what: "move the status to ready", mine: true, actions: []string{"move-ready"}},
		{name: "ready, no criteria", in: input("ready", func(in *Input) { in.Criteria = nil }), step: 1, login: "dan", what: "add acceptance criteria"},
		{name: "ready, no risk", in: input("ready", nil), step: 2, login: "alice", what: "set the risk class", mine: true, gate: "Class written on the issue", actions: []string{"set-risk"}},
		{name: "ready, normal", in: input("ready", risk("normal")), step: 3, login: "bob", what: "approve the plan", gate: "Plan approved by the plan role"},
		{name: "ready, normal, approved", in: input("ready", approved("normal")), step: 3, login: "erin", what: "move the status to in-progress", note: "plan approved, waits for the status in-progress"},
		{name: "ready, normal, plan accepted", in: input("ready", func(in *Input) { in.Risk = "normal"; in.Plan = &Plan{Path: "p.md", Status: "accepted"} }), step: 3, login: "erin", what: "move the status to in-progress", note: "plan approved, waits for the status in-progress"},
		{name: "ready, low, the assignee", in: input("ready", func(in *Input) { in.Risk = "low"; in.Me = "erin" }), step: 3, login: "erin", what: "move the status to in-progress", mine: true, note: "low risk, waits for the status in-progress", skipped: []int{3}, actions: []string{"start"}},
		{name: "ready, low", in: input("ready", risk("low")), step: 3, login: "erin", what: "move the status to in-progress", note: "low risk, waits for the status in-progress", gate: "Skipped, low risk", skipped: []int{3}},
		{name: "ready, structural, PR approved", in: input("ready", func(in *Input) { in.Risk = "structural"; in.PlanPR = &ghpr.PR{Decision: "APPROVED"} }), step: 3, login: "erin", what: "move the status to in-progress", note: "plan approved, waits for the status in-progress", gate: "Structural: plan PR approved first"},
		{name: "ready, structural, PR waiting", in: input("ready", func(in *Input) {
			in.Risk = "structural"
			in.PlanPR = &ghpr.PR{Decision: "REVIEW_REQUIRED"}
			in.Me = "bob"
		}), step: 3, login: "bob", what: "approve the plan PR on GitHub", mine: true},
		{name: "in-progress, no tests", in: input("in-progress", approved("normal")), step: 4, login: "erin", what: "write the failing test", gate: "The touched suites green, each criterion names its test"},
		{name: "in-progress, a bug", in: input("in-progress", func(in *Input) { approved("normal")(in); in.Kind = "bug" }), step: 4, login: "erin", what: "write the failing test", gate: "A test that fails before the fix"},
		{name: "in-progress, a doc", in: input("in-progress", func(in *Input) { approved("normal")(in); in.Kind = "doc"; in.Me = "erin" }), step: 5, login: "erin", what: "commit and open the PR", mine: true, gate: "A human commits and opens the PR", skipped: []int{4}, actions: []string{"ready-for-review"}},
		{name: "in-progress, tests", in: input("in-progress", func(in *Input) { approved("normal")(in); in.Tests = []TestFile{{Path: "a_test.go", Commit: "c1"}} }), step: 5, login: "erin", what: "commit and open the PR", gate: "A human commits and opens the PR"},
		{name: "in-progress, the floor", in: input("in-progress", func(in *Input) { in.Criteria = nil }), step: 4, login: "erin", what: "write the failing test", note: "risk not set"},
		{name: "in-progress, plan not approved", in: input("in-progress", risk("normal")), step: 4, login: "erin", what: "write the failing test", note: "plan not approved"},
		{name: "in-progress, low", in: input("in-progress", risk("low")), step: 4, login: "erin", what: "write the failing test", skipped: []int{3}},
		{name: "in-progress, PR open", in: input("in-progress", func(in *Input) {
			approved("normal")(in)
			in.Tests = []TestFile{{Path: "a_test.go"}}
			in.PR = &ghpr.PR{Number: 3082, State: "OPEN"}
		}), step: 5, login: "erin", what: "commit and open the PR", note: "PR #3082 open, waits for the status in-review"},
		{name: "in-progress, draft PR", in: input("in-progress", func(in *Input) {
			approved("normal")(in)
			in.Tests = []TestFile{{Path: "a_test.go"}}
			in.PR = &ghpr.PR{Number: 41, State: "OPEN", Draft: true}
			in.Me = "erin"
		}), step: 5, login: "erin", what: "commit and open the PR", mine: true, note: "draft PR #41, waits for it to be marked ready for review"},
		{name: "in-progress, draft PR, no test yet", in: input("in-progress", func(in *Input) {
			approved("normal")(in)
			in.PR = &ghpr.PR{Number: 41, State: "OPEN", Draft: true}
		}), step: 4, login: "erin", what: "write the failing test", note: "draft PR #41, waits for it to be marked ready for review"},
		{name: "in-review, draft PR", in: input("in-review", func(in *Input) {
			approved("normal")(in)
			in.PR = &ghpr.PR{Number: 41, State: "OPEN", Draft: true}
			in.Me = "carol"
		}), step: 6, login: "carol", what: "review the PR", mine: true, note: "PR #41 is still a draft on GitHub"},
		{name: "in-progress, tests, the assignee", in: input("in-progress", func(in *Input) { approved("normal")(in); in.Tests = []TestFile{{Path: "a_test.go"}}; in.Me = "erin" }), step: 5, login: "erin", what: "commit and open the PR", mine: true, actions: []string{"ready-for-review"}},
		{name: "in-review", in: input("in-review", approved("normal")), step: 6, login: "carol", what: "review the PR", gate: "A human who can explain the diff"},
		{name: "in-review, structural", in: input("in-review", func(in *Input) { in.Risk = "structural"; in.Me = "carol" }), step: 6, login: "carol", what: "review the PR", mine: true, gate: "Structural: a second human", actions: []string{"review"}},
		{name: "in-review, merged", in: input("in-review", func(in *Input) {
			approved("normal")(in)
			in.PR = &ghpr.PR{Number: 3082, State: "MERGED"}
			in.Me = "erin"
		}), step: 6, login: "erin", what: "move the status to ready-for-validation", mine: true, note: "merged, waits for the status ready-for-validation", actions: []string{"to-validation"}},
		{name: "in-review, merged, no validation status", in: input("in-review", func(in *Input) {
			approved("normal")(in)
			in.Stages = noValidation
			in.PR = &ghpr.PR{Number: 3082, State: "MERGED"}
		}), step: 7, what: "sign off in-review", mine: true, note: "no plan file", gate: "Green codes are not proof", actions: []string{"verify", "check", "signoff"}},
		{name: "in-review, open, no validation status", in: input("in-review", func(in *Input) {
			approved("normal")(in)
			in.Stages = noValidation
			in.PR = &ghpr.PR{Number: 3082, State: "OPEN"}
		}), step: 6, login: "carol", what: "review the PR"},
		{name: "qa", in: input("qa", risk("normal")), step: 7, login: "alice", what: "sign off qa", mine: true, note: "no plan file", gate: "Green codes are not proof", actions: []string{"verify", "check", "signoff"}},
		{name: "qa, plan not delivered", in: input("qa", func(in *Input) { in.Risk = "normal"; in.Plan = &Plan{Path: "p.md", Status: "accepted"} }), step: 7, login: "alice", what: "sign off qa", mine: true, note: "plan not marked delivered", actions: []string{"verify", "check", "signoff"}},
		{name: "tests-metier, anyone", in: input("tests-metier", risk("low")), step: 7, what: "sign off tests-metier", mine: true, actions: []string{"verify", "check", "signoff"}, skipped: []int{3}},
		{name: "validated", in: input("validated", risk("low")), step: 7, done: true, gate: "Green codes are not proof", skipped: []int{3}},
		{name: "closed at qa", in: input("qa", func(in *Input) { in.Risk = "structural"; in.Closed = true }), step: 7, done: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Compute(tc.in)
			if c.Step != tc.step || c.Of != 7 || c.Waits.Login != tc.login || c.Waits.What != tc.what || c.Mine != tc.mine || c.Note != tc.note || c.Done != tc.done {
				t.Errorf("got step %d of %d, waits %q to %q, mine %v, note %q, done %v\nwant step %d, waits %q to %q, mine %v, note %q, done %v",
					c.Step, c.Of, c.Waits.Login, c.Waits.What, c.Mine, c.Note, c.Done, tc.step, tc.login, tc.what, tc.mine, tc.note, tc.done)
			}
			if tc.gate != "" && c.Gate != tc.gate {
				t.Errorf("gate = %q, want %q", c.Gate, tc.gate)
			}
			if !slices.Equal(c.Actions, tc.actions) || !slices.Equal(c.Skipped, tc.skipped) {
				t.Errorf("actions %v skipped %v, want %v %v", c.Actions, c.Skipped, tc.actions, tc.skipped)
			}
			if want := Steps[tc.step]; tc.done {
				if c.Name != "Done" {
					t.Errorf("name = %q, want Done", c.Name)
				}
			} else if c.Name != want {
				t.Errorf("name = %q, want %q", c.Name, want)
			}
		})
	}
}

func TestStatusAloneNeverTellsTwoFromThree(t *testing.T) {
	t.Parallel()
	a, b := Compute(input("ready", nil)), Compute(input("ready", risk("normal")))
	if a.Step != 2 || b.Step != 3 {
		t.Errorf("steps %d and %d, want 2 and 3", a.Step, b.Step)
	}
}

func TestComputeNoStatus(t *testing.T) {
	t.Parallel()
	c := Compute(Input{})
	if c.Step != 1 || c.Name != "Intent" || !c.Waits.Anyone || !c.Mine || c.Done {
		t.Errorf("Compute(Input{}) = %+v", c)
	}
}

func TestCriteriaCount(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, desc string
		want       []string
	}{
		{"checklist", "Intro\n\n## Acceptance criteria\n\n- [ ] one\n- [x] two\n- [ ] three\n", []string{"one", "two", "three"}},
		{"bold, numbered", "**Acceptance criteria:**\n1. first\n2) second\n", []string{"first", "second"}},
		{"heading, no list", "### Acceptance Criteria\n\nTo be written.\n- not this\n", nil},
		{"no heading", "- one\n- two\n", nil},
		{"cut by a paragraph", "# Acceptance criteria\n* a\n+ b\n\nNotes: later\n- c\n", []string{"a", "b"}},
		// GB-31: its four criteria were under "Expected"
		{"expected", "## Steps\n\n1. open it\n\n## Expected\n\n- one\n- two\n- three\n- four\n", []string{"one", "two", "three", "four"}},
		{"done when, bold", "**Done when:**\n- shipped\n", []string{"shipped"}},
		{"the first heading found", "## Expected\n- e\n\n## Acceptance criteria\n- a\n", []string{"e"}},
		{"a heading's prefix", "## Expected results\n- no\n", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := Criteria(tc.desc, co2.Criteria); !slices.Equal(got, tc.want) {
				t.Errorf("Criteria = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCriteriaProblems(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, desc string
		want       []string
	}{
		{"clean", "Intro\n\n## Expected\n\n- one\n- two\n\n## Notes\n\nlater\n- an aside\n", nil},
		{"no heading", "- one\n", nil},
		{"no list", "## Expected\n\nTo be written.\n", []string{"line 1: no list right under “Expected”"}},
		{"bold, no list", "**Done when:**\n", []string{"line 1: no list right under “Done when”"}},
		{"a second heading", "## Expected\n- e\n\n**Done when**\n- d\n", []string{"line 4: a second criteria heading, ignored: only the first list counts"}},
		{"cut by a paragraph", "## Expected\n- a\nAlso:\n- b\n", []string{"line 3 is not a list item: the list stops there, and the items after it don't count"}},
		{"a sub-item", "## Expected\n- a\n  - a.1\n- b\n", []string{"line 3: a sub-item, counted as a criterion of its own"}},
		{"indented, all alike", "## Expected\n  - a\n  - b\n", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, got := Criteria(tc.desc, co2.Criteria); !slices.Equal(got, tc.want) {
				t.Errorf("problems = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCardShowsToRoleOnly(t *testing.T) {
	t.Parallel()
	bob := Compute(input("qa", func(in *Input) { in.Risk = "low"; in.Me = "bob" }))
	if bob.Mine || bob.Actions != nil {
		t.Errorf("bob at qa: mine %v, actions %v", bob.Mine, bob.Actions)
	}
	if alice := Compute(input("qa", risk("low"))); !alice.Mine {
		t.Error("alice signs off qa")
	}
	if c := Compute(input("qa", func(in *Input) { in.Risk = "low"; in.Me = "ALICE" })); !c.Mine {
		t.Error("logins compare without case")
	}
	nobody := map[string]string{"signoff:qa": ""}
	for _, me := range []string{"alice", "bob"} {
		if c := Compute(input("qa", func(in *Input) { in.Risk = "low"; in.People = nobody; in.Me = me })); !c.Mine || !c.Waits.Anyone {
			t.Errorf("%s with nobody at qa: mine %v, anyone %v", me, c.Mine, c.Waits.Anyone)
		}
	}
}

func TestCriteriaOwnHeading(t *testing.T) {
	t.Parallel()
	d := "## Acceptance criteria\n- not this\n\n### Definition of Done (v2)\n- this\n"
	if got, _ := Criteria(d, []string{"Definition of done (v2)"}); !slices.Equal(got, []string{"this"}) {
		t.Errorf("Criteria = %q", got)
	}
	if got, _ := Criteria(d, nil); got != nil {
		t.Errorf("no headings: %q", got)
	}
}
