package config

import (
	"slices"
	"strings"
	"testing"
)

// A project names its people by role; an issue takes each role it doesn't
// set itself.
func TestRoles(t *testing.T) {
	cfg, _, err := parse([]byte("[roles]\nlead-dev = \"bob\"\npm = \"claire\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.RolesFor([]string{"status:ready", "lead-dev:dan"}); !slices.Equal(got, []string{"pm:claire"}) {
		t.Errorf("RolesFor = %v, want pm:claire alone", got)
	}
	if got := cfg.RolesFor(nil); !slices.Equal(got, []string{"lead-dev:bob", "pm:claire"}) {
		t.Errorf("RolesFor(nil) = %v", got)
	}
	if cfg.TypeOf("pm", nil) != TypePerson {
		t.Errorf("pm is a %s", cfg.TypeOf("pm", nil))
	}
	var none *Config
	if got := none.RolesFor(nil); got != nil {
		t.Errorf("no config: %v", got)
	}
	if got := (&Config{}).RolesFor(nil); got != nil {
		t.Errorf("no [roles]: %v", got)
	}

	for text, want := range map[string]string{
		"[roles]\nPM = \"x\"\n":                                "roles.PM",
		"[roles]\npm = \"\"\n":                                 "roles.pm",
		"[roles]\npm = \"a, b\"\n":                             "roles.pm",
		"[fields.pm]\ntype = \"select\"\n[roles]\npm = \"x\"\n": "a person",
	} {
		if _, _, err := parse([]byte(text)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want an error naming %q", text, err, want)
		}
	}
}
