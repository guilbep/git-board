package gitbugio_test

import (
	"testing"

	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

func TestGitHubRepo(t *testing.T) {
	t.Parallel()
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
		gitbugiotest.Git(t, b.GitDir(), "remote", "add", "origin", url)
		if got := b.GitHubRepo("origin"); got != want {
			t.Errorf("GitHubRepo(%q) = %q, want %q", url, got, want)
		}
	}
	if got := gitbugiotest.New(t).GitHubRepo("origin"); got != "" {
		t.Errorf("no origin: %q", got)
	}
}

// One ancestry check per pair of refs tells both ways: mine holds theirs
// (a push sends it), theirs holds mine (a pull receives it), or neither,
// through every parent of a merge.
func TestCompare(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	commit := func(name string, parents ...gitstore.Hash) gitstore.Hash {
		t.Helper()
		h, err := gitbugio.StoreCommit(b, name, parents...)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	base := commit("base")
	m1 := commit("m1", base)
	m2 := commit("m2", m1)
	t1 := commit("t1", base)
	merged := commit("merge", m2, t1) // theirs, merged in through the second parent
	later := commit("later", t1, merged)

	for _, tc := range []struct {
		name                  string
		mine, theirs          gitstore.Hash
		wantAhead, wantBehind bool
	}{
		{"ahead", m2, base, true, false},
		{"behind", base, m2, false, true},
		{"diverged", m2, t1, false, false},
		{"ahead through a merge's second parent", merged, t1, true, false},
		{"behind through a merge's second parent", m2, later, false, true},
	} {
		ahead, behind, err := gitbugio.Compare(b, tc.mine, tc.theirs)
		if err != nil || ahead != tc.wantAhead || behind != tc.wantBehind {
			t.Errorf("%s: ahead %v, behind %v, %v; want %v, %v", tc.name, ahead, behind, err, tc.wantAhead, tc.wantBehind)
		}
	}
}
