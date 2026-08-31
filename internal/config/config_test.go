package config

import (
	"testing"
	"time"
)

const fixture = "testdata/git-board.toml"

func mustLoad(t *testing.T) *Config {
	t.Helper()
	cfg, err := Load(fixture)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

func TestParsesReposAndFields(t *testing.T) {
	cfg := mustLoad(t)

	want := []string{"EPFL-ENAC/co2-calculator", "EPFL-ENAC/co2-calculator-back-office-doc"}
	if len(cfg.Repos) != len(want) || cfg.Repos[0] != want[0] || cfg.Repos[1] != want[1] {
		t.Fatalf("Repos = %v, want %v", cfg.Repos, want)
	}

	status := cfg.Fields["status"]
	if !status.Single {
		t.Error("status.Single = false, want true")
	}
	if status.Options[0] != "backlog" || status.Options[len(status.Options)-1] != "validated" {
		t.Errorf("status.Options endpoints = %q..%q", status.Options[0], status.Options[len(status.Options)-1])
	}
	if len(status.Options) != 9 {
		t.Errorf("len(status.Options) = %d, want 9", len(status.Options))
	}

	if !cfg.Fields["est"].Numeric {
		t.Error("est.Numeric = false, want true")
	}
	if cfg.Fields["assignee"].Single {
		t.Error("assignee.Single = true, want false")
	}
}

func TestParsesSprints(t *testing.T) {
	cfg := mustLoad(t)

	if cfg.Sprints.LengthDays != 14 {
		t.Errorf("LengthDays = %d, want 14", cfg.Sprints.LengthDays)
	}
	if cfg.Sprints.StartWeekday != "monday" {
		t.Errorf("StartWeekday = %q, want monday", cfg.Sprints.StartWeekday)
	}
	if len(cfg.Sprints.List) != 2 || cfg.Sprints.List[0].Slug != "delivery" || cfg.Sprints.List[1].Slug != "post-delivery" {
		t.Fatalf("sprint slugs = %+v", cfg.Sprints.List)
	}
	wantStart := time.Date(2026, 8, 24, 0, 0, 0, 0, cfg.Sprints.List[0].Start.Location())
	if !cfg.Sprints.List[0].Start.Equal(wantStart) {
		t.Errorf("delivery.Start = %v, want %v", cfg.Sprints.List[0].Start, wantStart)
	}
	if cfg.Sprints.List[1].Title != "Post-delivery" {
		t.Errorf("post-delivery.Title = %q", cfg.Sprints.List[1].Title)
	}
}

func TestParsesViews(t *testing.T) {
	cfg := mustLoad(t)

	if cfg.Views["current-sprint"].Layout != "board" {
		t.Errorf("current-sprint.Layout = %q", cfg.Views["current-sprint"].Layout)
	}
	if cfg.Views["current-sprint"].Group != "status" {
		t.Errorf("current-sprint.Group = %q", cfg.Views["current-sprint"].Group)
	}
	if cfg.Views["backlog"].Sort != "prio asc" {
		t.Errorf("backlog.Sort = %q", cfg.Views["backlog"].Sort)
	}
	if cfg.Views["mine"].Filter != "assignee:@me is:open" {
		t.Errorf("mine.Filter = %q", cfg.Views["mine"].Filter)
	}
}

func TestResolveSprintCurrentNextPrevious(t *testing.T) {
	cfg := mustLoad(t)
	today := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC) // inside "delivery" sprint window

	current, err := ResolveSprint(cfg.Sprints, "current", today)
	if err != nil || current == nil || current.Slug != "delivery" {
		t.Fatalf("current = %+v, err = %v", current, err)
	}
	next, err := ResolveSprint(cfg.Sprints, "next", today)
	if err != nil || next == nil || next.Slug != "post-delivery" {
		t.Fatalf("next = %+v, err = %v", next, err)
	}
	previous, err := ResolveSprint(cfg.Sprints, "previous", today)
	if err != nil || previous != nil {
		t.Fatalf("previous = %+v, err = %v, want nil", previous, err)
	}
}

func TestResolveSprintBeforeFirstStart(t *testing.T) {
	cfg := mustLoad(t)
	today := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if current, _ := ResolveSprint(cfg.Sprints, "current", today); current != nil {
		t.Errorf("current = %+v, want nil", current)
	}
	next, err := ResolveSprint(cfg.Sprints, "next", today)
	if err != nil || next == nil || next.Slug != "delivery" {
		t.Fatalf("next = %+v, err = %v", next, err)
	}
	if previous, _ := ResolveSprint(cfg.Sprints, "previous", today); previous != nil {
		t.Errorf("previous = %+v, want nil", previous)
	}
}
