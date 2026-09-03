// Package config parses .git-board.toml and resolves sprint aliases.
// Pure logic only — no I/O beyond reading the file, no printing.
package config

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Field describes one namespaced-label field (e.g. "status", "prio").
type Field struct {
	Single  bool     `toml:"single" json:"single"`
	Options []string `toml:"options" json:"options"`
	Numeric bool     `toml:"numeric" json:"numeric"`
}

// KnownValue reports whether value is one of Options — or true if Options is
// empty (a free-form field, nothing configured to validate against). Shared
// by internal/ghbridge (warns on a bridge pull) and internal/lint (the same
// check, run over the local repo instead of an incoming GitHub value).
func (f Field) KnownValue(value string) bool {
	return len(f.Options) == 0 || slices.Contains(f.Options, value)
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
	Layout string `toml:"layout" json:"layout"`
	Filter string `toml:"filter" json:"filter"`
	Group  string `toml:"group" json:"group"`
	Sort   string `toml:"sort" json:"sort"`
}

// reposTable mirrors the [repos] table shape (`list = [...]`).
type reposTable struct {
	List []string `toml:"list"`
}

// GithubProject is the [github-project] table: which GitHub Projects (v2)
// board `bridge pull` reads from by default, so --owner/--project don't
// need retyping on every run. Set via `git-board bridge configure`.
type GithubProject struct {
	Owner  string `toml:"owner" json:"owner"`
	Number int    `toml:"number" json:"number"`
}

// raw mirrors the on-disk TOML shape for decoding.
type raw struct {
	Repos         reposTable       `toml:"repos"`
	Fields        map[string]Field `toml:"fields"`
	Sprints       SprintsConfig    `toml:"sprints"`
	Views         map[string]View  `toml:"views"`
	GithubProject GithubProject    `toml:"github-project"`
}

// Config is the parsed .git-board.toml.
type Config struct {
	Repos         []string
	Fields        map[string]Field
	Sprints       SprintsConfig
	Views         map[string]View
	GithubProject GithubProject
}

// Load reads and parses a .git-board.toml file at path.
func Load(path string) (*Config, error) {
	var r raw
	if _, err := toml.DecodeFile(path, &r); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &Config{
		Repos:         r.Repos.List,
		Fields:        r.Fields,
		Sprints:       r.Sprints,
		Views:         r.Views,
		GithubProject: r.GithubProject,
	}, nil
}

// SetGithubProject upserts the [github-project] table in the
// .git-board.toml at path, so `bridge pull` doesn't need --owner/--project
// on every run. It edits the file textually (find-the-table-or-append)
// rather than decoding and re-encoding the whole document, because
// BurntSushi/toml's encoder drops comments and formatting on a full
// round-trip — every other table in the file is left byte-for-byte alone.
// Creates the file if it doesn't exist yet.
func SetGithubProject(path, owner string, number int) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	block := fmt.Sprintf("[github-project]\nowner = %q\nnumber = %d\n", owner, number)
	updated := upsertTable(string(data), "[github-project]", block)
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// upsertTable replaces the named top-level table (header through the line
// before the next "[" header, or EOF) if present, else appends it — a
// blank line ahead of it if the file is non-empty.
func upsertTable(content, header, block string) string {
	lines := strings.Split(content, "\n")
	start := -1
	end := len(lines)
	for i, line := range lines {
		if start < 0 {
			if strings.TrimSpace(line) == header {
				start = i
			}
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			end = i
			break
		}
	}

	if start < 0 {
		if strings.TrimSpace(content) == "" {
			return block
		}
		return strings.TrimRight(content, "\n") + "\n\n" + block
	}

	before := strings.Join(lines[:start], "\n")
	after := strings.Join(lines[end:], "\n")
	var b strings.Builder
	if before != "" {
		b.WriteString(before)
		b.WriteString("\n")
	}
	b.WriteString(block)
	if strings.TrimSpace(after) != "" {
		b.WriteString("\n")
		b.WriteString(after)
	}
	return b.String()
}

