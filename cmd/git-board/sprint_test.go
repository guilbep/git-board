package main

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/gitbugio"
)

// sprintFixtureConfig configures two sprints with dates chosen so
// current/next/previous resolve deterministically against real wall-clock
// time for a very long time (both "early" and "late" straddle any
// realistic test run): "early" is always in the past, "late" always in the
// future, so current -> early, next -> late, previous -> nil forever until
// well past this century.
const sprintFixtureConfig = `
[sprints]
length_days = 14
start_weekday = "monday"

[[sprints.list]]
slug = "early"
title = "Early Sprint"
start = 2020-01-01

[[sprints.list]]
slug = "late"
title = "Late Sprint"
start = 2099-01-01
`

// allFutureSprintConfig has no sprint that has started yet — current must
// resolve to "no sprint currently active", not an error or a crash.
const allFutureSprintConfig = `
[sprints]
length_days = 14
start_weekday = "monday"

[[sprints.list]]
slug = "future"
title = "Future Sprint"
start = 2099-01-01
`

func writeConfig(t *testing.T, content string) {
	t.Helper()
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestSprintLsListsAllConfiguredSprints(t *testing.T) {
	newTestRepo(t)
	writeConfig(t, sprintFixtureConfig)

	out, err := execute(t, []string{"sprint", "ls"})
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "early") || !strings.Contains(out, "2020-01-01") {
		t.Errorf("output missing early sprint: %q", out)
	}
	if !strings.Contains(out, "late") || !strings.Contains(out, "2099-01-01") {
		t.Errorf("output missing late sprint: %q", out)
	}
	// early's computed end is late's start (no third sprint to bound it).
	if !strings.Contains(out, "2099-01-01") {
		t.Errorf("output missing early's computed end (late's start): %q", out)
	}
}

func TestSprintLsWithNoSprintsConfigured(t *testing.T) {
	newTestRepo(t)
	// newTestRepo's own minimal config has no [sprints] table at all.

	out, err := execute(t, []string{"sprint", "ls"})
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "no sprints configured") {
		t.Errorf("output = %q, want a clear \"no sprints configured\" message", out)
	}
}

func TestSprintCurrentNextPreviousResolve(t *testing.T) {
	newTestRepo(t)
	writeConfig(t, sprintFixtureConfig)

	out, err := execute(t, []string{"sprint", "current"})
	if err != nil {
		t.Fatalf("current: %v\noutput:\n%s", err, out)
	}
	if !strings.HasPrefix(out, "early:") {
		t.Errorf("current = %q, want it to resolve to \"early\"", out)
	}

	out, err = execute(t, []string{"sprint", "next"})
	if err != nil {
		t.Fatalf("next: %v\noutput:\n%s", err, out)
	}
	if !strings.HasPrefix(out, "late:") {
		t.Errorf("next = %q, want it to resolve to \"late\"", out)
	}

	out, err = execute(t, []string{"sprint", "previous"})
	if err != nil {
		t.Fatalf("previous: %v\noutput:\n%s", err, out)
	}
	if strings.TrimSpace(out) != "no previous sprint" {
		t.Errorf("previous = %q, want \"no previous sprint\" (early has no predecessor)", out)
	}
}

func TestSprintCurrentNoSprintActive(t *testing.T) {
	newTestRepo(t)
	writeConfig(t, allFutureSprintConfig)

	out, err := execute(t, []string{"sprint", "current"})
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}
	if strings.TrimSpace(out) != "no sprint currently active" {
		t.Errorf("current = %q, want the clear no-sprint-active message, not an error or crash", out)
	}
}

