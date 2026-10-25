package github

import (
	"cmp"
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

// liveProjects are the only projects the live test may write to.
var liveProjects = []string{"guilbep/git-board-test"}

// TestLive runs a short two-way sync with GitHub, with the token from
// GITHUB_TOKEN or git's credential helper. It runs only with BOARD_LIVE=1,
// on guilbep/git-board-test, or on BOARD_LIVE_PROJECT when liveProjects
// has it. It leaves one closed issue there, labelled live-test.
func TestLive(t *testing.T) {
	if os.Getenv("BOARD_LIVE") != "1" {
		t.Skip("BOARD_LIVE=1 runs it against GitHub")
	}
	project := cmp.Or(os.Getenv("BOARD_LIVE_PROJECT"), liveProjects[0])
	if !slices.Contains(liveProjects, project) {
		t.Fatalf("refusing %s: the live test writes only to %v", project, liveProjects)
	}
	ctx := t.Context()
	start := time.Now().Unix()
	a, err := New(ctx, Options{Project: project})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	s, err := gitstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := people.Create(ctx, s, people.NewCreate("", "live test", ""))
	if err != nil {
		t.Fatal(err)
	}
	me := p.ID
	var warnings []string
	opt := syncer.Options{Warn: func(w string) { warnings = append(warnings, w) }}
	run := func() syncer.Report {
		t.Helper()
		r, err := syncer.Sync(ctx, s, opt, a)
		if err != nil {
			t.Fatalf("sync: %v", err)
		}
		t.Logf("sync: %+v", r)
		return r
	}
	title := "git-board live test " + time.Now().UTC().Format("2006-01-02 15:04:05")

	// git-board to GitHub: an issue, its comment and a label.
	is, err := issue.Create(ctx, s, issue.NewCreate(me, title, "Made by core/forge/github's live test."),
		issue.NewComment(me, "a comment from git-board"), issue.ChangeLabels(me, []string{"live-test"}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if r := run(); r.Posted != 1 {
		t.Fatalf("posted %d issues, want 1", r.Posted)
	}
	// GitHub's since listing trails a new issue by a few seconds.
	var list []forge.Issue
	i := -1
	for try := 0; i < 0 && try < 15; try++ {
		if try > 0 {
			time.Sleep(2 * time.Second)
		}
		if list, err = a.Issues(ctx, start); err != nil {
			t.Fatal(err)
		}
		i = slices.IndexFunc(list, func(fi forge.Issue) bool { return fi.Title == title })
	}
	if i < 0 {
		t.Fatalf("%q isn't on GitHub", title)
	}
	gi := list[i]
	t.Logf("on GitHub: %s", gi.URL)
	if !strings.Contains(gi.Body, "<!-- git-board issue "+is.ID) || !slices.Contains(gi.Labels, "live-test") || len(gi.Comments) != 1 {
		t.Fatalf("on GitHub: body %q, labels %v, %d comments", gi.Body, gi.Labels, len(gi.Comments))
	}

	// GitHub to git-board: a rename, and a comment with a browser's \r\n.
	if err := a.SetTitle(ctx, gi.ID, title+" (renamed on GitHub)"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateComment(ctx, gi.ID, "a comment from GitHub\r\non two lines"); err != nil {
		t.Fatal(err)
	}
	run()
	got, err := issue.Load(s, is.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != title+" (renamed on GitHub)" || len(got.Comments) != 2 || got.Comments[1].Body != "a comment from GitHub\non two lines" {
		t.Fatalf("in git-board: %q, %d comments", got.Title, len(got.Comments))
	}

	// git-board closes it; the run after writes nothing.
	if _, err := issue.Do(ctx, s, is.ID, issue.SetStatus(me, issue.Closed)); err != nil {
		t.Fatal(err)
	}
	run()
	if fi, err := a.Issue(ctx, gi.ID); err != nil || fi.Status != "closed" {
		t.Fatalf("on GitHub: %q, %v; want closed", fi.Status, err)
	}
	if r := run(); r.Created+r.Updated+r.Posted+r.Pushed != 0 {
		t.Fatalf("the run after wrote: %+v", r)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings: %q", warnings)
	}
}
