package gitbugio_test

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

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
	t.Parallel()
	b := gitbugiotest.New(t)
	id, err := b.NewIssue("Export CSV", "the **body**", []string{"status:ready"})
	if err != nil {
		t.Fatal(err)
	}
	comment, err := b.AddComment(id, "first try")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.EditComment(id, comment, "second try", "first try"); err != nil {
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
	if err := b.EditComment(id, created.ID, "the new body", ""); err != nil {
		t.Fatal(err)
	}
	if th, _ = b.Thread(id); th.Events[0].Body != "the new body" || !th.Events[0].Edited {
		t.Errorf("description %+v", th.Events[0])
	}
}

func TestEditingAnotherIssuesCommentIsRefused(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	one, _ := b.NewIssue("one", "", nil)
	two, _ := b.NewIssue("two", "", nil)
	comment, err := b.AddComment(one, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.EditComment(two, comment, "hijacked", ""); !errors.Is(err, gitbugio.ErrNoSuchComment) {
		t.Errorf("err = %v, want ErrNoSuchComment", err)
	}
}

func TestThreadForgeLinks(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "synced")
	gitbugiotest.Link(t, b, id, "https://gitlab.example.com/g/r/-/work_items/3")
	gitbugiotest.Link(t, b, id, "https://github.com/o/r/issues/7")
	th, err := b.Thread(id)
	if err != nil {
		t.Fatal(err)
	}
	want := []gitbugio.Forge{
		{Name: "GitHub", URL: "https://github.com/o/r/issues/7", Number: "7"},
		{Name: "GitLab", URL: "https://gitlab.example.com/g/r/-/work_items/3", Number: "3"},
	}
	if !slices.Equal(th.Forges, want) || th.GithubURL != want[0].URL {
		t.Errorf("forges %+v, GitHub %q; want %+v", th.Forges, th.GithubURL, want)
	}
	if !th.HasForgeNumber("7") || !th.HasForgeNumber("3") || th.HasForgeNumber("8") {
		t.Errorf("forge numbers of %+v", th.Forges)
	}
}

func TestMe(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	me, ok, err := b.Me()
	if err != nil || !ok || me.Name == "" || me.ID == "" {
		t.Errorf("Me = %+v, %v, %v", me, ok, err)
	}
}

func TestForgeToken(t *testing.T) {
	t.Skip("M5 phase 2")
}

func TestIdentities(t *testing.T) {
	t.Parallel()
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

// An edit says which text it started from: if someone changed the comment
// since, it's refused, not silently replacing theirs.
func TestEditingAChangedCommentIsRefused(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	id, _ := b.NewIssue("x", "", nil)
	comment, _ := b.AddComment(id, "v1")
	if err := b.EditComment(id, comment, "v2", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := b.EditComment(id, comment, "mine", "v1"); !errors.Is(err, gitbugio.ErrChangedMeanwhile) {
		t.Errorf("err = %v, want ErrChangedMeanwhile", err)
	}
}

// @me is the forge login of the person git-board writes as: their
// account's, none without one.
func TestMyLogin(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	if got := b.MyLogin(); got != "" {
		t.Errorf("MyLogin = %q without an account, want empty", got)
	}
	alice := gitbugiotest.NewPerson(t, b, "Alice", "alice")
	if err := b.Adopt(alice.ID[:10]); err != nil {
		t.Fatal(err)
	}
	if got := b.MyLogin(); got != "alice" {
		t.Errorf("MyLogin = %q, want alice", got)
	}
}

// An edit waiting for a forge says what the forge has now: the newest
// version that reached it, under any number of edits still waiting.
func TestThreadEditWasIsWhatTheForgesHave(t *testing.T) {
	t.Skip("M5 phase 2")
}

func TestCommentMeta(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	id, err := b.NewIssue("Export CSV", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddComment(id, "plain"); err != nil {
		t.Fatal(err)
	}
	meta := map[string]string{"signoff:status": "qa", "signoff:commit": "4f2e9c1"}
	if _, err := b.AddCommentMeta(id, "Signed off qa at 4f2e9c1", meta); err != nil {
		t.Fatal(err)
	}
	th, err := b.Thread(id)
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(th); !slices.Equal(got, []string{"created", "comment", "comment"}) {
		t.Fatalf("kinds = %v", got)
	}
	if th.Events[1].Meta != nil {
		t.Errorf("a plain comment's Meta = %v", th.Events[1].Meta)
	}
	if got := th.Events[2].Meta; !maps.Equal(got, meta) {
		t.Errorf("Meta = %v, want %v", got, meta)
	}
	imported := gitbugiotest.NewImportedIssue(t, b, "Imported", "https://github.com/o/r/issues/1")
	th, err = b.Thread(imported)
	if err != nil {
		t.Fatal(err)
	}
	for k := range th.Events[0].Meta {
		if strings.HasPrefix(k, "sync:") {
			t.Errorf("created event carries %s", k)
		}
	}
}
