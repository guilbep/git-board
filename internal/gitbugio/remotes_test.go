package gitbugio_test

import (
	"testing"

	"github.com/git-bug/git-bug/repository/repotest"

	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

func TestGitHubRepo(t *testing.T) {
	for url, want := range map[string]string{
		"https://github.com/o/r":          "o/r",
		"https://github.com/o/r.git":      "o/r",
		"git@github.com:o/r.git":          "o/r",
		"ssh://git@github.com/o/r.git":    "o/r",
		"ssh://git@github.com/o/r":        "o/r",
		"https://gitlab.com/o/r.git":      "",
		"/srv/git/r.git":                  "",
		"https://github.com.evil.com/o/r": "",
	} {
		b := gitbugiotest.New(t)
		repotest.AddRemote(t, b.TestCache(), "origin", url)
		if got := b.GitHubRepo("origin"); got != want {
			t.Errorf("GitHubRepo(%q) = %q, want %q", url, got, want)
		}
	}
	if got := gitbugiotest.New(t).GitHubRepo("origin"); got != "" {
		t.Errorf("no origin: %q", got)
	}
}
