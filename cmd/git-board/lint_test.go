package main

import (
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
