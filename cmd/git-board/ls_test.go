package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/store"
)

func titlesOf(t *testing.T, out string) []string {
	t.Helper()
	var issues []store.Issue
	if err := json.Unmarshal([]byte(out), &issues); err != nil {
		t.Fatalf("ls --json: %v\n%s", err, out)
	}
	var res []string
	for _, is := range issues {
		res = append(res, is.Title)
	}
	return res
}

func TestLsSortsByBuiltInKeysAndLimits(t *testing.T) {
	newTestRepo(t)
	if _, err := execute(t, []string{"new", "Add CSV export"}); err != nil {
		t.Fatal(err)
	}
	if out, err := execute(t, []string{"ls", "--sort", "title", "--json"}); err != nil || strings.Join(titlesOf(t, out), "|") != "Add CSV export|Fix invoice bug" {
		t.Errorf("ls --sort title: %v\n%s", err, out)
	}
	if out, err := execute(t, []string{"ls", "--sort", "title desc", "--limit", "1", "--json"}); err != nil || strings.Join(titlesOf(t, out), "|") != "Fix invoice bug" {
		t.Errorf("ls --sort 'title desc' -L 1: %v\n%s", err, out)
	}
	if out, err := execute(t, []string{"ls", "--sort", "created desc", "-L", "1", "--json"}); err != nil || len(titlesOf(t, out)) != 1 { // created in the same second, maybe: no order to check
		t.Errorf("ls --sort 'created desc' -L 1: %v\n%s", err, out)
	}
	for _, sort := range []string{"nonsense", "title sideways"} {
		if _, err := execute(t, []string{"ls", "--sort", sort}); exitCode(err) != exitUsage {
			t.Errorf("ls --sort %q = %v, want exit %d", sort, err, exitUsage)
		}
	}
	if out, err := execute(t, []string{"ls", "--sort", "status desc"}); err != nil {
		t.Errorf("ls --sort 'status desc' (a field): %v\n%s", err, out)
	}
}

// The separator row of ls --md had a | too many.
func TestLsMarkdownTableIsWellFormed(t *testing.T) {
	newTestRepo(t)
	out, err := execute(t, []string{"ls", "--md"})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(out, "\n")
	if strings.Count(lines[0], "|") != strings.Count(lines[1], "|") || strings.Contains(lines[1], "||") {
		t.Errorf("ls --md header:\n%s\n%s", lines[0], lines[1])
	}
}
