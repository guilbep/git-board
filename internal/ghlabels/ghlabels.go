// Package ghlabels creates GitHub-side labels matching the ns:value labels
// git-board writes into git-bug (status:*, prio:*, sprint:*, …), so they
// exist and are visibly colored/filterable in the GitHub Issues UI before
// the first bridge run. Read/write against GitHub via the `gh` CLI only —
// never touches the local git-bug repo (that's internal/gitbugio's job).
package ghlabels

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

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

// Current is the GitHub repository of the working directory's checkout,
// as gh sees it: its remotes, or gh repo set-default. Read-only.
func Current() (string, error) {
	cmd := exec.Command("gh", "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gh repo view: %w: %s", err, stderr.String())
	}
	return strings.TrimSpace(string(out)), nil
}

// Template is an issue template and the labels it applies.
type Template struct {
	File   string
	Labels []string
}

// Templates reads the labels each issue template in dir applies
// (.github/ISSUE_TEMPLATE): an issue form's labels, or a Markdown
// template's front matter, as a list or as "a, b". GitHub applies only
// the labels the repository has and drops the others without a word.
// config.yml is the template chooser, not a template. No dir: none.
func Templates(dir string) ([]Template, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var res []Template
	for _, e := range entries {
		name, ext := e.Name(), filepath.Ext(e.Name())
		if e.IsDir() || !slices.Contains([]string{".md", ".yml", ".yaml"}, ext) || strings.TrimSuffix(name, ext) == "config" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		if ext == ".md" {
			data = frontMatter(data)
		}
		var t struct {
			Labels any `yaml:"labels"`
		}
		if err := yaml.Unmarshal(data, &t); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		var labels []string
		switch v := t.Labels.(type) {
		case string:
			for _, l := range strings.Split(v, ",") {
				if l = strings.TrimSpace(l); l != "" {
					labels = append(labels, l)
				}
			}
		case []any:
			for _, l := range v {
				if l, ok := l.(string); ok && l != "" {
					labels = append(labels, l)
				}
			}
		}
		if len(labels) > 0 {
			res = append(res, Template{File: name, Labels: labels})
		}
	}
	return res, nil
}

// frontMatter is a Markdown file's leading YAML, between "---" lines
// (trailing spaces allowed: co2's specs.md opens with "--- ").
func frontMatter(md []byte) []byte {
	first, rest, _ := bytes.Cut(bytes.ReplaceAll(md, []byte("\r\n"), []byte("\n")), []byte("\n"))
	if string(bytes.TrimSpace(first)) != "---" {
		return nil
	}
	fm, _, _ := bytes.Cut(rest, []byte("\n---"))
	return fm
}
