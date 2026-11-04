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

// --reason is a closed-as label (completed none); --duplicate-of also
// links and comments; reopen takes closed-as away.
func TestCloseReasons(t *testing.T) {
	newTestRepo(t)
	writeStatusOptionsConfig(t)
	id := openIssueID(t)
	other := listIDs(t)
	var dup string
	for _, o := range other {
		if o != id {
			dup = o
		}
	}

	if out, err := execute(t, []string{"close", id, "--reason", "not planned"}); err != nil || !strings.Contains(out, "closed-as:not-planned") {
		t.Fatalf("close --reason 'not planned': %v\n%s", err, out)
	}
	if labels, state := labelsOf(t, id); state != "closed" || !containsLabel(labels, "closed-as:not-planned") {
		t.Errorf("after not-planned: %v %s", labels, state)
	}
	if out, err := execute(t, []string{"ls", "closed-as:not-planned is:closed"}); err != nil || !strings.Contains(out, "Fix invoice bug") {
		t.Errorf("ls closed-as:not-planned: %v\n%s", err, out)
	}

	// completed, again: the earlier reason goes
	if _, err := execute(t, []string{"close", id}); err != nil {
		t.Fatal(err)
	}
	if labels, _ := labelsOf(t, id); slices.ContainsFunc(labels, func(l string) bool { return strings.HasPrefix(l, "closed-as:") }) {
		t.Errorf("closed as completed keeps %v", labels)
	}

	if out, err := execute(t, []string{"close", id, "--duplicate-of", dup[:7]}); err != nil {
		t.Fatalf("close --duplicate-of: %v\n%s", err, out)
	}
	th := threadOf(t, id)
	if !containsLabel(th.Labels, "closed-as:duplicate") || !containsLabel(th.Labels, "duplicate-of:"+dup[:8]) {
		t.Errorf("duplicate labels = %v", th.Labels)
	}
	if c := comments(th); len(c) != 1 || c[0] != "Duplicate of "+dup[:8] {
		t.Errorf("comments = %q", c)
	}

	for _, args := range [][]string{
		{"close", id, "--reason", "wontfix"},
		{"close", id, "--reason", "completed", "--duplicate-of", dup[:7]},
		{"close", id, "--duplicate-of", id[:7]},
	} {
		if _, err := execute(t, args); exitCode(err) != exitUsage {
			t.Errorf("%v = %v, want exit %d", args, err, exitUsage)
		}
	}
	if _, err := execute(t, []string{"close", id, "--duplicate-of", "0000000"}); exitCode(err) != exitNotFound {
		t.Errorf("--duplicate-of nothing = %v, want exit %d", err, exitNotFound)
	}

	if _, err := execute(t, []string{"reopen", id}); err != nil {
		t.Fatal(err)
	}
	if labels, state := labelsOf(t, id); state != "open" || containsLabel(labels, "closed-as:duplicate") || !containsLabel(labels, "duplicate-of:"+dup[:8]) {
		t.Errorf("after reopen: %v %s", labels, state)
	}
}
