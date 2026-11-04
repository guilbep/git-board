package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/store"
)

func TestCloseSetsTerminalStatusAndCloses(t *testing.T) {
	newTestRepo(t)
	writeStatusOptionsConfig(t)
	id := openIssueID(t)

	out, err := execute(t, []string{"close", id})
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "status:validated") {
		t.Errorf("output = %q, want status:validated (the last configured option)", out)
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
	if !containsLabel(labels, "status:validated") {
		t.Errorf("labels = %v, want status:validated", labels)
	}
	if containsLabel(labels, "status:ready") {
		t.Errorf("labels = %v, want status:ready gone (single-cardinality swap)", labels)
	}

	issue, err := b.GetIssue(id)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if issue.Status != "closed" {
		t.Errorf("issue.Status = %q, want closed", issue.Status)
	}
}

func TestCloseRequiresConfiguredStatusOptions(t *testing.T) {
	newTestRepo(t) // testConfig has [fields.status] with no options
	id := openIssueID(t)

	_, err := execute(t, []string{"close", id})
	if err == nil {
		t.Fatal("expected an error with no [fields.status] options configured, got nil")
	}
}

func containsLabel(labels []string, label string) bool {
	for _, l := range labels {
		if l == label {
			return true
		}
	}
	return false
}

func TestCloseWithCommentAndReopen(t *testing.T) {
	newTestRepo(t)
	writeStatusOptionsConfig(t)
	id := openIssueID(t)

	if out, err := execute(t, []string{"close", id, "-c", "Fixed in 4e5f6a7"}); err != nil {
		t.Fatalf("close -c: %v\n%s", err, out)
	}
	th := threadOf(t, id)
	if th.Status != "closed" || !slices.Equal(comments(th), []string{"Fixed in 4e5f6a7"}) {
		t.Fatalf("after close -c: %s, %q", th.Status, comments(th))
	}
	out, err := execute(t, []string{"reopen", id, "-c", "Not on Safari"})
	if err != nil || !strings.Contains(out, "status:backlog") {
		t.Fatalf("reopen: %v\n%s", err, out)
	}
	if labels, state := labelsOf(t, id); state != "open" || !slices.Equal(labels, []string{"status:backlog"}) {
		t.Errorf("after reopen: %s %v", state, labels)
	}

	// a status that isn't the terminal one stays
	if _, err := execute(t, []string{"set", id, "status=in-progress"}); err != nil {
		t.Fatal(err)
	}
	b, err := store.OpenCwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := b.CloseBug(id); err != nil {
		t.Fatal(err)
	}
	b.Close()
	if _, err := execute(t, []string{"reopen", id}); err != nil {
		t.Fatal(err)
	}
	if labels, state := labelsOf(t, id); state != "open" || !slices.Equal(labels, []string{"status:in-progress"}) {
		t.Errorf("after reopen: %s %v, want in-progress kept", state, labels)
	}
}
