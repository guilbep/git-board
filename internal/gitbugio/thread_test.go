package gitbugio_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/git-bug/git-bug/bridge/core/auth"

	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

func kinds(th gitbugio.Thread) []string {
	var res []string
	for _, e := range th.Events {
		res = append(res, e.Kind)
	}
	return res
}

func TestThreadTellsTheStory(t *testing.T) {
	b := gitbugiotest.New(t)
	id, err := b.NewIssue("Export CSV", "the **body**", []string{"status:ready"})
	if err != nil {
		t.Fatal(err)
	}
	comment, err := b.AddComment(id, "first try")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.EditComment(id, comment, "second try"); err != nil {
		t.Fatal(err)
	}
	if err := b.SetTitle(id, "Export CSV and XLSX"); err != nil {
		t.Fatal(err)
	}
	if err := b.SetStatus(id, "closed"); err != nil {
		t.Fatal(err)
	}

	th, err := b.Thread(id)
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(th); !slices.Equal(got, []string{"created", "labels", "comment", "title", "status"}) {
		t.Fatalf("events %v", got)
	}
	created, labels, c, title, status := th.Events[0], th.Events[1], th.Events[2], th.Events[3], th.Events[4]
	if created.Body != "the **body**" || created.Author.Name == "" || created.At.IsZero() {
		t.Errorf("created %+v", created)
	}
	if !slices.Equal(labels.Added, []string{"status:ready"}) {
		t.Errorf("labels %+v", labels)
	}
	if c.ID != comment || c.Body != "second try" || !c.Edited || len(c.History) != 2 || c.History[0].Body != "first try" {
		t.Errorf("comment %+v", c)
	}
	if title.Title != "Export CSV and XLSX" || title.Was != "Export CSV" {
		t.Errorf("title %+v", title)
	}
	if status.Status != "closed" || th.Status != "closed" || th.Title != "Export CSV and XLSX" {
		t.Errorf("status %+v, issue %s %q", status, th.Status, th.Title)
	}

	// the description is the created event's text, edited the same way
	if err := b.EditComment(id, created.ID, "the new body"); err != nil {
		t.Fatal(err)
	}
	if th, _ = b.Thread(id); th.Events[0].Body != "the new body" || !th.Events[0].Edited {
		t.Errorf("description %+v", th.Events[0])
	}
}

func TestEditingAnotherIssuesCommentIsRefused(t *testing.T) {
	b := gitbugiotest.New(t)
	one, _ := b.NewIssue("one", "", nil)
	two, _ := b.NewIssue("two", "", nil)
	comment, err := b.AddComment(one, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.EditComment(two, comment, "hijacked"); !errors.Is(err, gitbugio.ErrNoSuchComment) {
		t.Errorf("err = %v, want ErrNoSuchComment", err)
	}
}

func TestThreadForgeLinks(t *testing.T) {
	b := gitbugiotest.New(t)
	author, err := b.TestCache().GetUserIdentity()
	if err != nil {
		t.Fatal(err)
	}
	bug, _, err := b.TestCache().Bugs().NewRaw(author, 1, "synced", "", nil, map[string]string{
		"sync:github.com/o/r:id":     "I_1",
		"sync:github.com/o/r:url":    "https://github.com/o/r/issues/7",
		"sync:github.com/o/r:number": "7",
		"github-url":                 "https://github.com/o/r/issues/7",
		"gitlab-url":                 "https://gitlab.example.com/g/r/-/work_items/3",
	})
	if err != nil {
		t.Fatal(err)
	}
	th, err := b.Thread(bug.Id().String())
	if err != nil {
		t.Fatal(err)
	}
	want := []gitbugio.Forge{
		{Name: "GitHub", URL: "https://github.com/o/r/issues/7", Number: "7"},
		{Name: "GitLab", URL: "https://gitlab.example.com/g/r/-/work_items/3", Number: "3"},
	}
	if !slices.Equal(th.Forges, want) {
		t.Errorf("forges %+v, want %+v", th.Forges, want)
	}

	// a comment the sync hasn't posted yet waits for the forges the issue is on
	if _, err := b.AddComment(bug.Id().String(), "not posted"); err != nil {
		t.Fatal(err)
	}
	th, _ = b.Thread(bug.Id().String())
	if last := th.Events[len(th.Events)-1]; !slices.Equal(last.Pending, []string{"GitHub"}) {
		t.Errorf("pending %v, want GitHub", last.Pending)
	}
}

func TestMe(t *testing.T) {
	b := gitbugiotest.New(t)
	me, ok, err := b.Me()
	if err != nil || !ok || me.Name == "" || me.ID == "" {
		t.Errorf("Me = %+v, %v, %v", me, ok, err)
	}
}

func TestForgeToken(t *testing.T) {
	b := gitbugiotest.New(t)
	gh := auth.NewToken("github", "gh-secret")
	gl := auth.NewToken("gitlab", "gl-secret")
	gl.SetMetadata(auth.MetaKeyBaseURL, "https://gitlab.example.com/")
	for _, c := range []auth.Credential{gh, gl} {
		if err := auth.Store(b.TestCache(), c); err != nil {
			t.Fatal(err)
		}
	}
	for host, want := range map[string]string{"github.com": "gh-secret", "gitlab.example.com": "gl-secret", "elsewhere.example": ""} {
		if got := b.ForgeToken(host); got != want {
			t.Errorf("ForgeToken(%s) = %q, want %q", host, got, want)
		}
	}
	if got := b.ForgeHosts(); !slices.Contains(got, "gitlab.example.com") {
		t.Errorf("ForgeHosts = %v, want the GitLab instance", got)
	}
}

func TestIdentities(t *testing.T) {
	b := gitbugiotest.New(t)
	other, err := b.NewIdentity("Bob", "bob@example.com")
	if err != nil {
		t.Fatal(err)
	}
	all, err := b.Identities()
	if err != nil || len(all) != 2 {
		t.Fatalf("Identities = %v, %v", all, err)
	}
	if err := b.Adopt(other.ID); err != nil {
		t.Fatal(err)
	}
	if me, _, _ := b.Me(); me.ID != other.ID {
		t.Errorf("Me = %+v, want Bob", me)
	}
}
