package lint_test

import (
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/lint"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

func mustLoadConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load("../config/testdata/git-board.toml")
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}

// countChecks tallies how many findings of each check name are present, so
// tests assert "this check fired N times" without depending on ListIssues'
// iteration order.
func countChecks(findings []lint.Finding) map[string]int {
	counts := map[string]int{}
	for _, f := range findings {
		counts[f.Check]++
	}
	return counts
}

func labelIssue(t *testing.T, b *store.Backend, id string, labels ...string) {
	t.Helper()
	if _, err := b.ApplyLabelChange(id, labels, nil); err != nil {
		t.Fatalf("ApplyLabelChange: %v", err)
	}
}

// A "clean" repo is one where every check has nothing to say: every open
// issue carries exactly one value per single-cardinality field, from the
// configured options, every sprint label matches a configured sprint, and
// (for dead-option, which scans the config's options against the whole
// repo) every configured option is actually used somewhere. The shared
// testdata config has far more options than a single fixture issue could
// exercise, so this test uses its own minimal one-option-per-field config
// instead — the only way to construct a repo with truly zero drift.
func TestCleanRepoReportsNothing(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Fields: map[string]config.Field{
			"status": {Single: true, Options: []string{"ready"}},
			"prio":   {Single: true, Options: []string{"p1"}},
			"cat":    {Single: true, Options: []string{"bug"}},
		},
		Sprints: config.SprintsConfig{List: []config.Sprint{{Slug: "delivery", Title: "Delivery"}}},
	}
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "clean issue")
	labelIssue(t, b, id, "status:ready", "prio:p1", "cat:bug", "sprint:delivery")

	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	findings := lint.Run(issues, cfg)
	if len(findings) != 0 {
		t.Errorf("expected no findings on a fully-compliant repo, got %+v", findings)
	}
}

func TestDoubleSingleDetected(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	tripping := storetest.NewIssue(t, b, "double status")
	labelIssue(t, b, tripping, "status:ready", "status:qa", "prio:p1", "est:1", "cat:bug")
	clean := storetest.NewIssue(t, b, "single status")
	labelIssue(t, b, clean, "status:ready", "prio:p1", "est:1", "cat:bug")

	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	findings := lint.Run(issues, cfg)
	counts := countChecks(findings)
	if counts[lint.CheckDoubleSingle] != 1 {
		t.Errorf("CheckDoubleSingle count = %d, want 1: %+v", counts[lint.CheckDoubleSingle], findings)
	}
	for _, f := range findings {
		if f.Check == lint.CheckDoubleSingle && f.IssueID != tripping {
			t.Errorf("double-single reported on %s, want %s", f.IssueID, tripping)
		}
	}
}

func TestUnknownOptionValueDetected(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	tripping := storetest.NewIssue(t, b, "bad prio")
	labelIssue(t, b, tripping, "status:ready", "prio:p9", "est:1", "cat:bug")
	clean := storetest.NewIssue(t, b, "good prio")
	labelIssue(t, b, clean, "status:ready", "prio:p1", "est:1", "cat:bug")

	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	findings := lint.Run(issues, cfg)
	counts := countChecks(findings)
	if counts[lint.CheckUnknownValue] != 1 {
		t.Errorf("CheckUnknownValue count = %d, want 1: %+v", counts[lint.CheckUnknownValue], findings)
	}
	for _, f := range findings {
		if f.Check == lint.CheckUnknownValue && f.IssueID != tripping {
			t.Errorf("unknown-value reported on %s, want %s", f.IssueID, tripping)
		}
	}
}

