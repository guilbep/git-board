package forge

import (
	"reflect"
	"testing"
)

// A remote's URL names its forge in every form git takes, and a URL that
// names none says why.
func TestFromRemote(t *testing.T) {
	t.Parallel()
	gh := Table{Kind: "github", Host: "github.com", Project: "guilbep/git-board", Sync: true, Discovered: true}
	gl := Table{Kind: "gitlab", Host: "gitlab.epfl.ch", Project: "enac/co2", Path: "enac/co2", Sync: true, Discovered: true}
	for _, c := range []struct {
		url  string
		want Table
		skip string
	}{
		{"git@github.com:guilbep/git-board.git", gh, ""},
		{"https://github.com/guilbep/git-board", gh, ""},
		{"ssh://git@github.com/guilbep/git-board.git/", gh, ""},
		{"git@gitlab-ssh.epfl.ch:enac/co2.git", gl, ""},
		{"https://gitlab.epfl.ch/enac/co2.git", gl, ""},
		{"/home/me/repo", Table{}, ""},
		{"http://github.com/o/r", Table{}, "not git@host:path, ssh:// or https://"},
		{"https://example.com/o/r", Table{}, "not github.com, nor a GitLab host"},
		{"https://github.com/onlyowner", Table{}, "not owner/repo on github.com"},
	} {
		if f, skip := FromRemote(c.url); !reflect.DeepEqual(f, c.want) || skip != c.skip {
			t.Errorf("%s: %+v, %q; want %+v, %q", c.url, f, skip, c.want, c.skip)
		}
	}
	fs, skipped := Discover([]string{"git@github.com:o/r.git", "https://github.com/o/r", "git@github.com:o/s.git", "https://example.com/x/y"})
	if len(fs) != 2 || fs[0].Name != "github" || fs[1].Name != "github-2" || len(skipped) != 1 {
		t.Errorf("discovered %+v, skipped %q", fs, skipped)
	}
}
