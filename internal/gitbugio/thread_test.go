package gitbugio_test

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/git-bug/git-bug/bridge/core"
	"github.com/git-bug/git-bug/bridge/core/auth"
	"github.com/git-bug/gitconfig"

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
	author, err := b.TestCache().GetUserIdentity(t.Context())
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
	if err := bug.Commit(t.Context()); err != nil {
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
	id := bug.Id().String()
	comment, err := b.AddComment(id, "not posted")
	if err != nil {
		t.Fatal(err)
	}
	th, _ = b.Thread(id)
	if last := th.Events[len(th.Events)-1]; !slices.Equal(last.Pending, []string{"GitHub"}) {
		t.Errorf("pending %v, want GitHub", last.Pending)
	}

	// once pushed it's there; its edit, and a status change, wait again
	cached, _ := b.TestCache().Bugs().ResolvePrefix(t.Context(), id)
	op := cached.Snapshot().Operations[len(cached.Snapshot().Operations)-1]
	if _, err := cached.SetMetadata(t.Context(), op.Id(), map[string]string{"sync:github.com/o/r:pushed": "1"}); err != nil {
		t.Fatal(err)
	}
	if err := cached.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := b.EditComment(id, comment, "edited", ""); err != nil {
		t.Fatal(err)
	}
	if err := b.SetStatus(id, "closed"); err != nil {
		t.Fatal(err)
	}
	th, _ = b.Thread(id)
	c, status := th.Events[len(th.Events)-2], th.Events[len(th.Events)-1]
	if len(c.Pending) != 0 || !slices.Equal(c.EditPending, []string{"GitHub"}) {
		t.Errorf("comment pending %v, edit pending %v, want the edit waiting for GitHub", c.Pending, c.EditPending)
	}
	if !slices.Equal(status.Pending, []string{"GitHub"}) {
		t.Errorf("status pending %v, want GitHub", status.Pending)
	}
	// what came before the issue was on GitHub went with it
	if th.Events[0].Pending != nil {
		t.Errorf("description pending %v, want none", th.Events[0].Pending)
	}
}

