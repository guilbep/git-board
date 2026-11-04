package main

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/store"
)

// editorWrites makes the editor save text, recording what it was opened
// on, until the test ends.
func editorWrites(t *testing.T, text string) *string {
	t.Helper()
	var opened string
	was := runEditor
	runEditor = func(_, initial string) (string, error) { opened = initial; return text, nil }
	t.Cleanup(func() { runEditor = was })
	return &opened
}

// threadOf reads an issue's thread afresh.
func threadOf(t *testing.T, id string) store.Thread {
	t.Helper()
	b, err := store.OpenCwd()
	if err != nil {
		t.Fatalf("OpenCwd: %v", err)
	}
	defer b.Close()
	th, err := b.Thread(id)
	if err != nil {
		t.Fatalf("Thread: %v", err)
	}
	return th
}

func comments(th store.Thread) []string {
	var res []string
	for _, e := range th.Events {
		if e.Kind == "comment" {
			res = append(res, e.Body)
		}
	}
	return res
}

func TestNewCreatesWithBodyAndFields(t *testing.T) {
	newTestRepo(t)
	writeConfig(t, fieldsConfig)

	out, err := execute(t, []string{"new", "Login fails", "prio=p1", "assignee=+@me", "-b", "On Safari.", "--me", "ada"})
	if err != nil {
		t.Fatalf("new: %v\n%s", err, out)
	}
	id := strings.TrimSpace(out)
	th := threadOf(t, id)
	slices.Sort(th.Labels)
	if th.Title != "Login fails" || th.Events[0].Body != "On Safari." || !slices.Equal(th.Labels, []string{"assignee:ada", "prio:p1"}) {
		t.Errorf("new issue = %q %q %v", th.Title, th.Events[0].Body, th.Labels)
	}
	if n := slices.IndexFunc(th.Events, func(e store.Event) bool { return e.Kind == "labels" }); n < 0 {
		t.Errorf("events = %+v, want the labels", th.Events)
	}

	// --json is ls --json's issue
	out, err = execute(t, []string{"new", "--title", "a=b", "--json"})
	if err != nil {
		t.Fatalf("new --json: %v\n%s", err, out)
	}
	var is store.Issue
	if err := json.Unmarshal([]byte(out), &is); err != nil || is.Title != "a=b" || is.Status != "open" || len(is.ID) != 40 {
		t.Errorf("new --json = %s, %v", out, err)
	}

	// refused: nothing created
	before := len(listIDs(t))
	if _, err := execute(t, []string{"new", "Bad", "prio=p9"}); exitCode(err) != exitUsage {
		t.Errorf("new prio=p9 = %v, want exit %d", err, exitUsage)
	}
	if _, err := execute(t, []string{"new"}); exitCode(err) != exitUsage {
		t.Errorf("new with no title = %v, want exit %d", err, exitUsage)
	}
	if after := len(listIDs(t)); after != before {
		t.Errorf("%d issues after refusals, want %d", after, before)
	}
}

func TestNewFromTheEditorAndStdin(t *testing.T) {
	newTestRepo(t)
	opened := editorWrites(t, "\nFrom the editor\n\nIts body,\n\non two paragraphs.\n")
	out, err := execute(t, []string{"new", "Draft", "-e"})
	if err != nil {
		t.Fatalf("new -e: %v\n%s", err, out)
	}
	if *opened != "Draft\n\n" {
		t.Errorf("editor opened on %q, want the title first", *opened)
	}
	th := threadOf(t, strings.TrimSpace(out))
	if th.Title != "From the editor" || th.Events[0].Body != "Its body,\n\non two paragraphs." {
		t.Errorf("new -e = %q %q", th.Title, th.Events[0].Body)
	}

	editorWrites(t, "\n\n")
	if _, err := execute(t, []string{"new", "-e"}); !errors.Is(err, errAborted) {
		t.Errorf("new -e, saved empty = %v, want aborted", err)
	}

	cmd := newRootCmd()
	cmd.SetIn(strings.NewReader("piped body\n"))
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"new", "Piped", "-F", "-"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("new -F -: %v", err)
	}
	if th := threadOf(t, strings.TrimSpace(buf.String())); th.Events[0].Body != "piped body" {
		t.Errorf("new -F - body = %q", th.Events[0].Body)
	}
}

func listIDs(t *testing.T) []string {
	t.Helper()
	b, err := store.OpenCwd()
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	issues, err := b.ListIssues()
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, is := range issues {
		ids = append(ids, is.ID)
	}
	return ids
}
