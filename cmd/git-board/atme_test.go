package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/core/people"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// newForgeless is a fresh board that never synced with a forge, in the
// working directory, with Ada Lovelace chosen by git board me --new, and
// one issue: its id.
func newForgeless(t *testing.T, dir string) string {
	t.Helper()
	t.Chdir(dir)
	if out, err := execute(t, []string{"me", "--new", "--name", "Ada Lovelace", "--email", "ada@example.com"}); err != nil {
		t.Fatalf("me --new: %v\n%s", err, out)
	}
	out, err := execute(t, []string{"new", "Fix the invoice"})
	if err != nil {
		t.Fatalf("new: %v\n%s", err, out)
	}
	return strings.TrimSpace(out)
}

// Issue #87: on a board with no forge, @me is the person you write as, by
// name: set assignee=+@me and ls involves:@me work. Serial: it chdirs.
func TestAtMeWithoutAForgeIsYourName(t *testing.T) {
	id := newForgeless(t, storetest.Repo(t, false))

	if out, err := execute(t, []string{"set", id, "assignee=+@me"}); err != nil {
		t.Fatalf("set assignee=+@me: %v\n%s", err, out)
	}
	if labels, _ := labelsOf(t, id); !slices.Contains(labels, "assignee:Ada Lovelace") {
		t.Fatalf("labels = %v, want assignee:Ada Lovelace", labels)
	}
	for _, f := range []string{"involves:@me", "assignee:@me", `"assignee:Ada Lovelace"`} {
		if out, err := execute(t, []string{"ls", f}); err != nil || !strings.Contains(out, "Fix the invoice") {
			t.Errorf("ls %s: %v\n%s", f, err, out)
		}
	}
	if out, err := execute(t, []string{"set", id, "assignee=-@me"}); err != nil {
		t.Fatalf("set assignee=-@me: %v\n%s", err, out)
	}
	if out, err := execute(t, []string{"ls", "assignee:@me"}); err != nil || strings.Contains(out, "Fix the invoice") {
		t.Errorf("ls assignee:@me after -@me: %v\n%s", err, out)
	}
}

// Issue #87, after a GitHub sync: the label naming Ada by name goes to
// GitHub as her login there (sync.md, Assignees: a label names a person
// by a login, failing that by a name with an account there), the label
// stays, and @me still finds the issue, now that @me is her login.
// Serial: it chdirs, and swaps the forges' opener.
func TestAtMeByNameSyncsAsTheLogin(t *testing.T) {
	dir := storetest.Repo(t, false)
	storetest.Git(t, dir, "remote", "add", "origin", storetest.Repo(t, true))
	id := newForgeless(t, dir)
	if out, err := execute(t, []string{"set", id, "assignee=+@me"}); err != nil {
		t.Fatalf("set assignee=+@me: %v\n%s", err, out)
	}

	// Ada has an account on GitHub, ada-gh
	b, err := store.OpenCwd()
	if err != nil {
		t.Fatal(err)
	}
	me, _, err := b.Me()
	if err == nil {
		_, err = people.Do(t.Context(), b.Datastore(), me.ID, people.AddAccount(me.ID, people.Account{Server: "github.com", ID: "42", Login: "ada-gh"}))
	}
	b.Close()
	if err != nil {
		t.Fatal(err)
	}

	storetest.ForgeOrigin(t, dir, "https://github.com/o/r.git")
	f := fake.New("github.com/o/r")
	stubForges(t, f)
	storetest.GitHub(t, map[string]string{"o/r": "o/r"})
	if out, err := execute(t, []string{"sync", "--allow-create", dir}); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	all := f.All()
	if len(all) != 1 || !slices.Equal(all[0].Assignees, []forge.Account{{ID: "42", Login: "ada-gh"}}) {
		t.Fatalf("on GitHub: %+v, want one issue assigned to ada-gh", all)
	}
	if labels, _ := labelsOf(t, id); !slices.Contains(labels, "assignee:Ada Lovelace") {
		t.Errorf("labels after the sync = %v, want assignee:Ada Lovelace kept", labels)
	}
	for _, q := range []string{"assignee:@me", "involves:@me"} {
		if out, err := execute(t, []string{"ls", q}); err != nil || !strings.Contains(out, "Fix the invoice") {
			t.Errorf("ls %s after the sync: %v\n%s", q, err, out)
		}
	}
	// the name and the login are one person: a second sync sends nothing
	before := len(f.Writes())
	if out, err := execute(t, []string{"sync", dir}); err != nil {
		t.Fatalf("second sync: %v\n%s", err, out)
	}
	if w := f.Writes()[before:]; len(w) != 0 {
		t.Errorf("a second sync wrote %+v", w)
	}
}
