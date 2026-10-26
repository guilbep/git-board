package main

import (
	"os"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/store"
)

// statusOptionsConfig extends newTestRepo's minimal fixture with a real
// [fields.status] options list — needed for move/close, which read that
// order/terminal-value directly, unlike most other commands' tests.
const statusOptionsConfig = `
[fields.status]
single = true
options = ["backlog", "ready", "in-progress", "validated"]
`

func writeStatusOptionsConfig(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(configPath, []byte(statusOptionsConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// openIssueID returns the id of newTestRepo's open bug ("Fix invoice
// bug", status:ready).
func openIssueID(t *testing.T) string {
	t.Helper()
	b, err := store.OpenCwd()
	if err != nil {
		t.Fatalf("OpenCwd: %v", err)
	}
	defer b.Close()

	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	for _, issue := range issues {
		if issue.Status == "open" {
			return issue.ID
		}
	}
	t.Fatal("no open issue in fixture")
	return ""
}

func TestMoveAdvancesToNextOption(t *testing.T) {
	newTestRepo(t)
	writeStatusOptionsConfig(t)
	id := openIssueID(t)

	out, err := execute(t, []string{"move", id, "--next"})
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "status:in-progress") {
		t.Errorf("output = %q, want status:in-progress (ready -> in-progress)", out)
	}

	b, err := store.OpenCwd()
	if err != nil {
		t.Fatalf("OpenCwd: %v", err)
	}
	defer b.Close()
	labels, err := b.GetLabels(id)
	if err != nil {
		t.Fatalf("GetLabels: %v", err)
	}
	if !containsLabel(labels, "status:in-progress") {
		t.Errorf("labels = %v, want status:in-progress", labels)
	}
	if containsLabel(labels, "status:ready") {
		t.Errorf("labels = %v, want status:ready gone (single-cardinality swap)", labels)
	}
}

func TestMoveWithoutNextFlagErrors(t *testing.T) {
	newTestRepo(t)
	writeStatusOptionsConfig(t)
	id := openIssueID(t)

	_, err := execute(t, []string{"move", id})
	if err == nil {
		t.Fatal("expected an error without --next, got nil")
	}
}

func TestMoveAtLastOptionErrors(t *testing.T) {
	newTestRepo(t)
	writeStatusOptionsConfig(t)
	id := openIssueID(t)

	// Advance ready -> in-progress -> validated (the last option).
	if _, err := execute(t, []string{"move", id, "--next"}); err != nil {
		t.Fatalf("first move: %v", err)
	}
	if _, err := execute(t, []string{"move", id, "--next"}); err != nil {
		t.Fatalf("second move: %v", err)
	}

	_, err := execute(t, []string{"move", id, "--next"})
	if err == nil {
		t.Fatal("expected an error advancing past the last status option, got nil")
	}
	if !strings.Contains(err.Error(), "last option") {
		t.Errorf("error = %q, want it to explain there's no next option", err.Error())
	}
}

func TestMoveWithNoStatusSetGoesToFirstOption(t *testing.T) {
	newTestRepo(t)
	writeStatusOptionsConfig(t)
	id := openIssueID(t)

	// Clear the fixture's status:ready first.
	b, err := store.OpenCwd()
	if err != nil {
		t.Fatalf("OpenCwd: %v", err)
	}
	if _, err := b.ApplyLabelChange(id, nil, []string{"status:ready"}); err != nil {
		t.Fatalf("clear status: %v", err)
	}
	b.Close()

	out, err := execute(t, []string{"move", id, "--next"})
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "status:backlog") {
		t.Errorf("output = %q, want status:backlog (first option, issue had no status)", out)
	}
}
