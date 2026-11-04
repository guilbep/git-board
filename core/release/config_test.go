package release

import (
	"slices"
	"testing"
)

func TestRoles(t *testing.T) {
	t.Parallel()
	none := ""
	for _, c := range []struct {
		name                 string
		cfg                  Config
		followed, last       string
		candidates           []string
		followsLast, cutsRC  bool
		candidateTo, finalTo string
	}{
		{"three", Config{Environments: []string{"dev", "stage", "prod"}}, "dev", "prod", []string{"stage"}, false, true, "stage", "prod"},
		{"no follow", Config{Environments: []string{"dev", "stage", "prod"}, Follow: &none}, "", "prod", []string{"dev", "stage"}, false, true, "dev", "prod"},
		{"one", Config{Environments: []string{"dev"}}, "dev", "dev", nil, true, false, "", "dev"},
		{"binaries only", Config{Binaries: []string{"./cmd/x"}}, "", "", nil, true, false, "", ""},
	} {
		r := c.cfg
		if r.Followed() != c.followed || r.Last() != c.last || !slices.Equal(r.Candidates(), c.candidates) || r.FollowsLast() != c.followsLast {
			t.Errorf("%s: followed %q, last %q, candidates %q, follows last %v", c.name, r.Followed(), r.Last(), r.Candidates(), r.FollowsLast())
		}
		if r.CutsFinal(false) == c.cutsRC || !r.CutsFinal(true) {
			t.Errorf("%s: cuts a final unasked: %v", c.name, r.CutsFinal(false))
		}
		if c.cutsRC && r.DeployTo(false) != c.candidateTo {
			t.Errorf("%s: a candidate goes to %q", c.name, r.DeployTo(false))
		}
		if r.DeployTo(true) != c.finalTo {
			t.Errorf("%s: a final goes to %q", c.name, r.DeployTo(true))
		}
		if !r.Enabled() || r.RemoteName() != "origin" {
			t.Errorf("%s: enabled %v, remote %q", c.name, r.Enabled(), r.RemoteName())
		}
	}
	if got := (Config{}).BuildPlatforms(); !slices.Equal(got, DefaultPlatforms) {
		t.Errorf("BuildPlatforms = %q", got)
	}
}
