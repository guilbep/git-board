package store_test

import (
	"errors"
	"maps"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

func TestListIssuesAndGetLabels(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	id1 := storetest.NewIssue(t, b, "Fix invoice bug")
	id2 := storetest.NewIssue(t, b, "Write specs")
	storetest.CloseIssue(t, b, id2)

	if _, err := b.ApplyLabelChange(id1, []string{"status:ready"}, nil); err != nil {
		t.Fatalf("ApplyLabelChange: %v", err)
	}

	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("len(issues) = %d, want 2", len(issues))
	}

	byTitle := map[string]store.Issue{}
	for _, i := range issues {
		byTitle[i.Title] = i
	}

	fix := byTitle["Fix invoice bug"]
	if fix.Status != "open" {
		t.Errorf("fix.Status = %q, want open", fix.Status)
	}
	if len(fix.Labels) != 1 || fix.Labels[0] != "status:ready" {
		t.Errorf("fix.Labels = %v, want [status:ready]", fix.Labels)
	}

	specs := byTitle["Write specs"]
	if specs.Status != "closed" {
		t.Errorf("specs.Status = %q, want closed", specs.Status)
	}

	labels, err := b.GetLabels(id1)
	if err != nil {
		t.Fatalf("GetLabels: %v", err)
	}
	if len(labels) != 1 || labels[0] != "status:ready" {
		t.Errorf("GetLabels = %v", labels)
	}
}

func TestApplyLabelChangeAddAndRemove(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")

	labels, err := b.ApplyLabelChange(id, []string{"status:backlog", "prio:p1"}, nil)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("labels after add = %v", labels)
	}

	labels, err = b.ApplyLabelChange(id, []string{"status:ready"}, []string{"status:backlog"})
	if err != nil {
		t.Fatalf("swap: %v", err)
	}
	want := map[string]bool{"status:ready": true, "prio:p1": true}
	if len(labels) != 2 {
		t.Fatalf("labels after swap = %v", labels)
	}
	for _, l := range labels {
		if !want[l] {
			t.Errorf("unexpected label %q in %v", l, labels)
		}
	}
}

func TestApplyLabelChangeNoOpDoesNotError(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")

	if _, err := b.ApplyLabelChange(id, nil, nil); err != nil {
		t.Fatalf("empty add/remove should be a no-op, got err: %v", err)
	}
}

func TestListIssuesExposesGithubURL(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	storetest.NewIssue(t, b, "local only")
	storetest.NewImportedIssue(t, b, "from github", "https://github.com/o/r/issues/42")

	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	byTitle := map[string]store.Issue{}
	for _, i := range issues {
		byTitle[i.Title] = i
	}
	if byTitle["local only"].GithubURL != "" {
		t.Errorf("local only GithubURL = %q, want empty", byTitle["local only"].GithubURL)
	}
	if got := byTitle["from github"].GithubURL; got != "https://github.com/o/r/issues/42" {
		t.Errorf("GithubURL = %q, want https://github.com/o/r/issues/42", got)
	}
}

func TestGetIssueResolvesByIDPrefix(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "prefix test")

	issue, err := b.GetIssue(id[:8])
	if err != nil {
		t.Fatalf("GetIssue by prefix: %v", err)
	}
	if issue.Title != "prefix test" {
		t.Errorf("Title = %q", issue.Title)
	}
}

// What a GitHub Project and git-board last agreed on is kept per Project,
// the last record of a field winning, and an empty value is a value.
func TestProjectBasesRoundTrip(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "title")

	got, err := b.ProjectBases(id, "git-board:o/1:")
	if err != nil || len(got) != 0 {
		t.Fatalf("fresh bases = %v, %v", got, err)
	}
	if err := b.SetProjectBases(id, "git-board:o/1:", map[string]string{"status": "ready", "prio": "p1"}); err != nil {
		t.Fatal(err)
	}
	if err := b.SetProjectBases(id, "git-board:o/1:", map[string]string{"status": ""}); err != nil {
		t.Fatal(err)
	}
	if err := b.SetProjectBases(id, "git-board:o/2:", map[string]string{"status": "qa"}); err != nil {
		t.Fatal(err)
	}

	got, err = b.ProjectBases(id, "git-board:o/1:")
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"status": "", "prio": "p1"}; !maps.Equal(got, want) {
		t.Errorf("bases = %v, want %v", got, want)
	}
}

func TestShownTitle(t *testing.T) {
	t.Parallel()
	gh := []store.Forge{{Name: "GitHub", Number: "2518"}}
	tests := []struct {
		title  string
		forges []store.Forge
		want   string
	}{
		{"[BUG](Equipment) Wrong example (#2518)", gh, "[BUG](Equipment) Wrong example"},
		{"Wrong example (#2518)  ", gh, "Wrong example"},
		{"Wrong example (#45)", gh, "Wrong example (#45)"},      // another issue's number
		{"Fix (#2518) first", gh, "Fix (#2518) first"},          // only a trailing one
		{"Wrong example (#2518)", nil, "Wrong example (#2518)"}, // no forge number to tell
	}
	for _, tt := range tests {
		is := store.Issue{Title: tt.title, Forges: tt.forges}
		if got := is.ShownTitle(); got != tt.want {
			t.Errorf("ShownTitle(%q) = %q, want %q", tt.title, got, tt.want)
		}
	}
}

func TestListIssuesShowsTitlesWithoutTheirNumber(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	storetest.NewImportedIssue(t, b, "Wrong example (#42)", "https://github.com/o/r/issues/42")
	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if is := issues[0]; is.Shown != "Wrong example" || is.Title != "Wrong example (#42)" {
		t.Errorf("Shown, Title = %q, %q; want the title without (#42), and the title as stored", is.Shown, is.Title)
	}
}

// What the HTTP API hands on is checked before the core sees it: an id
// prefix is hex, a comment id is <pack>:<n>, a remote is one git has.
func TestInputsAreChecked(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")
	for _, p := range []string{"", "%", "a_", "../x", strings.ToUpper(id[:8]), id + "0"} {
		if _, err := b.GetIssue(p); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("GetIssue(%q): %v, want ErrNotFound", p, err)
		}
	}
	for _, c := range []string{"", "x:0", id, id + ":", id + ":-1", id[:39] + ":0"} {
		if err := b.EditComment(id, c, "y", ""); !errors.Is(err, store.ErrNoSuchComment) {
			t.Errorf("EditComment(%q): %v, want ErrNoSuchComment", c, err)
		}
	}
	for _, r := range []string{"", "--upload-pack=touch x", "origin", "a/../b"} {
		if err := b.Pull(r); err == nil {
			t.Errorf("Pull(%q) took it", r)
		}
	}
}
