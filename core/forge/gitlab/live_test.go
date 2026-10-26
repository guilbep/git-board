package gitlab

import (
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/people"
	"github.com/guilbep/git-board/core/syncer"
)

// live is the only project the live test may write to.
var live = map[string]bool{"gitlab.epfl.ch/42400": true}

// TestLive is a short two-way sync with gitlab.epfl.ch project 42400, run
// only with BOARD_LIVE=1. Its token is GITLAB_TOKEN, else the credential
// helper's for gitlab.epfl.ch, with BOARD_GITLAB_LOGIN as the username when
// set. It creates one issue, labelled live-board and assigned to the
// posting account, and closes it.
func TestLive(t *testing.T) {
	t.Parallel()
	if os.Getenv("BOARD_LIVE") != "1" {
		t.Skip("BOARD_LIVE=1 runs it, on gitlab.epfl.ch project 42400")
	}
	ctx := t.Context()
	a, err := New(ctx, "https://gitlab.epfl.ch", "42400", os.Getenv("BOARD_GITLAB_LOGIN"))
	if err != nil {
		t.Fatal(err) // never holds the token
	}
	if !live[a.Key()] {
		t.Fatalf("refusing %s: the live test writes to gitlab.epfl.ch/42400 only", a.Key())
	}
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	s, err := gitstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	me, err := people.Create(ctx, s, people.NewCreate("", "live", ""))
	if err != nil {
		t.Fatal(err)
	}
	title := "git-board core live test " + time.Now().UTC().Format(time.RFC3339)
	is, err := issue.Create(ctx, s, issue.NewCreate(me.ID, title, "Made by the GitLab adapter's live test."),
		issue.NewComment(me.ID, "a git-board comment"), issue.ChangeLabels(me.ID, []string{"live-test"}, nil))
	if err != nil {
		t.Fatal(err)
	}
	sync := func() syncer.Report {
		t.Helper()
		r, err := syncer.Sync(ctx, s, syncer.Options{Warn: func(w string) { t.Log("warning:", w) }}, a)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("report: %+v", r)
		return r
	}
	find := func(title string) forge.Issue {
		t.Helper()
		list, err := a.Issues(ctx, time.Now().Unix()-600)
		if err != nil {
			t.Fatal(err)
		}
		for _, fi := range list {
			if fi.Title == title {
				return fi
			}
		}
		t.Fatalf("no %q on GitLab", title)
		return forge.Issue{}
	}

	// git-board's issue goes to GitLab.
	if r := sync(); r.Posted != 1 {
		t.Fatalf("posted %d issues, want 1", r.Posted)
	}
	fi := find(title)
	if len(fi.Comments) != 1 || !strings.Contains(fi.Comments[0].Body, "a git-board comment") || !slices.Contains(fi.Labels, "live-test") {
		t.Fatalf("on GitLab: %+v", fi)
	}
	// Labels, each way: one added and one removed on GitLab, then in
	// git-board, with an assignee, the posting account, read back.
	if err := a.ChangeLabels(ctx, fi.ID, []string{"live-forge"}, []string{"live-test"}); err != nil {
		t.Fatal(err)
	}
	sync()
	if got, err := issue.Load(s, is.ID); err != nil || !slices.Contains(got.Labels, "live-forge") || slices.Contains(got.Labels, "live-test") {
		t.Fatalf("in git-board: labels %v, %v", got.Labels, err)
	}
	acc, err := a.Me(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := issue.Do(ctx, s, is.ID, issue.ChangeLabels(me.ID, []string{"live-board", "assignee:" + acc.Login}, []string{"live-forge"})); err != nil {
		t.Fatal(err)
	}
	sync()
	if on, err := a.Issue(ctx, fi.ID); err != nil || !slices.Contains(on.Labels, "live-board") || slices.Contains(on.Labels, "live-forge") ||
		!slices.ContainsFunc(on.Assignees, func(x forge.Account) bool { return x.ID == acc.ID }) {
		t.Fatalf("on GitLab: labels %v, assignees %v, %v", on.Labels, on.Assignees, err)
	}
	// A GitLab comment comes back; a rename and a close go.
	if _, err := a.CreateComment(ctx, fi.ID, "a GitLab comment"); err != nil {
		t.Fatal(err)
	}
	if _, err := issue.Do(ctx, s, is.ID, issue.SetTitle(me.ID, title+" (closed)"), issue.SetStatus(me.ID, issue.Closed)); err != nil {
		t.Fatal(err)
	}
	sync()
	got, err := issue.Load(s, is.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Comments) != 2 || got.Comments[1].Body != "a GitLab comment" {
		t.Fatalf("in git-board: %+v", got.Comments)
	}
	if on, err := a.Issue(ctx, fi.ID); err != nil || on.Title != title+" (closed)" || on.Status != "closed" {
		t.Fatalf("on GitLab: %q %q, %v", on.Title, on.Status, err)
	}
	// The run after writes nothing.
	if r := sync(); r.Created+r.Updated+r.Posted+r.Pushed != 0 {
		t.Fatalf("the run after wrote: %+v", r)
	}
}
