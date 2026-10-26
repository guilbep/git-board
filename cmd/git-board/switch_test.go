package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// switch reports a dry run, pulls only the forges whose sync is on, and
// --apply writes the issues; the forge gets no write. Serial: it chdirs
// and swaps the forges' opener.
func TestSwitchCommand(t *testing.T) {
	dir, _ := seedRepo(t)
	toml := "[forges.github]\nkind = \"github\"\nproject = \"o/r\"\nsync = true\n\n" +
		"[forges.gitlab]\nkind = \"gitlab\"\nproject = \"7\"\nsync = false\n"
	if err := os.WriteFile(filepath.Join(dir, configPath), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	f := fake.New("github.com/o/r")
	f.Add(forge.Issue{Title: "From GitHub", Body: "b", Author: forge.Account{ID: "7", Login: "alice"}})
	saved := openForges
	t.Cleanup(func() { openForges = saved })
	openForges = func(_ context.Context, fs []config.Forge) ([]store.NamedForge, error) {
		if len(fs) != 1 || fs[0].Name != "github" {
			t.Errorf("forges opened: %+v", fs)
		}
		return []store.NamedForge{{Name: "github", Forge: f}}, nil
	}

	out, err := execute(t, []string{"switch"})
	if err != nil || !strings.Contains(out, "A dry run") || !strings.Contains(out, "github: 1 issues") ||
		!strings.Contains(out, "gitlab: its sync is off") {
		t.Fatalf("dry run: %v\n%s", err, out)
	}
	if got := bugTitles(t, dir); len(got) != 0 {
		t.Fatalf("the dry run wrote %v", got)
	}
	out, err = execute(t, []string{"switch", "--apply"})
	if err != nil || !strings.Contains(out, "Applied") || !strings.Contains(out, "Then git-board sync") ||
		strings.Contains(out, "git push origin refs/board/seq") { // no numbers to push
		t.Fatalf("apply: %v\n%s", err, out)
	}
	if got := bugTitles(t, dir); len(got) != 1 || got[0] != "From GitHub" {
		t.Fatalf("issues %v", got)
	}
	if w := f.Writes(); len(w) != 0 {
		t.Fatalf("forge writes %+v", w)
	}
}

// With no [forges.<name>] table, switch pulls the forges of the git
// remotes, each told in a line, and says which URL it skipped. Serial: it
// chdirs and swaps the forges' opener.
func TestSwitchFindsTheForges(t *testing.T) {
	dir, _ := seedRepo(t)
	storetest.Git(t, dir, "remote", "add", "gh", "git@github.com:o/r.git") // not origin, which the switch fetches
	storetest.Git(t, dir, "remote", "add", "other", "git@bitbucket.org:o/r.git")
	t.Chdir(dir)
	f := fake.New("github.com/o/r")
	saved := openForges
	t.Cleanup(func() { openForges = saved })
	openForges = func(_ context.Context, fs []config.Forge) ([]store.NamedForge, error) {
		if len(fs) != 1 || fs[0].Name != "github" || fs[0].Project != "o/r" || !fs[0].Discovered {
			t.Errorf("forges opened: %+v", fs)
		}
		return []store.NamedForge{{Name: "github", Forge: f}}, nil
	}
	out, err := execute(t, []string{"switch"})
	if err != nil || !strings.Contains(out, "forge github, found in the remotes: github.com o/r\n") ||
		!strings.Contains(out, "skipped git@bitbucket.org:o/r.git") {
		t.Fatalf("%v\n%s", err, out)
	}
}
