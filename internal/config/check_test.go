package config

import (
	"errors"
	"strings"
	"testing"
)

func TestCheckFindsWhereTheFileIsWrong(t *testing.T) {
	cfg, warnings, err := Check([]byte("[views.mine]\nfilter = \"assignee:@me\"\nlayout = \"board\"\n\n[fields.status]\noptions = [\"todo\", \"done\"\n"))
	var se *SyntaxError
	if !errors.As(err, &se) || se.Line != 6 || se.Message == "" {
		t.Fatalf("unclosed array: %v (%#v), want a syntax error on line 6", err, se)
	}
	if cfg != nil || warnings != nil {
		t.Errorf("a broken file still gave %v %v", cfg, warnings)
	}
}

func TestCheckWarnsAboutWhatItWontUse(t *testing.T) {
	src := `[views.mine]
filtr = "assignee:@me"
layout = "kanban"

[[sprints.list]]
slug = "s1"
start = 2026-10-05

[[sprints.list]]
slug = "s1"
start = 2026-10-19
`
	cfg, warnings, err := Check([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(warnings, "\n")
	for _, want := range []string{"views.mine.filtr", `"kanban"`, `"s1"`} {
		if !strings.Contains(all, want) {
			t.Errorf("warnings %q miss %s", warnings, want)
		}
	}
	if cfg.Views["mine"].Layout != "kanban" || len(cfg.Sprints.List) != 2 {
		t.Errorf("config %+v", cfg)
	}
}

func TestCheckTakesAGoodFileQuietly(t *testing.T) {
	_, warnings, err := Check([]byte("[views.backlog]\nfilter = \"status:backlog\"\nlayout = \"table\"\ngroup = \"prio\"\n"))
	if err != nil || len(warnings) != 0 {
		t.Errorf("%v %v", err, warnings)
	}
}

func TestCheckTakesBranches(t *testing.T) {
	src := `[branches]
base = "dev"
name = "{kind}/{number}-{slug}"
kinds = { bug = "fix", feat = "feat" }
`
	cfg, warnings, err := Check([]byte(src))
	if err != nil || len(warnings) != 0 {
		t.Fatalf("%v %v", err, warnings)
	}
	b := cfg.Branches
	if b.Base != "dev" || b.Name != "{kind}/{number}-{slug}" || b.Kinds["bug"] != "fix" || b.Kinds["feat"] != "feat" {
		t.Errorf("branches %+v", b)
	}
}

func TestCheckTakesRelease(t *testing.T) {
	src := `[release]
remote = "origin"
environments = ["dev", "stage", "prod"]
follow = "dev"
checks = "make ci"
images = [
  { name = "frontend", context = "./frontend" },
  { name = "backend",  context = "./backend" },
]
chart = { path = "./helm", registry = "oci://ghcr.io/epfl-enac/co2-calculator/helm" }
version_values = ["backend.env.APP_VERSION"]
builder = ""

[[release.deploy]]
repo = "git@github.com:EPFL-ENAC/enack8s-app-config.git"
path = "epfl/co2-calculator/overlays/{env}"
registry = "ghcr.io/epfl-enac/epfl/co2-calculator"

[[release.deploy]]
repo = "git@github.com:EPFL-ENAC/openshift-app-config.git"
path = "epfl/co2-calculator/overlays/{env}"
registry = "quay-its.epfl.ch/svc1751/co2-calculator"
`
	cfg, warnings, err := Check([]byte(src))
	if err != nil || len(warnings) != 0 {
		t.Fatalf("%v %v", err, warnings)
	}
	r := cfg.Release
	if len(r.Images) != 2 || r.Chart == nil || r.Chart.Path != "./helm" || len(r.Deploy) != 2 || r.Deploy[1].Registry != "quay-its.epfl.ch/svc1751/co2-calculator" || r.VersionValues[0] != "backend.env.APP_VERSION" {
		t.Errorf("release %+v", r)
	}
}
