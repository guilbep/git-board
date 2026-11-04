package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestLogTellsEveryChange(t *testing.T) {
	newTestRepo(t)
	writeStatusOptionsConfig(t)
	id := openIssueID(t)
	for _, args := range [][]string{
		{"comment", id, "-b", "first"},
		{"comment", id, "--edit-last", "-b", "first, edited"},
		{"edit", id, "--title", "Fix the invoice bug", "-b", "Steps."},
		{"set", id, "status=in-progress"},
		{"close", id},
	} {
		if out, err := execute(t, args); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	out, err := execute(t, []string{"log", id})
	if err != nil {
		t.Fatalf("log: %v\n%s", err, out)
	}
	// in the order they were made, though made in the same second
	last := -1
	for _, want := range []string{
		`created "Fix invoice bug"`, "+status:ready", "commented [", "edited comment [", "first, edited",
		`retitled "Fix invoice bug" → "Fix the invoice bug"`, "edited the description: Steps.",
		"+status:in-progress -status:ready", "closed",
	} {
		i := strings.Index(out, want)
		if i < 0 {
			t.Errorf("log lacks %q:\n%s", want, out)
		} else if i < last {
			t.Errorf("log has %q too early:\n%s", want, out)
		}
		last = max(last, i)
	}

	out, err = execute(t, []string{"log", id, "--field", "status", "--json"})
	if err != nil {
		t.Fatalf("log --field --json: %v", err)
	}
	var entries []logEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("log --json: %v\n%s", err, out)
	}
	var path []string
	for _, e := range entries {
		if e.Kind != "labels" || e.Author.Name != "guilbep" {
			t.Errorf("entry %+v: want labels by guilbep", e)
		}
		path = append(path, e.Added...)
	}
	if !slices.Equal(path, []string{"status:ready", "status:in-progress", "status:validated"}) {
		t.Errorf("status path = %v", path)
	}

	if out, err := execute(t, []string{"log", id, "--since", "2999-01-01", "--json"}); err != nil || strings.TrimSpace(out) != "[]" {
		t.Errorf("log --since the future = %q, %v; want []", out, err)
	}
	if _, err := execute(t, []string{"log", id, "--since", "yesterday"}); exitCode(err) != exitUsage {
		t.Errorf("log --since yesterday = %v, want exit %d", err, exitUsage)
	}
}

func TestParseSince(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.Local)
	for s, want := range map[string]time.Time{
		"2026-10-01": time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local),
		"7d":         now.AddDate(0, 0, -7),
		"36h":        now.Add(-36 * time.Hour),
	} {
		if got, err := parseSince(s, now); err != nil || !got.Equal(want) {
			t.Errorf("parseSince(%q) = %v, %v; want %v", s, got, err, want)
		}
	}
}
