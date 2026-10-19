package release

import "testing"

func TestAppOf(t *testing.T) {
	for _, c := range []struct{ path, env, want string }{
		{"apps/co2/overlays/prod", "prod", "apps/co2"},
		{"app/overlays/dev-eu", "dev", "app/overlays/dev-eu"}, // not a suffix match
		{"overlays/dev", "dev", "overlays/dev"},               // no leading directory
	} {
		if got := appOf(c.path, c.env); got != c.want {
			t.Errorf("appOf(%q, %q) = %q, want %q", c.path, c.env, got, c.want)
		}
	}
}
