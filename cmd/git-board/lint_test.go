package main

import (
	"os"
	"strings"
	"testing"

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

	origExisting, origCreate := existingGithubLabels, createGithubLabel
	t.Cleanup(func() { existingGithubLabels, createGithubLabel = origExisting, origCreate })
	existingGithubLabels = func(repo string) ([]string, error) {
		return []string{"status:backlog"}, nil // status:ready and prio:p0 are missing
	}
	created := 0
	createGithubLabel = func(repo, name string) error {
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

	origExisting, origCreate := existingGithubLabels, createGithubLabel
	t.Cleanup(func() { existingGithubLabels, createGithubLabel = origExisting, origCreate })
	existingGithubLabels = func(repo string) ([]string, error) {
		return []string{"status:backlog"}, nil
	}
	var createdNames []string
	createGithubLabel = func(repo, name string) error {
		createdNames = append(createdNames, name)
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
