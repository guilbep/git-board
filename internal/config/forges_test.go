package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestForges(t *testing.T) {
	t.Parallel()
	src := `[forges]
publish-private = true

[forges.gh]
kind = "github"
project = "guilbep/git-board"
login = "guilbep"
sync = true

[forges.epfl]
kind = "gitlab"
host = "gitlab.epfl.ch"
project = "42400"
`
	cfg, warnings, err := Check([]byte(src))
	if err != nil || len(warnings) > 0 {
		t.Fatalf("Check: %v %v", warnings, err)
	}
	want := []Forge{
		{Name: "epfl", Kind: "gitlab", Project: "42400", Host: "gitlab.epfl.ch"},
		{Name: "gh", Kind: "github", Project: "guilbep/git-board", Host: "github.com", Login: "guilbep", Sync: true},
	}
	if !cfg.PublishPrivate || cfg.Discover || !reflect.DeepEqual(cfg.Forges, want) {
		t.Fatalf("forges %+v, publish-private %v", cfg.Forges, cfg.PublishPrivate)
	}
	if w := cfg.Forges[0].Where(); w != "https://gitlab.epfl.ch/projects/42400" {
		t.Errorf("where %q", w)
	}
	if u := (Forge{Host: "127.0.0.1:8080"}).BaseURL(); u != "http://127.0.0.1:8080" {
		t.Errorf("loopback base URL %q", u)
	}
	for src, discover := range map[string]bool{"": true, "[forges]\npublish-private = true\n": true, "[forges]\ndiscover = false\n": false} {
		if cfg, warnings, err := Check([]byte(src)); err != nil || len(warnings) > 0 || cfg.Discover != discover {
			t.Errorf("%q: discover %v, %v %v", src, cfg.Discover, warnings, err)
		}
	}
	for _, bad := range []string{
		"[forges.x]\nkind = \"gitea\"\nproject = \"a/b\"",
		"[forges.x]\nkind = \"gitlab\"\nproject = \"group/project\"",
		"[forges.x]\nkind = \"github\"\nproject = \"nope\"",
		"[forges.x]\nkind = \"github\"\nproject = \"a/b\"\nhost = \"evil.com/x\"",
		"[forges.x]\nkind = \"github\"\nproject = \"a/b\"\ntoken = \"t\"",
	} {
		if cfg, warnings, err := Check([]byte(bad)); err == nil && len(warnings) == 0 {
			t.Errorf("%q: forges %+v, no error nor warning", bad, cfg.Forges)
		}
	}
}

func TestWithAndWithoutForge(t *testing.T) {
	t.Parallel()
	src := "# mine\n[fields.status]\noptions = [\"todo\"]\n"
	gh := Forge{Name: "gh", Kind: "github", Project: "o/r", Host: "github.com", Login: "me", Sync: true}
	gl := Forge{Name: "gl", Kind: "gitlab", Project: "7", Host: "gitlab.epfl.ch"}
	text := WithForge(WithForge(src, gh), gl)
	cfg, _, err := parse([]byte(text))
	if err != nil || len(cfg.Forges) != 2 || cfg.Forges[0] != gh || cfg.Forges[1] != gl {
		t.Fatalf("%s\nforges %+v, %v", text, cfg.Forges, err)
	}
	gh.Sync = false
	text = WithoutForge(WithForge(text, gh), "gl")
	cfg, _, err = parse([]byte(text))
	if err != nil || len(cfg.Forges) != 1 || cfg.Forges[0] != gh || !strings.HasPrefix(text, src) || strings.Contains(text, "\n\n\n") {
		t.Fatalf("%q\nforges %+v, %v", text, cfg.Forges, err)
	}
}
