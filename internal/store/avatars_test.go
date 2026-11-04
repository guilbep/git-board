package store_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/model"
	"github.com/guilbep/git-board/core/people"
	"github.com/guilbep/git-board/internal/avatar"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// board.avatars is a person's, as board.view: every level git reads, set
// here in this repository's own config. Serial: it sets the global
// config's path.
func TestAvatarsSetting(t *testing.T) {
	global := filepath.Join(t.TempDir(), "gitconfig")
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	b := storetest.New(t)
	check := func(what string, want avatar.Mode, wantScope, wantIgnored string) {
		t.Helper()
		m, scope, ignored, err := b.AvatarsSetting()
		if err != nil || m != want || scope != wantScope || ignored != wantIgnored {
			t.Errorf("%s: %q (%q, ignored %q), %v", what, m, scope, ignored, err)
		}
	}
	check("unset", avatar.Forge, "", "")
	if err := b.SetAvatars(avatar.Gravatar); err != nil {
		t.Fatal(err)
	}
	check("set here", avatar.Gravatar, "local", "")
	for range 2 { // unsetting what isn't set is no error
		if err := b.SetAvatars(""); err != nil {
			t.Fatal(err)
		}
	}
	check("unset again", avatar.Forge, "", "")
	storetest.Git(t, b.GitDir(), "config", "--file", global, "board.avatars", "faces")
	check("not a mode", avatar.Forge, "global", "faces")
}

// withGitHub gives a person a github.com account with GitHub's numeric id,
// as the sync records them (the test store's accounts use the login).
func withGitHub(t *testing.T, b *store.Backend, p model.Person, id string) {
	t.Helper()
	if _, err := people.Do(t.Context(), b.Store(), p.ID, people.AddAccount(p.ID, people.Account{Server: "github.com", ID: id, Login: p.Login})); err != nil {
		t.Fatal(err)
	}
}

func TestPersonsCarryTheirPicture(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	ann := storetest.NewPerson(t, b, "Ann", "")
	ann.Login = "ann"
	withGitHub(t, b, ann, "17847282")
	id := storetest.NewIssue(t, b, "Export CSV")
	storetest.CommentAs(t, b, id, ann, 1_700_000_000, "hello")
	want := "https://avatars.githubusercontent.com/u/17847282?s=88"

	th, err := b.Thread(id)
	if err != nil {
		t.Fatal(err)
	}
	var got string
	for _, e := range th.Events {
		if e.Author.ID == ann.ID {
			got = e.Author.AvatarURL
		}
	}
	if got != want {
		t.Errorf("the comment's author: %q, want %q", got, want)
	}
	all, err := b.Identities()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range all {
		if p.ID == ann.ID && p.AvatarURL != want {
			t.Errorf("Identities: %q", p.AvatarURL)
		}
	}

	if err := b.SetAvatars(avatar.None); err != nil {
		t.Fatal(err)
	}
	th, err = b.Thread(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range th.Events {
		if e.Author.AvatarURL != "" {
			t.Errorf("none: %s has %q", e.Author.Name, e.Author.AvatarURL)
		}
	}
	all, _ = b.Identities()
	for _, p := range all {
		if p.AvatarURL != "" || strings.Contains(p.AvatarURL, "@") {
			t.Errorf("none: identity %s has %q", p.Name, p.AvatarURL)
		}
	}
}
