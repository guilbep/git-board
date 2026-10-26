// Package ghlabels keeps the GitHub-side labels matching the ns:value labels
// git-board writes into git-bug (status:*, prio:*, sprint:*, …), so they
// exist and are visibly colored/filterable in the GitHub Issues UI before
// the first bridge run, with the colors and descriptions of
// .git-board.toml. Read/write against GitHub via the `gh` CLI only —
// never touches the local git-bug repo (that's internal/store's job).
package ghlabels

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/guilbep/git-board/internal/config"
)

// Gh runs `gh` with args and returns its output: the one door to GitHub,
// a var so tests answer for it.
var Gh = func(args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh %s: %w: %s", strings.Join(args[:min(len(args), 3)], " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// Label is a GitHub label, or one the project defines (Desired).
type Label struct {
	Name        string `json:"name"`
	Color       string `json:"color"` // hex without #, as GitHub has it
	Description string `json:"description"`
	// Desired's alone: the field and value it is, and whether the file
	// sets its color, its description: Plan changes only those.
	Field, Value             string `json:"-"`
	OwnColor, OwnDescription bool   `json:"-"`
}

// Desired returns every label git-board's config implies: one per
// [fields.<name>] option (fields with no configured Options are free-form —
// assignee — and have no fixed value set to create labels for), plus
// one "sprint:<slug>" per [[sprints.list]] entry; the kinds as the kind
// field stores them (cfg.Label). Fields by name, values in their order. A
// label's color is the one the board shows, its description the file's.
func Desired(cfg *config.Config) []Label {
	values := map[string][]string{}
	for name, f := range cfg.Fields {
		if len(f.Options) > 0 {
			values[name] = f.Options
		}
	}
	for _, s := range cfg.Sprints.List {
		values["sprint"] = append(values["sprint"], s.Slug)
	}
	var labels []Label
	for _, name := range slices.Sorted(maps.Keys(values)) {
		f, colors := cfg.Fields[name], cfg.Colors(name, values[name])
		for _, v := range values[name] {
			own, desc := find(f.Colors, v), find(f.Descriptions, v)
			labels = append(labels, Label{Name: cfg.Label(name, v), Color: config.Hex(colors[v]), Description: desc,
				Field: name, Value: v, OwnColor: own != "", OwnDescription: desc != ""})
		}
	}
	return labels
}

// find is m's value for key, without case as GitHub's names have it.
func find(m map[string]string, key string) string {
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

// Existing lists repo's labels, read-only.
func Existing(repo string) ([]Label, error) {
	out, err := Gh("label", "list", "--repo", repo, "--json", "name,color,description", "--limit", "1000")
	if err != nil {
		return nil, err
	}
	var labels []Label
	if err := json.Unmarshal(out, &labels); err != nil {
		return nil, fmt.Errorf("parsing gh label list JSON: %w", err)
	}
	return labels, nil
}

// lookup is name's label in labels, without case (GitHub's names have none).
func lookup(labels []Label, name string) (Label, bool) {
	i := slices.IndexFunc(labels, func(l Label) bool { return strings.EqualFold(l.Name, name) })
	if i < 0 {
		return Label{}, false
	}
	return labels[i], true
}

func sameColor(a, b string) bool { return strings.EqualFold(a, b) }

func sameDescription(a, b string) bool { return strings.TrimSpace(a) == strings.TrimSpace(b) }

// State is how GitHub's copy of want compares: missing, color (differs),
// description (differs) or same.
func State(want Label, existing []Label) string {
	got, ok := lookup(existing, want.Name)
	switch {
	case !ok:
		return "missing"
	case want.Color != "" && !sameColor(want.Color, got.Color):
		return "color"
	case !sameDescription(want.Description, got.Description):
		return "description"
	}
	return "same"
}

// Change is a label Apply creates, or updates (Update) from Was.
type Change struct {
	Repo        string `json:"repo"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
	Update      bool   `json:"update"`
	Was         *Label `json:"was,omitempty"`
}

// Plan is what repo needs to have desired: every missing label, created
// with its color and description, and an existing one updated only where
// the file sets its color or description and GitHub's differs. A label the
// file says nothing about stays as GitHub has it: an organization's labels
// keep their colors.
func Plan(repo string, desired, existing []Label) []Change {
	var res []Change
	for _, want := range desired {
		got, ok := lookup(existing, want.Name)
		if !ok {
			res = append(res, Change{Repo: repo, Name: want.Name, Color: want.Color, Description: want.Description})
			continue
		}
		c := Change{Repo: repo, Name: got.Name, Color: got.Color, Description: got.Description, Update: true, Was: &got}
		if want.OwnColor && !sameColor(want.Color, got.Color) {
			c.Color = want.Color
		}
		if want.OwnDescription && !sameDescription(want.Description, got.Description) {
			c.Description = want.Description
		}
		if c.Color != got.Color || c.Description != got.Description {
			res = append(res, c)
		}
	}
	return res
}

// Apply carries out c: `gh label create`, with --force for an update.
func Apply(c Change) error {
	args := []string{"label", "create", c.Name, "--repo", c.Repo}
	if c.Color != "" {
		args = append(args, "--color", c.Color)
	}
	if c.Description != "" {
		args = append(args, "--description", c.Description)
	}
	if c.Update {
		args = append(args, "--force")
	}
	_, err := Gh(args...)
	return err
}

// Stray is repo's labels outside the project's: not desired, and not a
// value of a free-form field (assignee:alice). A bug beside kind:bug,
// GitHub's old defaults.
func Stray(cfg *config.Config, desired, existing []Label) []Label {
	var res []Label
	for _, l := range existing {
		ns, _, field := strings.Cut(l.Name, ":")
		if f, ok := cfg.Fields[ns]; field && ok && len(f.Options) == 0 {
			continue
		}
		if _, ok := lookup(desired, l.Name); !ok {
			res = append(res, l)
		}
	}
	return res
}

// countQuery asks GitHub how many issues and pull requests carry a label.
const countQuery = `query($owner: String!, $name: String!, $label: String!) {
  repository(owner: $owner, name: $name) {
    label(name: $label) { issues { totalCount } pullRequests { totalCount } }
  }
}`

// Count is how many of repo's issues and pull requests carry name.
func Count(repo, name string) (int, error) {
	owner, repoName, _ := strings.Cut(repo, "/")
	out, err := Gh("api", "graphql", "-f", "query="+countQuery, "-f", "owner="+owner, "-f", "name="+repoName, "-f", "label="+name,
		"--jq", ".data.repository.label | .issues.totalCount + .pullRequests.totalCount")
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
}

// Delete removes name from repo, and so from its issues.
func Delete(repo, name string) error {
	_, err := Gh("label", "delete", name, "--repo", repo, "--yes")
	return err
}

// Current is the GitHub repository of the working directory's checkout,
// as gh sees it: its remotes, or gh repo set-default. Read-only.
func Current() (string, error) {
	out, err := Gh("repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner")
	if err != nil {
		return "", err
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