func TestMissingRequiredValueDetected(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	tripping := storetest.NewIssue(t, b, "no prio")
	labelIssue(t, b, tripping, "status:ready", "est:1", "cat:bug")
	clean := storetest.NewIssue(t, b, "has prio")
	labelIssue(t, b, clean, "status:ready", "prio:p1", "est:1", "cat:bug")

	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	findings := lint.Run(issues, cfg)

	var got []string
	for _, f := range findings {
		if f.Check == lint.CheckMissingRequired && f.IssueID == tripping {
			got = append(got, f.Message)
		}
	}
	if len(got) != 1 || got[0] != "no prio value" {
		t.Errorf("missing-value findings on tripping issue = %v, want exactly [\"no prio value\"]", got)
	}
	for _, f := range findings {
		if f.Check == lint.CheckMissingRequired && f.IssueID == clean {
			t.Errorf("clean issue unexpectedly flagged missing: %+v", f)
		}
	}
}

func TestClosedIssueSkipsOpenOnlyChecks(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "closed, no prio")
	labelIssue(t, b, id, "status:ready", "status:qa") // would trip double-single AND missing-prio if open
	storetest.CloseIssue(t, b, id)

	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	findings := lint.Run(issues, cfg)
	counts := countChecks(findings)
	if counts[lint.CheckDoubleSingle] != 0 || counts[lint.CheckMissingRequired] != 0 {
		t.Errorf("closed issue should not trip open-only checks, got %+v", findings)
	}
}

func TestOrphanSprintDetected(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	tripping := storetest.NewIssue(t, b, "orphan sprint")
	labelIssue(t, b, tripping, "status:ready", "prio:p1", "est:1", "cat:bug", "sprint:nonexistent")
	clean := storetest.NewIssue(t, b, "known sprint")
	labelIssue(t, b, clean, "status:ready", "prio:p1", "est:1", "cat:bug", "sprint:delivery")

	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	findings := lint.Run(issues, cfg)
	counts := countChecks(findings)
	if counts[lint.CheckOrphanSprint] != 1 {
		t.Errorf("CheckOrphanSprint count = %d, want 1: %+v", counts[lint.CheckOrphanSprint], findings)
	}
	for _, f := range findings {
		if f.Check == lint.CheckOrphanSprint && f.IssueID != tripping {
			t.Errorf("orphan-sprint reported on %s, want %s", f.IssueID, tripping)
		}
	}
}

func TestDeadOptionDetected(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	// cat has options bug/feature/chore; only "bug" gets used, so
	// "feature" and "chore" should be reported dead.
	id := storetest.NewIssue(t, b, "only cat:bug used")
	labelIssue(t, b, id, "status:ready", "prio:p1", "est:1", "cat:bug")

	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	findings := lint.Run(issues, cfg)

	dead := map[string]bool{}
	for _, f := range findings {
		if f.Check == lint.CheckDeadOption {
			dead[f.Message] = true
		}
	}
	if !dead["cat:feature: no issue carries it"] || !dead["cat:chore: no issue carries it"] {
		t.Errorf("expected cat:feature and cat:chore reported dead, got dead findings: %+v", findings)
	}
	for _, f := range findings {
		if f.Check == lint.CheckDeadOption && f.Message == "cat:bug: no issue carries it" {
			t.Errorf("cat:bug is used, should not be reported dead: %+v", f)
		}
	}
}

// The regression case for dead-option's open/closed asymmetry: a status
// value that only appears on a closed issue (e.g. a terminal "validated"
// status) is still in use — checkDeadOptions must not report it, even
// though the open-only checks never see the issue that carries it.
func TestDeadOptionSeesClosedIssues(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "validated and closed")
	labelIssue(t, b, id, "status:validated", "prio:p1", "est:1", "cat:bug")
	storetest.CloseIssue(t, b, id)

	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	findings := lint.Run(issues, cfg)
	for _, f := range findings {
		if f.Check == lint.CheckDeadOption && f.Message == "status:validated: no issue carries it" {
			t.Errorf("status:validated is used by a closed issue, must not be reported dead: %+v", f)
		}
	}
}

