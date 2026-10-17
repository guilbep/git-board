package main

import (
	"os"
	"testing"
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
	name := id[:7] + "-fix-invoice-bug"
	if want := name + "\ngit switch -c " + name + " origin/dev\n"; out != want {
		t.Errorf("output %q, want %q", out, want)
	}
}
