package config

import (
	"slices"
	"strings"
	"testing"
)

func loadRelease(t *testing.T, src string) Release {
	t.Helper()
	cfg, _, err := parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return cfg.Release
}

func TestReleaseRoles(t *testing.T) {
	r := loadRelease(t, "[release]\nenvironments = [\"dev\", \"stage\", \"prod\"]")
	if !r.Enabled() || r.Followed() != "dev" || r.Last() != "prod" {
		t.Errorf("followed %q last %q", r.Followed(), r.Last())
	}
	if !slices.Equal(r.Candidates(), []string{"stage"}) {
		t.Errorf("candidates %v", r.Candidates())
	}
	if r.RemoteName() != "origin" {
		t.Errorf("remote %q", r.RemoteName())
	}
	p := loadRelease(t, "[release]\nenvironments = [\"prod\"]")
	if p.Followed() != "" || p.Last() != "prod" || len(p.Candidates()) != 0 {
		t.Errorf("prod only: %q %q %v", p.Followed(), p.Last(), p.Candidates())
	}
	d := loadRelease(t, "[release]\nenvironments = [\"dev\"]")
	if d.Followed() != "dev" || d.Last() != "dev" || len(d.Candidates()) != 0 {
		t.Errorf("dev only: %q %q %v", d.Followed(), d.Last(), d.Candidates())
	}
	if loadRelease(t, "").Enabled() {
		t.Error("an empty config has releases")
	}
}

func TestReleaseFollow(t *testing.T) {
	none := loadRelease(t, "[release]\nenvironments = [\"dev\", \"stage\", \"prod\"]\nfollow = \"\"")
	if none.Followed() != "" || !slices.Equal(none.Candidates(), []string{"dev", "stage"}) {
		t.Errorf("follow = \"\": %q %v", none.Followed(), none.Candidates())
	}
	st := loadRelease(t, "[release]\nenvironments = [\"dev\", \"stage\", \"prod\"]\nfollow = \"stage\"")
	if st.Followed() != "stage" || len(st.Candidates()) != 0 {
		t.Errorf("follow = stage: %q %v", st.Followed(), st.Candidates())
	}
	if r := loadRelease(t, "[release]\nremote = \"upstream\""); r.RemoteName() != "upstream" {
		t.Errorf("remote %q", r.RemoteName())
	}
}

func TestCheckReleaseRefuses(t *testing.T) {
	for _, bad := range []struct{ src, want string }{
		{`environments = ["dev", "dev"]`, "listed twice"},
		{"environments = [\"dev\"]\nfollow = \"stage\"", "isn't in environments"},
		{"environments = [\"dev\"]\n[[release.deploy]]\nrepo = \"x\"\npath = \"overlays/dev\"", "path needs {env}"},
		{`environments = ["dev"]
images = [{ name = "", context = "." }]`, "image needs a name"},
	} {
		_, _, err := parse([]byte("[release]\n" + bad.src))
		if err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%q: err %v, want %q", bad.src, err, bad.want)
		}
	}
}