// dateOnly strips time-of-day/location so sprint boundaries compare by
// calendar day regardless of the TOML decoder's local timezone.
func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// sortedSprints returns a copy of list ordered by start date — shared by
// ResolveSprint (current/next/previous neighbour lookup) and SprintWindows
// (end-date computation), so the ordering rule lives in one place.
func sortedSprints(list []Sprint) []Sprint {
	ordered := make([]Sprint, len(list))
	copy(ordered, list)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Start.Before(ordered[j].Start) })
	return ordered
}

// ResolveSprint resolves "current" / "next" / "previous" against the sprint
// calendar. The "current" sprint is the last one whose start date is on or
// before today; next/previous are its neighbours in start-date order.
func ResolveSprint(sprints SprintsConfig, which string, today time.Time) (*Sprint, error) {
	today = dateOnly(today)
	ordered := sortedSprints(sprints.List)

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

// SprintWindow is a configured sprint plus its computed end date.
type SprintWindow struct {
	Sprint
	End time.Time
}

// SprintWindows returns every configured sprint, ordered by start date (the
// same ordering ResolveSprint uses — see sortedSprints), with End computed
// as the next sprint's start date, or Start+LengthDays for the last one.
// This window math isn't in ResolveSprint itself (which only needs
// start-date ordering to resolve @current/@next/@previous), so it's new
// here rather than duplicated from anywhere.
func SprintWindows(sprints SprintsConfig) []SprintWindow {
	ordered := sortedSprints(sprints.List)
	windows := make([]SprintWindow, len(ordered))
	for i, s := range ordered {
		end := dateOnly(s.Start).AddDate(0, 0, sprints.LengthDays)
		if i+1 < len(ordered) {
			end = dateOnly(ordered[i+1].Start)
		}
		windows[i] = SprintWindow{Sprint: s, End: end}
	}
	return windows
}

// AppendSprint appends a new [[sprints.list]] entry to the .git-board.toml
// at path, via the same targeted text-surgery SetGithubProject uses (find
// the right spot, splice text) rather than decode+re-encode, which would
// drop comments/formatting elsewhere in the file. Errors if slug is already
// used by a sprint in the current file.
func AppendSprint(path, slug, title string, start time.Time) error {
	cfg, err := Load(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	for _, s := range cfg.Sprints.List {
		if s.Slug == slug {
			return fmt.Errorf("sprint %q already exists in %s", slug, path)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	block := fmt.Sprintf("[[sprints.list]]\nslug = %q\ntitle = %q\nstart = %s\n",
		slug, title, start.Format("2006-01-02"))
	updated := appendAfterLastMatch(string(data), "[[sprints.list]]", block)
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// appendAfterLastMatch inserts block right after the last line-exact-match
// of header (through the line before the next "[" header, or EOF) — so a
// new [[sprints.list]] entry lands grouped with the existing ones instead
// of at the end of unrelated tables. Appends at EOF (blank-line separated)
// if header doesn't occur in content at all.
func appendAfterLastMatch(content, header, block string) string {
	lines := strings.Split(content, "\n")
	end := -1
	for i, line := range lines {
		if strings.TrimSpace(line) != header {
			continue
		}
		end = len(lines)
		for j := i + 1; j < len(lines); j++ {
			if strings.HasPrefix(strings.TrimSpace(lines[j]), "[") {
				end = j
				break
			}
		}
	}

	if end < 0 {
		if strings.TrimSpace(content) == "" {
			return block
		}
		return strings.TrimRight(content, "\n") + "\n\n" + block
	}

	before := strings.TrimRight(strings.Join(lines[:end], "\n"), "\n")
	after := strings.Join(lines[end:], "\n")
	var b strings.Builder
	b.WriteString(before)
	b.WriteString("\n\n")
	b.WriteString(block)
	if strings.TrimSpace(after) != "" {
		b.WriteString("\n")
		b.WriteString(after)
	}
	return b.String()
}
