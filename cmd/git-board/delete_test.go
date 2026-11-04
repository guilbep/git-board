package main

import (
	"bytes"
	"io"
	"slices"
	"strings"
	"testing"
)

// executeIn is execute with stdin, at a terminal or not.
func executeIn(t *testing.T, stdin string, tty bool, args []string) (string, error) {
	t.Helper()
	was := isTerminal
	isTerminal = func(io.Reader) bool { return tty }
	t.Cleanup(func() { isTerminal = was })
	cmd := newRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestDeleteIssue(t *testing.T) {
	newTestRepo(t)
	id := openIssueID(t)

	if _, err := executeIn(t, "", false, []string{"delete", id}); exitCode(err) != exitUsage {
		t.Fatalf("delete with no terminal and no --yes = %v, want exit %d", err, exitUsage)
	}
	if _, err := executeIn(t, "n\n", true, []string{"delete", id}); err == nil || !strings.Contains(err.Error(), "aborted") {
		t.Fatalf("delete answered no = %v, want aborted", err)
	}
	if _, err := execute(t, []string{"show", id}); err != nil {
		t.Fatalf("after a no, show = %v", err)
	}
	out, err := executeIn(t, "y\n", true, []string{"delete", id})
	if err != nil || !strings.Contains(out, "deleted "+id[:8]) || !strings.Contains(out, "history") {
		t.Fatalf("delete answered yes = %q, %v", out, err)
	}
	if _, err := execute(t, []string{"show", id}); exitCode(err) != exitNotFound {
		t.Errorf("show of the deleted issue = %v, want exit %d", err, exitNotFound)
	}
	if out, _ := execute(t, []string{"ls", "is:open"}); strings.Contains(out, "invoice") {
		t.Errorf("ls shows the deleted issue:\n%s", out)
	}
	if _, err := execute(t, []string{"delete", id, "--yes"}); exitCode(err) != exitNotFound {
		t.Errorf("deleting it again = %v, want exit %d", err, exitNotFound)
	}
}

func TestCommentDelete(t *testing.T) {
	newTestRepo(t)
	id := openIssueID(t)
	out, err := execute(t, []string{"comment", id, "-b", "token=hunter2"})
	if err != nil {
		t.Fatal(err)
	}
	ref := strings.TrimSpace(out)
	if _, err := execute(t, []string{"comment", id, "-b", "kept"}); err != nil {
		t.Fatal(err)
	}
	if _, err := execute(t, []string{"comment", id, "--delete", ref, "-b", "x"}); exitCode(err) != exitUsage {
		t.Errorf("--delete with a text = %v, want exit %d", err, exitUsage)
	}
	if _, err := executeIn(t, "", false, []string{"comment", id, "--delete", ref}); exitCode(err) != exitUsage {
		t.Errorf("--delete with no terminal and no --yes = %v, want exit %d", err, exitUsage)
	}
	if out, err := execute(t, []string{"comment", id, "--delete", ref, "--yes"}); err != nil || !strings.Contains(out, "deleted "+ref) {
		t.Fatalf("comment --delete = %q, %v", out, err)
	}
	if got := comments(threadOf(t, id)); !slices.Equal(got, []string{"kept"}) {
		t.Errorf("comments = %q", got)
	}
	if out, _ := execute(t, []string{"show", id}); strings.Contains(out, "hunter2") {
		t.Errorf("show prints the deleted comment:\n%s", out)
	}
	if _, err := execute(t, []string{"comment", id, "--delete", ref, "--yes"}); exitCode(err) != exitNotFound {
		t.Errorf("deleting it again = %v, want exit %d", err, exitNotFound)
	}
}