func TestSprintNewAppendsAndRejectsDuplicateAndBadStart(t *testing.T) {
	newTestRepo(t)
	writeConfig(t, sprintFixtureConfig)

	out, err := execute(t, []string{"sprint", "new", "q1", "--title", "Q1", "--start", "2026-10-05"})
	if err != nil {
		t.Fatalf("new: %v\noutput:\n%s", err, out)
	}

	lsOut, err := execute(t, []string{"sprint", "ls"})
	if err != nil {
		t.Fatalf("ls after new: %v", err)
	}
	if !strings.Contains(lsOut, "q1") || !strings.Contains(lsOut, "2026-10-05") {
		t.Errorf("ls after new = %q, want the new sprint listed", lsOut)
	}

	if _, err := execute(t, []string{"sprint", "new", "early", "--title", "Dup", "--start", "2021-01-01"}); err == nil {
		t.Error("expected an error for a duplicate slug, got nil")
	}

	if _, err := execute(t, []string{"sprint", "new", "bad", "--title", "Bad", "--start", "not-a-date"}); err == nil {
		t.Error("expected an error for an unparseable --start, got nil")
	}

	if _, err := execute(t, []string{"sprint", "new", "missing-flags"}); err == nil {
		t.Error("expected an error for missing --title/--start, got nil")
	}
}

func TestSprintReportAggregatesByStatus(t *testing.T) {
	newTestRepo(t)
	writeConfig(t, sprintFixtureConfig)

	b, err := gitbugio.OpenCwd()
	if err != nil {
		t.Fatalf("OpenCwd: %v", err)
	}
	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	// newTestRepo seeds one open bug (status:ready) and one closed bug
	// (no status). Tag both into "early", plus create a second
	// status:ready bug so a bucket actually has more than one issue in
	// it — otherwise every row and the total would print "1" or "2"
	// alike, and a report that returned len(issues) instead of
	// len(bucket.Issues) per row would pass by accident.
	for _, issue := range issues {
		if _, err := b.ApplyLabelChange(issue.ID, []string{"sprint:early"}, nil); err != nil {
			t.Fatalf("tag %s into sprint: %v", issue.ID, err)
		}
	}
	c := b.TestCache()
	extra, _, err := c.Bugs().New(t.Context(), "Second ready bug", "")
	if err != nil {
		t.Fatalf("new bug: %v", err)
	}
	if _, _, err := extra.ChangeLabels(t.Context(), []string{"status:ready", "sprint:early"}, nil); err != nil {
		t.Fatalf("label new bug: %v", err)
	}
	if err := extra.Commit(t.Context()); err != nil {
		t.Fatalf("commit new bug: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("close backend: %v", err)
	}

	out, err := execute(t, []string{"sprint", "report", "early"})
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}

	// The closed bug has no status label at all — BucketBy groups it as
	// "(none)"; the two open bugs both carry status:ready. Assert the
	// actual per-row counts (not just substring presence), so a report
	// that mixed up bucket.Issues with the full issue list would fail.
	counts := parseBucketCounts(t, out)
	if counts["ready"] != 2 {
		t.Errorf("counts[ready] = %d, want 2", counts["ready"])
	}
	if counts["(none)"] != 1 {
		t.Errorf("counts[(none)] = %d, want 1", counts["(none)"])
	}
	if !strings.Contains(out, "total: 3") {
		t.Errorf("report missing \"total: 3\":\n%s", out)
	}
}

// parseBucketCounts reads runSprintReport's "status\tcount" table rows
// (before the trailing "total: N" line) into a status -> count map.
func parseBucketCounts(t *testing.T, out string) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[0] == "status" || strings.HasPrefix(line, "total:") {
			continue
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		counts[fields[0]] = n
	}
	return counts
}

func TestSprintReportRejectsUnknownSlug(t *testing.T) {
	newTestRepo(t)
	writeConfig(t, sprintFixtureConfig)

	if _, err := execute(t, []string{"sprint", "report", "no-such-slug"}); err == nil {
		t.Error("expected an error for an unconfigured sprint slug, got nil")
	}
}

func TestSprintReportEmptySprintHasZeroTotal(t *testing.T) {
	newTestRepo(t)
	writeConfig(t, sprintFixtureConfig)

	out, err := execute(t, []string{"sprint", "report", "late"})
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "total: 0") {
		t.Errorf("report = %q, want \"total: 0\" for a sprint with no matching issues", out)
	}
}
