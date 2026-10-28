package main

import (
	"slices"
	"strings"
	"testing"
)

// Pull takes the Project's fields into the labels and edits nothing on
// the Project; push sends git-board's change and leaves the labels.
// Serial: they chdir and swap the Project's reader and writer.
func TestProjectPullThenPush(t *testing.T) {
	newBridgeTestRepo(t)
	rec := withFakeProject(t, true)

	mustExecute(t, "project", "pull", "--check")
	if _, labels := onlyIssue(t); len(labels) != 0 {
		t.Fatalf("the check wrote labels %v", labels)
	}
	mustExecute(t, "project", "pull")
	id, labels := onlyIssue(t)
	if !slices.Contains(labels, "status:qa-validation") || len(rec.edits) != 0 {
		t.Fatalf("pull: labels %v, edits %v", labels, rec.edits)
	}

	// changed here since they agreed: a pull leaves it, a push sends it
	mustExecute(t, "set", id, "status=in-review")
	mustExecute(t, "project", "pull")
	if _, labels := onlyIssue(t); !slices.Contains(labels, "status:in-review") || len(rec.edits) != 0 {
		t.Fatalf("pull after a change here: labels %v, edits %v", labels, rec.edits)
	}
	mustExecute(t, "project", "push")
	if len(rec.edits) != 1 || rec.edits[0].Arg != "o-review" {
		t.Fatalf("push: edits %+v, want status set to In review", rec.edits)
	}
	if _, labels := onlyIssue(t); !slices.Contains(labels, "status:in-review") {
		t.Errorf("push changed the labels: %v", labels)
	}
}

func TestBridgeStillWorks(t *testing.T) {
	newBridgeTestRepo(t)
	withFakeProject(t, true)
	out := mustExecute(t, "bridge", "sync", "--apply")
	if !strings.Contains(out, "deprecated") {
		t.Errorf("no deprecation notice:\n%s", out)
	}
	if _, labels := onlyIssue(t); !slices.Contains(labels, "status:qa-validation") {
		t.Errorf("labels %v", labels)
	}
}

func TestProjectNeedsAProject(t *testing.T) {
	newBridgeTestRepo(t)
	out, err := execute(t, []string{"project", "pull"})
	if err == nil || !strings.Contains(err.Error(), "no [github-project]") || !strings.Contains(err.Error(), "git board project configure") {
		t.Fatalf("err = %v\n%s", err, out)
	}
}

// A conflict held names the command that resolves it: project's own.
func TestProjectConflictNamesProjectResolve(t *testing.T) {
	newBridgeTestRepo(t)
	withFakeProject(t, true)
	mustExecute(t, "project", "sync")
	id, _ := onlyIssue(t)
	mustExecute(t, "set", id, "status=in-review")
	mustExecute(t, "project", "sync")
	mustExecute(t, "set", id, "status=ready")
	out := mustExecute(t, "project", "sync")
	if !strings.Contains(out, "held: git board project resolve") {
		t.Fatalf("no project resolve in:\n%s", out)
	}
}
