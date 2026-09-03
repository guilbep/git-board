package main

import (
	"os"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/gitbugio"
)

const renderTestConfig = `
[fields.status]
single = true
options = ["backlog", "ready", "validated"]

[fields.prio]
single = true
options = ["p0", "p1", "p2"]

[fields.assignee]
single = false

[fields.est]
single = true
options = ["0.5", "1", "2"]
numeric = true
`

func TestLsRendersOneColumnPerConfiguredField(t *testing.T) {
	newTestRepo(t)
	if err := os.WriteFile(configPath, []byte(renderTestConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	id := openIssueID(t)

	b, err := gitbugio.OpenCwd()
	if err != nil {
		t.Fatalf("OpenCwd: %v", err)
	}
	// status:ready is already on the fixture issue (from newTestRepo);
	// add the rest, plus one label that isn't a configured field at all
	// -- must still show up (under "other"), never silently dropped.
	if _, err := b.ApplyLabelChange(id, []string{"prio:p1", "assignee:guilbep", "est:2", "needs-design"}, nil); err != nil {
		t.Fatalf("seed labels: %v", err)
	}
	b.Close()

	out, err := execute(t, []string{"ls", "is:open"})
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}

	// Header: one column per configured field (sorted: assignee, est,
	// prio, status), display-name capitalized, plus "state" (git-bug
	// open/closed -- distinct from the "Status" *field* column) and a
	// trailing "other" column.
	header := strings.SplitN(out, "\n", 2)[0]
	for _, want := range []string{"id", "title", "state", "Assignee", "Est", "Prio", "Status", "other"} {
		if !strings.Contains(header, want) {
			t.Errorf("header = %q, want it to contain %q", header, want)
		}
	}

	if !strings.Contains(out, "guilbep") {
		t.Error("Assignee value missing from output")
	}
	if !strings.Contains(out, "p1") {
		t.Error("Prio value missing from output")
	}
	if !strings.Contains(out, "ready") {
		t.Error("Status value missing from output")
	}
	if !strings.Contains(out, "2h") {
		t.Errorf("Est value = missing \"2h\" (numeric field, hours suffix) in output:\n%s", out)
	}
	if !strings.Contains(out, "needs-design") {
		t.Error("unrecognized label \"needs-design\" missing from output -- must not be silently dropped")
	}
	if strings.Contains(out, "status:ready") || strings.Contains(out, "prio:p1") {
		t.Errorf("raw ns:value label leaked into output instead of the field's own column:\n%s", out)
	}
}

func TestLsMarkdownRendersOneColumnPerConfiguredField(t *testing.T) {
	newTestRepo(t)
	if err := os.WriteFile(configPath, []byte(renderTestConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	id := openIssueID(t)

	b, err := gitbugio.OpenCwd()
	if err != nil {
		t.Fatalf("OpenCwd: %v", err)
	}
	if _, err := b.ApplyLabelChange(id, []string{"prio:p0"}, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}
	b.Close()

	out, err := execute(t, []string{"ls", "is:open", "--md"})
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "| Prio |") && !strings.Contains(out, "Prio |") {
		t.Errorf("markdown header missing Prio column:\n%s", out)
	}
	if !strings.Contains(out, "p0") {
		t.Error("Prio value missing from markdown output")
	}
}
