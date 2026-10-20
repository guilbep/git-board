package main

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/ghlabels"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// seedDoubleSingle trips the double-single check directly via the backend
// (bypassing `set`'s swap-on-write behavior, the same way the fixture would
// concurrently from two people) on newTestRepo's open bug, so lint has a
// real finding to report.
func seedDoubleSingle(t *testing.T) {
	t.Helper()
	b, err := gitbugio.OpenCwd()
	if err != nil {
		t.Fatalf("OpenCwd: %v", err)
	}
	defer b.Close()

	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	var openID string
	for _, issue := range issues {
		if issue.Status == "open" {
			openID = issue.ID
		}
	}
	if openID == "" {
		t.Fatalf("no open issue found in %+v", issues)
	}
	if _, err := b.ApplyLabelChange(openID, []string{"status:qa"}, nil); err != nil {
		t.Fatalf("ApplyLabelChange: %v", err)
	}
}

func TestLintCleanRepoExitsZero(t *testing.T) {
	newTestRepo(t) // "Fix invoice bug" (open, status:ready) + "Write specs" (closed, no status)

	out, err := execute(t, []string{"lint"})
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "summary: 0 finding(s)") {
		t.Errorf("expected a zero-finding summary, got:\n%s", out)
	}
}

// The default exit code stays 0 even with findings present — a report, not
// a build gate, per the task's explicit call-out. --strict (tested below)
// is the opt-in way to fail a run on findings.
func TestLintReportsFindingsButExitsZeroByDefault(t *testing.T) {
	newTestRepo(t)
	seedDoubleSingle(t)

	out, err := execute(t, []string{"lint"})
	if err != nil {
		t.Fatalf("expected exit 0 with findings present (no --strict), got err: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "double-single") {
		t.Errorf("expected double-single in output:\n%s", out)
	}
}

const createLabelsTestConfig = `
[repos]
list = ["EPFL-ENAC/co2-calculator"]

[fields.status]
single = true
options = ["backlog", "ready"]

[fields.prio]
single = true
options = ["p0"]
`

func TestLintCreateLabelsPreviewsMissingLabelsWithoutWriting(t *testing.T) {
	newTestRepo(t)
	if err := os.WriteFile(configPath, []byte(createLabelsTestConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origExisting, origApply := existingGithubLabels, applyGithubLabel
	t.Cleanup(func() { existingGithubLabels, applyGithubLabel = origExisting, origApply })
	existingGithubLabels = func(repo string) ([]ghlabels.Label, error) {
		return ghLabels("status:backlog"), nil // status:ready and prio:p0 are missing
	}
	created := 0
	applyGithubLabel = func(ghlabels.Change) error {
		created++
		return nil
	}

	out, err := execute(t, []string{"lint", "--create-labels"})
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}
	if created != 0 {
		t.Errorf("preview (no --apply) must not create labels, got %d", created)
	}
	if !strings.Contains(out, "missing prio:p0") || !strings.Contains(out, "missing status:ready") {
		t.Errorf("expected both missing labels reported, got:\n%s", out)
	}
	if strings.Contains(out, "status:backlog") {
		t.Errorf("already-present label must not be reported missing:\n%s", out)
	}
}

func TestLintCreateLabelsApplyCreatesMissingLabels(t *testing.T) {
	newTestRepo(t)
	if err := os.WriteFile(configPath, []byte(createLabelsTestConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origExisting, origApply := existingGithubLabels, applyGithubLabel
	t.Cleanup(func() { existingGithubLabels, applyGithubLabel = origExisting, origApply })
	existingGithubLabels = func(repo string) ([]ghlabels.Label, error) {
		return ghLabels("status:backlog"), nil
	}
	var createdNames []string
	applyGithubLabel = func(c ghlabels.Change) error {
		createdNames = append(createdNames, c.Name)
		return nil
	}

	out, err := execute(t, []string{"lint", "--create-labels", "--apply"})
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}
	if len(createdNames) != 2 {
		t.Errorf("expected 2 labels created, got %v", createdNames)
	}
	if !strings.Contains(out, "created prio:p0") || !strings.Contains(out, "created status:ready") {
		t.Errorf("expected both created labels reported, got:\n%s", out)
	}
}

// ghLabels is GitHub's labels by name, uncolored.
func ghLabels(names ...string) []ghlabels.Label {
	res := make([]ghlabels.Label, len(names))
	for i, n := range names {
		res[i] = ghlabels.Label{Name: n}
	}
	return res
}

func TestDescribeChange(t *testing.T) {
	t.Parallel()
	was := ghlabels.Label{Name: "bug", Color: "d73a4a", Description: "Broken"}
	for c, want := range map[ghlabels.Change]string{
		{Name: "feat", Color: "0e8a16", Description: "A new feature"}:       `missing feat #0e8a16 "A new feature"`,
		{Name: "bug", Color: "b60205", Description: "Broken", Was: &was}:    "update bug: color d73a4a → b60205",
		{Name: "bug", Color: "d73a4a", Description: "It breaks", Was: &was}: `update bug: description "Broken" → "It breaks"`,
	} {
		if got := describeChange(c); got != want {
			t.Errorf("describeChange = %s, want %s", got, want)
		}
	}
}

// Without [repos], --create-labels plans for origin's GitHub repository,
// the one the Labels tab shows.
func TestLintCreateLabelsFallsBackToOrigin(t *testing.T) {
	newTestRepo(t)
	if err := os.WriteFile(configPath, []byte("[fields.prio]\nsingle = true\noptions = [\"p0\"]\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	origExisting, origOrigin := existingGithubLabels, originGithubRepo
	t.Cleanup(func() { existingGithubLabels, originGithubRepo = origExisting, origOrigin })
	var asked []string
	existingGithubLabels = func(repo string) ([]ghlabels.Label, error) {
		asked = append(asked, repo)
		return nil, nil
	}

	originGithubRepo = func() string { return "o/r" }
	out, err := execute(t, []string{"lint", "--create-labels"})
	if err != nil || !slices.Equal(asked, []string{"o/r"}) || !strings.Contains(out, "o/r: missing prio:p0") {
		t.Errorf("asked %v, err %v, output:\n%s", asked, err, out)
	}

	originGithubRepo = func() string { return "" }
	if out, err := execute(t, []string{"lint", "--create-labels"}); err == nil || !strings.Contains(err.Error(), "no GitHub repository") {
		t.Errorf("no repository at all: %v\noutput:\n%s", err, out)
	}
}

func TestLintApplyWithoutCreateLabelsErrors(t *testing.T) {
	newTestRepo(t)

	out, err := execute(t, []string{"lint", "--apply"})
	if err == nil {
		t.Errorf("expected --apply without --create-labels to error, output:\n%s", out)
	}
}

func TestLintStrictFailsOnFindings(t *testing.T) {
	newTestRepo(t)
	seedDoubleSingle(t)

	out, err := execute(t, []string{"lint", "--strict"})
	if err == nil {
		t.Errorf("expected --strict to fail with a double-single finding present, output:\n%s", out)
	}
	if !strings.Contains(out, "double-single") {
		t.Errorf("expected double-single in output:\n%s", out)
	}
}

func TestLintCreateLabelsReportsTemplateLabelsTheRepoLacks(t *testing.T) {
	newTestRepo(t)
	if err := os.WriteFile(configPath, []byte(createLabelsTestConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	templates := map[string]string{
		"feature.yaml": "name: Feature\ntitle: \"[FEAT]() \"\nlabels: [\"enhancement\", \"needs-triage\"]\nbody: []\n",
		"bug.yml":      "name: Bug\nlabels:\n  - bug\n",
		"task.md":      "---\nname: Task\nlabels: Feat\n---\nWhat to do\n", // GitHub's names ignore case
		"config.yml":   "blank_issues_enabled: false\n",                    // the chooser, not a template
	}
	if err := os.MkdirAll(".github/ISSUE_TEMPLATE", 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range templates {
		if err := os.WriteFile(".github/ISSUE_TEMPLATE/"+name, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	origExisting, origApply, origCurrent := existingGithubLabels, applyGithubLabel, currentGithubRepo
	t.Cleanup(func() {
		existingGithubLabels, applyGithubLabel, currentGithubRepo = origExisting, origApply, origCurrent
	})
	existingGithubLabels = func(repo string) ([]ghlabels.Label, error) {
		return ghLabels("status:backlog", "status:ready", "prio:p0", "bug", "feat"), nil
	}
	currentGithubRepo = func() (string, error) { return "EPFL-ENAC/co2-calculator", nil }
	var created []string
	applyGithubLabel = func(c ghlabels.Change) error {
		created = append(created, c.Name)
		return nil
	}

	out, err := execute(t, []string{"lint", "--create-labels", "--apply"})
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "feature.yaml applies enhancement, needs-triage") {
		t.Errorf("expected feature.yaml's missing labels reported, got:\n%s", out)
	}
	if strings.Contains(out, "bug.yml") || strings.Contains(out, "task.md") || strings.Contains(out, "config.yml") {
		t.Errorf("only feature.yaml lacks labels, got:\n%s", out)
	}
	if len(created) != 0 {
		t.Errorf("--apply created %v: a template's label is fixed in the template, not created", created)
	}
}

// The issue templates' check is a warning: when gh can't say which
// repository this checkout is, or a template doesn't parse, every label
// can still exist, and CI must not fail for it.
func TestLintCreateLabelsTemplateCheckOnlyWarns(t *testing.T) {
	newTestRepo(t)
	if err := os.WriteFile(configPath, []byte(createLabelsTestConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.MkdirAll(".github/ISSUE_TEMPLATE", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".github/ISSUE_TEMPLATE/bug.yml", []byte("name: Bug\nlabels: [bug]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	origExisting, origCurrent := existingGithubLabels, currentGithubRepo
	t.Cleanup(func() { existingGithubLabels, currentGithubRepo = origExisting, origCurrent })
	existingGithubLabels = func(repo string) ([]ghlabels.Label, error) {
		return ghLabels("status:backlog", "status:ready", "prio:p0", "bug"), nil
	}
	currentGithubRepo = func() (string, error) { return "", errors.New("gh repo view: no default repository") }

	out, err := execute(t, []string{"lint", "--create-labels"})
	if err != nil {
		t.Fatalf("every label exists, the template check couldn't run: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "issue templates: not checked: gh repo view: no default repository") {
		t.Errorf("expected the template check's warning, got:\n%s", out)
	}

	currentGithubRepo = func() (string, error) { return "o/r", nil }
	if err := os.WriteFile(".github/ISSUE_TEMPLATE/broken.yml", []byte("labels: [bug\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := execute(t, []string{"lint", "--create-labels"}); err != nil || !strings.Contains(out, "issue templates: not checked: broken.yml") {
		t.Errorf("a template that doesn't parse: %v\noutput:\n%s", err, out)
	}
}
