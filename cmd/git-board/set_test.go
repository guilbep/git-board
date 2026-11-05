package main

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/fields"
	"github.com/guilbep/git-board/internal/store"
)

const fieldsConfig = `
[fields.status]
single = true
options = ["backlog", "ready", "in-progress", "validated"]

[fields.prio]
single = true
options = ["p0", "p1", "p2", "p3"]

[fields.assignee]
single = false
`

// labelsOf reads an issue's labels and state afresh.
func labelsOf(t *testing.T, id string) ([]string, string) {
	t.Helper()
	b, err := store.OpenCwd()
	if err != nil {
		t.Fatalf("OpenCwd: %v", err)
	}
	defer b.Close()
	is, err := b.GetIssue(id)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	slices.Sort(is.Labels)
	return is.Labels, is.Status
}

// The matrix's probes: each wrote garbage and exited 0.
func TestSetRefusesWhatTheConfigDoesntAllow(t *testing.T) {
	newTestRepo(t)
	writeConfig(t, fieldsConfig)
	id := openIssueID(t)

	for _, args := range [][]string{
		{"prio=p9"},                // not an option
		{"milestone=v1"},           // a field nothing knows
		{"prio=p1", "status=nope"}, // one bad: none written
	} {
		out, err := execute(t, append([]string{"set", id}, args...))
		if !errors.Is(err, fields.ErrUnchecked) {
			t.Errorf("set %v = %v, want ErrUnchecked\n%s", args, err, out)
		}
		if err != nil && !strings.Contains(err.Error(), "--force") {
			t.Errorf("set %v: the error should name --force: %v", args, err)
		}
	}
	for _, args := range [][]string{{"=x"}, {"prio=-"}, {"prio=+p1"}, {"label=+prio:p1"}, {"noequals"}} {
		if out, err := execute(t, append([]string{"set", id}, args...)); !errors.Is(err, fields.ErrInvalid) {
			t.Errorf("set %v = %v, want ErrInvalid\n%s", args, err, out)
		}
	}
	if labels, _ := labelsOf(t, id); !slices.Equal(labels, []string{"status:ready"}) {
		t.Errorf("labels = %v, want [status:ready]: nothing written", labels)
	}

	if out, err := execute(t, []string{"set", id, "milestone=v1", "--force"}); err != nil {
		t.Fatalf("set --force: %v\n%s", err, out)
	}
	// now on an issue, milestone is a known field
	if out, err := execute(t, []string{"set", id, "milestone=v2"}); err != nil {
		t.Fatalf("set milestone=v2: %v\n%s", err, out)
	}
	if labels, _ := labelsOf(t, id); !slices.Equal(labels, []string{"milestone:v2", "status:ready"}) {
		t.Errorf("labels = %v", labels)
	}
}

func TestSetClearsRemovesAndAddsPlainLabels(t *testing.T) {
	newTestRepo(t)
	writeConfig(t, fieldsConfig)
	id := openIssueID(t)

	out, err := execute(t, []string{"set", id, "prio=p1", "assignee=+alice", "assignee=+@me", "label=+urgent", "--me", "bob"})
	if err != nil {
		t.Fatalf("set: %v\n%s", err, out)
	}
	if labels, _ := labelsOf(t, id); !slices.Equal(labels, []string{"assignee:alice", "assignee:bob", "prio:p1", "status:ready", "urgent"}) {
		t.Fatalf("labels = %v", labels)
	}
	out, err = execute(t, []string{"set", id, "prio=", "assignee=-@me", "label=-urgent", "--me", "bob"})
	if err != nil {
		t.Fatalf("set: %v\n%s", err, out)
	}
	for _, want := range []string{"prio cleared", "-assignee:bob", "-urgent"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if labels, _ := labelsOf(t, id); !slices.Equal(labels, []string{"assignee:alice", "status:ready"}) {
		t.Errorf("labels = %v, want [assignee:alice status:ready]", labels)
	}
}

// set status= and close never disagree: the last option closes, another
// one reopens.
func TestSetStatusKeepsTheStateInStep(t *testing.T) {
	newTestRepo(t)
	writeConfig(t, fieldsConfig)
	id := openIssueID(t)

	out, err := execute(t, []string{"set", id, "status=validated"})
	if err != nil || !strings.Contains(out, "closed") {
		t.Fatalf("set status=validated: %v\n%s", err, out)
	}
	if _, state := labelsOf(t, id); state != "closed" {
		t.Errorf("state = %s, want closed", state)
	}
	out, err = execute(t, []string{"set", id, "status=backlog"})
	if err != nil || !strings.Contains(out, "reopened") {
		t.Fatalf("set status=backlog: %v\n%s", err, out)
	}
	if _, state := labelsOf(t, id); state != "open" {
		t.Errorf("state = %s, want open", state)
	}
}

// set --filter shows what it would do, and writes it with --apply only.
func TestSetFilterPreviewsThenApplies(t *testing.T) {
	newTestRepo(t)
	writeConfig(t, fieldsConfig)
	id := openIssueID(t)
	if _, err := execute(t, []string{"new", "Second", "status=ready", "prio=p2"}); err != nil {
		t.Fatal(err)
	}

	out, err := execute(t, []string{"set", "--filter", "status:ready", "prio=p0"})
	if err != nil || !strings.Contains(out, "2 issues would change; --apply") || !strings.Contains(out, "+prio:p0 -prio:p2") {
		t.Fatalf("set --filter: %v\n%s", err, out)
	}
	if labels, _ := labelsOf(t, id); slices.Contains(labels, "prio:p0") {
		t.Fatalf("set --filter wrote without --apply: %v", labels)
	}
	out, err = execute(t, []string{"set", "--filter", "status:ready", "prio=p0", "--apply"})
	if err != nil || !strings.Contains(out, "2 issues changed") {
		t.Fatalf("set --filter --apply: %v\n%s", err, out)
	}
	if labels, _ := labelsOf(t, id); !slices.Contains(labels, "prio:p0") {
		t.Errorf("labels = %v, want prio:p0", labels)
	}
	if out, err := execute(t, []string{"set", "--filter", "status:ready", "prio=p0"}); err != nil || !strings.Contains(out, "nothing to change on the 2 issues") {
		t.Errorf("set --filter again: %v\n%s", err, out)
	}
	if _, err := execute(t, []string{"set", "--filter", "status:ready", "prio=p9", "--apply"}); exitCode(err) != exitUsage {
		t.Errorf("set --filter prio=p9 = %v, want exit %d", err, exitUsage)
	}
	if _, err := execute(t, []string{"set", id, "prio=p1", "--apply"}); exitCode(err) != exitUsage {
		t.Errorf("set <id> --apply = %v, want exit %d", err, exitUsage)
	}
}
