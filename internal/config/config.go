// Package config parses .git-board.toml and resolves sprint aliases.
// Pure logic only — no I/O beyond reading the file, no printing.
package config

import (
	"fmt"
	"sort"
	"time"

	"github.com/BurntSushi/toml"
)

// Field describes one namespaced-label field (e.g. "status", "prio").
type Field struct {
	Single  bool     `toml:"single"`
	Options []string `toml:"options"`
	Numeric bool     `toml:"numeric"`
}

// Sprint is one entry in [[sprints.list]].
type Sprint struct {
	Slug  string    `toml:"slug"`
	Title string    `toml:"title"`
	Start time.Time `toml:"start"`
}

// SprintsConfig is the [sprints] table.
type SprintsConfig struct {
	LengthDays   int      `toml:"length_days"`
	StartWeekday string   `toml:"start_weekday"`
	List         []Sprint `toml:"list"`
}

// View is one [views.<name>] table.
type View struct {
	Layout string `toml:"layout"`
	Filter string `toml:"filter"`
	Group  string `toml:"group"`
	Sort   string `toml:"sort"`
}

// reposTable mirrors the [repos] table shape (`list = [...]`).
type reposTable struct {
	List []string `toml:"list"`
}

// raw mirrors the on-disk TOML shape for decoding.
type raw struct {
	Repos   reposTable       `toml:"repos"`
	Fields  map[string]Field `toml:"fields"`
	Sprints SprintsConfig    `toml:"sprints"`
	Views   map[string]View  `toml:"views"`
}

// Config is the parsed .git-board.toml.
type Config struct {
	Repos   []string
	Fields  map[string]Field
	Sprints SprintsConfig
	Views   map[string]View
}

// Load reads and parses a .git-board.toml file at path.
func Load(path string) (*Config, error) {
	var r raw
	if _, err := toml.DecodeFile(path, &r); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &Config{
		Repos:   r.Repos.List,
		Fields:  r.Fields,
		Sprints: r.Sprints,
		Views:   r.Views,
	}, nil
}

// dateOnly strips time-of-day/location so sprint boundaries compare by
// calendar day regardless of the TOML decoder's local timezone.
func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// ResolveSprint resolves "current" / "next" / "previous" against the sprint
// calendar. The "current" sprint is the last one whose start date is on or
// before today; next/previous are its neighbours in start-date order.
func ResolveSprint(sprints SprintsConfig, which string, today time.Time) (*Sprint, error) {
	today = dateOnly(today)

	ordered := make([]Sprint, len(sprints.List))
	copy(ordered, sprints.List)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Start.Before(ordered[j].Start) })

	idx := -1
	for i, s := range ordered {
		if !dateOnly(s.Start).After(today) {
			idx = i
		} else {
			break
		}
	}

	switch which {
	case "current":
		if idx < 0 {
			return nil, nil
		}
		return &ordered[idx], nil
	case "next":
		nxt := idx + 1
		if nxt >= len(ordered) {
			return nil, nil
		}
		return &ordered[nxt], nil
	case "previous":
		if idx <= 0 {
			return nil, nil
		}
		return &ordered[idx-1], nil
	default:
		return nil, fmt.Errorf("unknown sprint alias: %q", which)
	}
}
