// Package ghlabels creates GitHub-side labels matching the ns:value labels
// git-board writes into git-bug (status:*, prio:*, sprint:*, …), so they
// exist and are visibly colored/filterable in the GitHub Issues UI before
// the first bridge run. Read/write against GitHub via the `gh` CLI only —
// never touches the local git-bug repo (that's internal/gitbugio's job).
package ghlabels

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"

	"github.com/guilbep/git-board/internal/config"
)

// Desired returns every ns:value label git-board's config implies: one per
// [fields.<name>] option (fields with no configured Options are free-form —
// assignee, est — and have no fixed value set to create labels for), plus
// one "sprint:<slug>" per [[sprints.list]] entry. Sorted for stable output.
func Desired(cfg *config.Config) []string {
	var labels []string
	for name, f := range cfg.Fields {
		for _, opt := range f.Options {
			labels = append(labels, name+":"+opt)
		}
	}
	for _, s := range cfg.Sprints.List {
		labels = append(labels, "sprint:"+s.Slug)
	}
	sort.Strings(labels)
	return labels
}

// existingLabel is the one field of `gh label list --json name` this
// package reads.
type existingLabel struct {
	Name string `json:"name"`
}

// Existing shells out to `gh label list`, read-only.
func Existing(repo string) ([]string, error) {
	cmd := exec.Command("gh", "label", "list", "--repo", repo, "--json", "name", "--limit", "1000")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh label list --repo %s: %w: %s", repo, err, stderr.String())
	}
	var rows []existingLabel
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("parsing gh label list JSON: %w", err)
	}
	names := make([]string, len(rows))
	for i, r := range rows {
		names[i] = r.Name
	}
	return names, nil
}

// Missing returns the Desired labels repo doesn't already have, preserving
// Desired's sorted order.
func Missing(cfg *config.Config, repo string) ([]string, error) {
	existing, err := Existing(repo)
	if err != nil {
		return nil, err
	}
	return MissingFrom(Desired(cfg), existing), nil
}

// MissingFrom is Missing's pure diff — desired entries absent from
// existing — split out so it's both reusable (cmd/git-board's lint
// command diffs against a swappable Existing seam) and testable without
// shelling out to `gh`.
func MissingFrom(desired, existing []string) []string {
	have := make(map[string]bool, len(existing))
	for _, n := range existing {
		have[n] = true
	}
	var missing []string
	for _, want := range desired {
		if !have[want] {
			missing = append(missing, want)
		}
	}
	return missing
}

// Create shells out to `gh label create`, writing one label to repo.
func Create(repo, name string) error {
	cmd := exec.Command("gh", "label", "create", name, "--repo", repo)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh label create %q --repo %s: %w: %s", name, repo, err, stderr.String())
	}
	return nil
}
