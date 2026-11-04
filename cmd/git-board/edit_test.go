package main

import (
	"strings"
	"testing"
)

func TestEditTitleAndDescription(t *testing.T) {
	newTestRepo(t)
	id := openIssueID(t)

	if out, err := execute(t, []string{"edit", id, "--title", "Fix the invoice bug", "-b", "Steps."}); err != nil || !strings.Contains(out, "title and description edited") {
		t.Fatalf("edit: %v\n%s", err, out)
	}
	opened := editorWrites(t, "Renamed\n\nNew steps.\n")
	if _, err := execute(t, []string{"edit", id, "-e"}); err != nil {
		t.Fatalf("edit -e: %v", err)
	}
	if *opened != "Fix the invoice bug\n\nSteps.\n" {
		t.Errorf("editor opened on %q", *opened)
	}
	th := threadOf(t, id)
	if th.Title != "Renamed" || th.Events[0].Body != "New steps." || len(th.Events[0].History) != 3 {
		t.Errorf("after edits: %q %q, %d versions", th.Title, th.Events[0].Body, len(th.Events[0].History))
	}
	if _, err := execute(t, []string{"edit", id}); exitCode(err) != exitUsage {
		t.Errorf("edit with nothing to change = %v, want exit %d", err, exitUsage)
	}
	if out, err := execute(t, []string{"edit", id, "--title", "Renamed"}); err != nil || !strings.Contains(out, "unchanged") {
		t.Errorf("edit to the same title: %v\n%s", err, out)
	}
}