// A gone mark lands on the forge of its own URL, even when another sync key
// listed that URL first: never on the forge listed last.
func TestForgeGoneMarksItsOwnURL(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	author, _ := b.TestCache().GetUserIdentity(t.Context())
	bug, _, err := b.TestCache().Bugs().NewRaw(author, 1, "renamed", "", nil, map[string]string{
		"sync:github.com/a/r:id":  "I_1",
		"sync:github.com/a/r:url": "https://github.com/a/r/issues/1",
		"sync:github.com/b/r:id":  "I_2",
		"sync:github.com/b/r:url": "https://github.com/b/r/issues/2",
		// the project moved: same issue, another key, unlinked there
		"sync:github.com/c/r:url":  "https://github.com/a/r/issues/1",
		"sync:github.com/c/r:gone": "unlinked",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := bug.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	is, err := b.GetIssue(bug.Id().String())
	if err != nil {
		t.Fatal(err)
	}
	want := []gitbugio.Forge{
		{Name: "GitHub", URL: "https://github.com/a/r/issues/1", Number: "1", Gone: "unlinked"},
		{Name: "GitHub", URL: "https://github.com/b/r/issues/2", Number: "2"},
	}
	if !slices.Equal(is.Forges, want) {
		t.Errorf("forges %+v, want %+v", is.Forges, want)
	}
}

// Each forge an issue is on names its sync bridge, found as git-bug's engine
// finds it: by its sync keys, or the metadata of an older bridge. Unlinked,
// nothing waits for it and its number isn't the issue's anymore.
func TestThreadForgeBridges(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	for name, conf := range map[string]map[string]string{
		"gh": {"target": "github", "owner": "O", "project": "R", "default-login": "bot", "sync": "true"},
		"gl": {"target": "gitlab", "project-id": "5", "base-url": "https://gitlab.example.com/", "default-login": "bot", "sync": "true"},
	} {
		for k, v := range conf {
			if err := core.SetBridgeConfig(b.TestCache(), name, k, v); err != nil {
				t.Fatal(err)
			}
		}
	}
	author, _ := b.TestCache().GetUserIdentity(t.Context())
	bug, _, err := b.TestCache().Bugs().NewRaw(author, 1, "on both", "", nil, map[string]string{
		"sync:github.com/o/r:id":     "I_1",
		"sync:github.com/o/r:url":    "https://github.com/o/r/issues/7",
		"sync:github.com/o/r:number": "7",
		// pulled by git-bug's GitLab bridge before sync
		"gitlab-id":         "33",
		"gitlab-url":        "https://gitlab.example.com/g/r/-/issues/3",
		"gitlab-project-id": "5",
		"gitlab-base-url":   "https://gitlab.example.com/",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := bug.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	id := bug.Id().String()
	th, err := b.Thread(id)
	if err != nil {
		t.Fatal(err)
	}
	want := []gitbugio.Forge{
		{Name: "GitHub", URL: "https://github.com/o/r/issues/7", Number: "7", Bridge: "gh"},
		{Name: "GitLab", URL: "https://gitlab.example.com/g/r/-/issues/3", Number: "3", Bridge: "gl"},
	}
	if !slices.Equal(th.Forges, want) {
		t.Errorf("forges %+v, want %+v", th.Forges, want)
	}

	if _, err := bug.SetMetadata(t.Context(), bug.Snapshot().Operations[0].Id(), map[string]string{"sync:github.com/o/r:gone": "unlinked"}); err != nil {
		t.Fatal(err)
	}
	if err := bug.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddComment(id, "after"); err != nil {
		t.Fatal(err)
	}
	th, _ = b.Thread(id)
	if f := th.Forges[0]; f.Gone != "unlinked" || f.Bridge != "gh" {
		t.Errorf("unlinked forge %+v", f)
	}
	if last := th.Events[len(th.Events)-1]; len(last.Pending) != 0 {
		t.Errorf("a comment waits for %v, unlinked", last.Pending)
	}
	if th.HasNumber("7") || !th.HasNumber("3") {
		t.Errorf("numbers of %+v, want 3 alone", th.Forges)
	}
}

// An issue an older bridge pulled has only its <kind>-url; Unlink, or a
// push finding it deleted, marks it gone under the sync key of its project.
// The board (ListIssues) and the panel (Thread) both see it gone.
func TestLegacyForgeGone(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	for name, conf := range map[string]map[string]string{
		"gh": {"target": "github", "owner": "O", "project": "R", "default-login": "bot", "sync": "true"},
		"gl": {"target": "gitlab", "project-id": "5", "base-url": "https://gitlab.example.com/", "default-login": "bot", "sync": "true"},
	} {
		for k, v := range conf {
			if err := core.SetBridgeConfig(b.TestCache(), name, k, v); err != nil {
				t.Fatal(err)
			}
		}
	}
	author, _ := b.TestCache().GetUserIdentity(t.Context())
	legacy := func(title string, meta map[string]string, gone map[string]string) string {
		t.Helper()
		bug, _, err := b.TestCache().Bugs().NewRaw(author, 1, title, "", nil, meta)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := bug.SetMetadata(t.Context(), bug.Snapshot().Operations[0].Id(), gone); err != nil {
			t.Fatal(err)
		}
		if err := bug.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
		return bug.Id().String()
	}
	both := legacy("both", map[string]string{
		"github-id":         "I_9",
		"github-url":        "https://github.com/O/R/issues/2518",
		"gitlab-id":         "33",
		"gitlab-url":        "https://gitlab.example.com/g/r/-/issues/77",
		"gitlab-project-id": "5",
		"gitlab-base-url":   "https://gitlab.example.com/",
	}, map[string]string{"sync:github.com/o/r:gone": "unlinked", "sync:gitlab.example.com/5:gone": "1"})
	// older GitLab exports left base-url empty
	old := legacy("old", map[string]string{
		"gitlab-id":         "34",
		"gitlab-url":        "https://gitlab.example.com/g/r/-/issues/78",
		"gitlab-project-id": "5",
		"gitlab-base-url":   "",
	}, map[string]string{"sync:gitlab.example.com/5:gone": "unlinked"})

	issues, err := b.ListIssues()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]gitbugio.Issue{}
	for _, is := range issues {
		byID[is.ID] = is
	}
	for id, want := range map[string][]gitbugio.Forge{
		both: {
			{Name: "GitHub", URL: "https://github.com/O/R/issues/2518", Number: "2518", Gone: "unlinked"},
			{Name: "GitLab", URL: "https://gitlab.example.com/g/r/-/issues/77", Number: "77", Gone: "deleted"},
		},
		old: {{Name: "GitLab", URL: "https://gitlab.example.com/g/r/-/issues/78", Number: "78", Gone: "unlinked"}},
	} {
		if got := byID[id].Forges; !slices.Equal(got, want) {
			t.Errorf("board: forges %+v, want %+v", got, want)
		}
		th, err := b.Thread(id)
		if err != nil {
			t.Fatal(err)
		}
		for i := range want {
			want[i].Bridge = map[string]string{"GitHub": "gh", "GitLab": "gl"}[want[i].Name]
		}
		if !slices.Equal(th.Forges, want) {
			t.Errorf("panel: forges %+v, want %+v", th.Forges, want)
		}
	}
	if is := byID[both]; is.HasNumber("2518") || is.HasForgeNumber("77") || is.BareNumber() != "" {
		t.Errorf("an unlinked forge's number still names %+v", is.Forges)
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
	t.Parallel()
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

// @me is the forge login of the identity git-board writes as: a bridge
// tags it (github-login…), or a sync link names it.
func TestMyLogin(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	if got := b.MyLogin(); got != "" {
		t.Errorf("MyLogin = %q before any tag, want empty", got)
	}
	me, err := b.TestCache().GetUserIdentity(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := b.TestCache().Config().Update(func(f *gitconfig.File) error {
		return f.Set("git-bug.bridge.gl.login-"+me.Id().String(), "bob")
	}); err != nil {
		t.Fatal(err)
	}
	if got := b.MyLogin(); got != "bob" {
		t.Errorf("MyLogin = %q, want the sync link's bob", got)
	}
	me.SetMetadata("github-login", "alice")
	if err := me.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := b.MyLogin(); got != "alice" {
		t.Errorf("MyLogin = %q, want the bridge's alice", got)
	}
}

// An edit waiting for a forge says what the forge has now: the newest
// version that reached it, under any number of edits still waiting.
func TestThreadEditWasIsWhatTheForgesHave(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	author, _ := b.TestCache().GetUserIdentity(t.Context())
	bug, _, err := b.TestCache().Bugs().NewRaw(author, 1, "synced", "", nil, map[string]string{
		"sync:github.com/o/r:id":  "I_1",
		"sync:github.com/o/r:url": "https://github.com/o/r/issues/7",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := bug.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	id := bug.Id().String()
	pushLast := func() {
		t.Helper()
		cached, _ := b.TestCache().Bugs().ResolvePrefix(t.Context(), id)
		ops := cached.Snapshot().Operations
		if _, err := cached.SetMetadata(t.Context(), ops[len(ops)-1].Id(), map[string]string{"sync:github.com/o/r:pushed": "1"}); err != nil {
			t.Fatal(err)
		}
		if err := cached.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	comment, _ := b.AddComment(id, "v1")
	pushLast()
	_ = b.EditComment(id, comment, "v2", "")
	pushLast()
	_ = b.EditComment(id, comment, "v3", "")
	_ = b.EditComment(id, comment, "v4", "")

	th, _ := b.Thread(id)
	c := th.Events[len(th.Events)-1]
	if !slices.Equal(c.EditPending, []string{"GitHub"}) || c.EditWas != "v2" || c.Body != "v4" {
		t.Errorf("edit pending %v, was %q, body %q; want GitHub, v2, v4", c.EditPending, c.EditWas, c.Body)
	}
	// every version says who wrote it (git-bug leaves some out)
	for i, v := range c.History {
		if v.Author.Name == "" || v.Author.Name != c.Author.Name {
			t.Errorf("version %d by %q, want %q", i, v.Author.Name, c.Author.Name)
		}
	}
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
	if _, err := b.AddCommentMeta(id, "Signed off qa at 4f2e9c1", map[string]string{"signoff:status": "qa", "signoff:commit": "4f2e9c1", "sync:github.com/o/r:id": "7"}); err != nil {
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
		t.Errorf("Meta = %v, want %v (no sync: key)", got, meta)
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
