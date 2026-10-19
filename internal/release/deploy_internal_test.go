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

func TestTargetName(t *testing.T) {
	for url, want := range map[string]string{
		"git@gitlab.test:org/enack8s-app-config.git": "enack8s",
		"https://host/org/co2-app-config":            "co2",
		"/tmp/x/config.git":                          "config",
	} {
		if got := targetName(url); got != want {
			t.Errorf("targetName(%q) = %q, want %q", url, got, want)
		}
	}
}
