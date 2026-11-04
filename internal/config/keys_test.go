package config

import (
	"errors"
	"strings"
	"testing"
)

const keysFile = `# the team's board
[github-project]
owner = "epfl-enac" # the org
number = 3

[views.backlog]
filter = "is:open"   # everything open
group = "status"

[[sprints.list]]
slug = "s1"
title = "Sprint 1"
start = 2026-11-02T00:00:00Z
`

func TestGetAndListKeys(t *testing.T) {
	t.Parallel()
	for key, want := range map[string]string{
		"github-project.owner":  `"epfl-enac"`,
		"github-project.number": "3",
		"views.backlog.filter":  `"is:open"`,
	} {
		if got, err := GetKey(keysFile, key); err != nil || got != want {
			t.Errorf("get %s = %q, %v; want %q", key, got, err, want)
		}
	}
	if _, err := GetKey(keysFile, "github-project.write"); !errors.Is(err, ErrNoKey) {
		t.Errorf("get of a key not set = %v", err)
	}
	list, err := ListKeys(keysFile)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, kv := range list {
		keys = append(keys, kv.Key)
	}
	if got := strings.Join(keys, " "); got != "github-project.number github-project.owner sprints.list views.backlog.filter views.backlog.group" {
		t.Errorf("keys = %s", got)
	}
}

// Set changes the key's line alone, comments kept; adds a missing key or
// table; removes with "". An unknown key, a wrong type, or a key in an
// array of tables is refused.
func TestSetKeyText(t *testing.T) {
	t.Parallel()
	out, err := SetKeyText(keysFile, "views.backlog.filter", Value("is:open prio:p1"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `filter = "is:open prio:p1"   # everything open`) || !strings.Contains(out, "# the team's board") || !strings.Contains(out, `owner = "epfl-enac" # the org`) {
		t.Errorf("set:\n%s", out)
	}
	if out, err = SetKeyText(out, "github-project.write", Value("true")); err != nil || !strings.Contains(out, "write = true") {
		t.Errorf("a new key: %v\n%s", err, out)
	}
	if out, err = SetKeyText(out, "sprints.length_days", Value("14")); err != nil || !strings.Contains(out, "[sprints]\nlength_days = 14") {
		t.Errorf("a new table: %v\n%s", err, out)
	}
	if out, err = SetKeyText(out, "views.backlog.group", ""); err != nil || strings.Contains(out, `group = "status"`) {
		t.Errorf("a removed key: %v\n%s", err, out)
	}
	for _, c := range []struct{ key, value string }{
		{"github-project.ownr", Value("x")},
		{"nonsense.key", Value("x")},
		{"views.backlog.filtre", Value("x")},
		{"owner", Value("x")},
	} {
		if _, err := SetKeyText(keysFile, c.key, c.value); !errors.Is(err, ErrUnknownKey) {
			t.Errorf("set %s = %v, want unknown key", c.key, err)
		}
	}
	if _, err := SetKeyText(keysFile, "github-project.number", Value("three")); err == nil {
		t.Errorf("a string for a number was taken")
	}
	if _, err := SetKeyText(keysFile, "sprints.list.slug", Value("x")); err == nil {
		t.Errorf("a key in an array of tables was set")
	}
}

func TestValue(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"true": "true", "14": "14", `["a", "b"]`: `["a", "b"]`, "ready": `"ready"`, `"x"`: `"x"`, "is:open": `"is:open"`} {
		if got := Value(in); got != want {
			t.Errorf("Value(%q) = %q, want %q", in, got, want)
		}
	}
}

// A forge's keys are its table's, read through a second decode.
func TestSetForgeKey(t *testing.T) {
	t.Parallel()
	file := "[forges.github]\nkind = \"github\"\nproject = \"guilbep/git-board\"\n"
	out, err := SetKeyText(file, "forges.github.sync", Value("true"))
	if err != nil || !strings.Contains(out, "sync = true") {
		t.Errorf("set: %v\n%s", err, out)
	}
	if _, err := SetKeyText(file, "forges.github.synk", Value("true")); !errors.Is(err, ErrUnknownKey) {
		t.Errorf("an unknown forge key = %v", err)
	}
}
