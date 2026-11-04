package main

import (
	"os"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/store/storetest"
)

func TestBranchPrintsTheNameAndTheCommand(t *testing.T) {
	newTestRepo(t)
	if err := os.WriteFile(configPath, []byte("[branches]\nbase = \"dev\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	id := openIssueID(t)

	out, err := execute(t, []string{"branch", id[:7]})
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}
	name := id[:7] + "/1-fix-invoice-bug" // in the issue's folder (docs/design/issue-head, decision 3)
	if want := name + "\ngit switch -c " + name + " origin/dev\n"; out != want {
		t.Errorf("output %q, want %q", out, want)
	}
}

// --create creates the issue's branch from the base and switches to it;
// a second run, its next branch.
func TestBranchCreateSwitches(t *testing.T) {
	newTestRepo(t)
	storetest.Git(t, ".", "commit", "-q", "--allow-empty", "-m", "init")
	storetest.Git(t, ".", "branch", "-M", "main")
	id := openIssueID(t)

	out, err := execute(t, []string{"branch", id[:7], "--create"})
	name := id[:7] + "/1-fix-invoice-bug"
	if err != nil || strings.TrimSpace(out) != name {
		t.Fatalf("branch --create: %v\n%s", err, out)
	}
	if head := strings.TrimSpace(storetest.Git(t, ".", "branch", "--show-current")); head != name {
		t.Errorf("on %q, want %q", head, name)
	}
	out, err = execute(t, []string{"branch", id[:7], "--create"})
	if err != nil || strings.TrimSpace(out) != id[:7]+"/2-fix-invoice-bug" {
		t.Errorf("second branch --create: %v\n%s", err, out)
	}
}
