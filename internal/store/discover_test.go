package store_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// found are the forges a repository with those remotes finds, each remote
// {name, url, push urls…}, under the settings src, and the URLs skipped.
func found(t *testing.T, src string, remotes ...[]string) ([]config.Forge, []string) {
	t.Helper()
	dir := storetest.Repo(t, false)
	for _, r := range remotes {
		storetest.Git(t, dir, "remote", "add", r[0], r[1])
		for _, push := range r[2:] {
			storetest.Git(t, dir, "remote", "set-url", "--add", "--push", r[0], push)
		}
	}
	cfg, _, err := config.Check([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	var skipped []string
	forges, err := storetest.Open(t, dir).Forges(cfg, func(s string) { skipped = append(skipped, s) })
	if err != nil {
		t.Fatal(err)
	}
	return forges, skipped
}

func gh(name, project string) config.Forge {
	return config.Forge{Name: name, Kind: "github", Host: "github.com", Project: project, Sync: true, Discovered: true}
}

func gl(name, host, path string) config.Forge {
	return config.Forge{Name: name, Kind: "gitlab", Host: host, Project: path, Path: path, Sync: true, Discovered: true}
}

// The wiki's real remotes, and the URL forms: each project once, by kind,
// host and project, whichever remote or way names it.
func TestForgesFoundInTheRemotes(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name    string
		remotes [][]string
		want    []config.Forge
		skipped string
	}{
		{"the wiki", [][]string{{"origin", "git@github.com:EPFL-ENAC/wiki.git"}, {"epfl", "git@gitlab-ssh.epfl.ch:EPFL-ENAC/wiki.git"}},
			[]config.Forge{gl("gitlab", "gitlab.epfl.ch", "EPFL-ENAC/wiki"), gh("github", "EPFL-ENAC/wiki")}, ""},
		{"fetch on GitHub, push on GitLab", [][]string{
			{"origin", "git@github.com:EPFL-ENAC/wiki.git", "git@github.com:EPFL-ENAC/wiki.git", "git@gitlab-ssh.epfl.ch:EPFL-ENAC/wiki.git"},
			{"upstream", "https://github.com/epfl-enac/wiki"}},
			[]config.Forge{gh("github", "EPFL-ENAC/wiki"), gl("gitlab", "gitlab.epfl.ch", "EPFL-ENAC/wiki")}, ""},
		{"ssh with a port", [][]string{{"origin", "ssh://git@gitlab-ssh.epfl.ch:2222/group/sub/proj.git"}},
			[]config.Forge{gl("gitlab", "gitlab.epfl.ch", "group/sub/proj")}, ""},
		{"https with and without .git", [][]string{{"a", "https://github.com/o/r.git"}, {"b", "https://github.com/o/r2"}, {"c", "ssh://ssh.github.com:443/o/r3.git"}},
			[]config.Forge{gh("github", "o/r"), gh("github-2", "o/r2"), gh("github-3", "o/r3")}, ""},
		{"gitlab.com", [][]string{{"origin", "https://oauth2:secret@gitlab.com/g/p.git"}},
			[]config.Forge{gl("gitlab", "gitlab.com", "g/p")}, ""},
		{"an unknown host", [][]string{{"origin", "git@bitbucket.org:o/r.git"}, {"local", "/srv/git/r.git"}, {"gh", "git@github.com:o/r.git"}},
			[]config.Forge{gh("github", "o/r")}, "skipped git@bitbucket.org:o/r.git: not github.com, nor a GitLab host"},
	} {
		forges, skipped := found(t, "", c.remotes...)
		if !reflect.DeepEqual(forges, c.want) {
			t.Errorf("%s: forges\n%+v, want\n%+v", c.name, forges, c.want)
		}
		if strings.Join(skipped, "\n") != c.skipped {
			t.Errorf("%s: skipped %q", c.name, skipped)
		}
	}

	wiki := []string{"origin", "git@github.com:EPFL-ENAC/wiki.git"}
	table := "[forges.gl]\nkind = \"gitlab\"\nhost = \"gitlab.epfl.ch\"\nproject = \"42400\"\n"
	if forges, _ := found(t, table, wiki); len(forges) != 1 || forges[0].Name != "gl" {
		t.Errorf("a table: forges %+v", forges)
	}
	if forges, _ := found(t, "[forges]\ndiscover = false\n", wiki); len(forges) != 0 {
		t.Errorf("discover = false: forges %+v", forges)
	}
}

// A GitLab forge found in the remotes reads its id once, and then has the
// Key() of the table naming that id; one whose path GitLab doesn't know is
// left out, and the others go on. Serial: it sets GitLab's token.
func TestResolveForges(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/api/v4/projects/EPFL-ENAC%2Fwiki" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"id": 42400, "path_with_namespace": "EPFL-ENAC/wiki", "visibility": "public"}`))
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	t.Setenv("GITLAB_HOST", host)
	t.Setenv("GITLAB_TOKEN", "t")

	ctx := context.Background()
	var left []string
	got := store.ResolveForges(ctx, []config.Forge{gl("gitlab", host, "EPFL-ENAC/wiki"), gl("gitlab-2", host, "nope/x"), gh("github", "o/r")},
		func(f config.Forge, why error) { left = append(left, f.Name) })
	if len(got) != 2 || got[0].Project != "42400" || got[0].Path != "EPFL-ENAC/wiki" || got[1].Name != "github" || len(left) != 1 || left[0] != "gitlab-2" {
		t.Fatalf("resolved %+v, left out %v", got, left)
	}
	opened, err := store.OpenForges(ctx, got[:1])
	if err != nil {
		t.Fatal(err)
	}
	table, err := store.OpenForges(ctx, []config.Forge{{Name: "gl", Kind: "gitlab", Host: host, Project: "42400"}})
	if err != nil {
		t.Fatal(err)
	}
	if opened[0].Key() != table[0].Key() {
		t.Fatalf("key %q, the table's %q", opened[0].Key(), table[0].Key())
	}
}