func TestKindChecks(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{Branches: config.Branches{Kinds: map[string]string{"bug": "fix", "feat": "feat"}}}
	open := func(id, title string, labels ...string) store.Issue {
		return store.Issue{ID: id, Title: title, Status: "open", Labels: labels}
	}
	issues := []store.Issue{
		open("a", "[BUG](Equipment) Wrong example", "bug"),  // agrees
		open("b", "[Bug](Equipment) Wrong example", "feat"), // the title disagrees
		open("c", "Export", "bug", "feat"),                  // two kinds
		open("d", "[FEAT] Export"),                          // no kind
		open("e", "[TASK] Clean up"),                        // no kind
		open("f", "[FEAT/DATA] Import", "feat"),             // not a kind in the title
		open("g", "[BUG] Crash", "kind:bug"),                // the kind field agrees
		{ID: "h", Title: "[BUG] Closed", Status: "closed"},  // closed: not looked at
		open("i", "Export", "kind:bug", "kind:feat"),        // two kinds, as fields
	}
	findings := lint.Run(issues, cfg)
	got := countChecks(findings)
	for check, n := range map[string]int{lint.CheckKindTitle: 4, lint.CheckTwoKinds: 2, lint.CheckNoKind: 1} {
		if got[check] != n {
			t.Errorf("%s: %d findings, want %d (all: %+v)", check, got[check], n, findings)
		}
	}
	// a title never carries the kind: a bracket goes, agreeing or not
	titles := map[string]string{
		"a": "the title starts with [BUG]: the kind is its label, bug",
		"b": "the title says [Bug], the label says feat: the label wins",
		"f": "the title starts with [FEAT/DATA]: the kind is its label, feat",
		"g": "the title starts with [BUG]: the kind is its label, bug",
	}
	for _, f := range findings {
		switch f.Check {
		case lint.CheckKindTitle:
			if f.Message != titles[f.IssueID] {
				t.Errorf("kind-title finding = %+v", f)
			}
		case lint.CheckNoKind:
			if f.Message != "2 open issues have none of [branches] kinds (bug, feat)" {
				t.Errorf("no-kind finding = %+v", f)
			}
		}
	}

	if got := countChecks(lint.Run(issues, &config.Config{})); got[lint.CheckNoKind]+got[lint.CheckKindTitle]+got[lint.CheckTwoKinds] != 0 {
		t.Errorf("no [branches] kinds, yet kind findings: %v", got)
	}

	// the kind a declared single field: told once, by the kind checks; no
	// hold is not a missing value
	cfg.Fields = map[string]config.Field{"kind": {Single: true, Options: []string{"bug", "feat"}}, "hold": {Single: true, Options: []string{"blocked", "paused"}}}
	got = countChecks(lint.Run(issues, cfg))
	if got[lint.CheckTwoKinds] != 2 || got[lint.CheckDoubleSingle]+got[lint.CheckMissingRequired] != 0 {
		t.Errorf("kind field: %v, want 2 two-kinds and no double-single or missing-value", got)
	}
}

// With [labels], an open issue's plain labels outside its list are
// reported; a kind (flat) and a field's value never are; without the
// table, nothing.
func TestLabelsOutsideTheList(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "Crash on save")
	labelIssue(t, b, id, "needs-design", "wontfix", "bug", "prio:p1")
	issues, err := b.ListIssues()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Branches: config.Branches{Kinds: map[string]string{"bug": "fix"}}, Labels: config.Labels{List: []string{"needs-design"}, Defined: true}}
	var got []string
	for _, f := range lint.Run(issues, cfg) {
		if f.Check == lint.CheckLabelOutside {
			got = append(got, f.Message)
		}
	}
	if len(got) != 1 || got[0] != `label "wontfix" isn't in [labels] list: add it there, or remove it` {
		t.Errorf("findings %q", got)
	}
	cfg.Labels = config.Labels{}
	if n := countChecks(lint.Run(issues, cfg))[lint.CheckLabelOutside]; n != 0 {
		t.Errorf("without [labels]: %d findings", n)
	}
}
