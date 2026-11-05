package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/github"
	"github.com/guilbep/git-board/forge/gitlab"
)

// git board lint --create-labels, as v1's (internal/ghlabels): the forge
// labels the project file implies, made on each forge with their colors
// and descriptions so that they show on the forge before the first sync.
// Through the forge adapters (forge.Labeler), with their tokens (ADR
// 0010).

// DesiredLabel is a label the project file implies (DesiredLabels), the
// field and value it is, and whether the file sets its color, its
// description: PlanLabels changes only those.
type DesiredLabel struct {
	forge.Label
	Field, Value             string
	OwnColor, OwnDescription bool
}

// DesiredLabels returns every label the schema implies: one per
// [fields.<name>] option (a field with no options is free-form, as the
// assignee, with no fixed values to make), plus one sprint:<slug> per
// [[sprints.list]] entry; the kinds as the kind field stores them (a flat
// kind plain); then [labels] list's, plain, with no field. Fields by name,
// values in their order. A label's color is the one the board shows, its
// description the file's.
func DesiredLabels(s *issues.Schema) []DesiredLabel {
	if s == nil {
		return nil
	}
	values := map[string][]string{}
	for name, f := range s.Fields {
		if len(f.Options) > 0 {
			values[name] = f.Options
		}
	}
	for _, sp := range s.Sprints.List {
		values["sprint"] = append(values["sprint"], sp.Slug)
	}
	var labels []DesiredLabel
	for _, name := range slices.Sorted(maps.Keys(values)) {
		f, colors := s.Fields[name], s.Colors(name, values[name])
		for _, v := range values[name] {
			own, desc := find(f.Colors, v), find(f.Descriptions, v)
			labels = append(labels, DesiredLabel{Label: forge.Label{Name: s.Label(name, v), Color: issues.Hex(colors[v]), Description: desc},
				Field: name, Value: v, OwnColor: own != "", OwnDescription: desc != ""})
		}
	}
	colors := s.LabelColors()
	for _, name := range s.Labels.List {
		own, desc := find(s.Labels.Colors, name), find(s.Labels.Descriptions, name)
		labels = append(labels, DesiredLabel{Label: forge.Label{Name: name, Color: issues.Hex(colors[name]), Description: desc},
			Value: name, OwnColor: own != "", OwnDescription: desc != ""})
	}
	return labels
}

// find is m's value for key, without case, as forges' label names.
func find(m map[string]string, key string) string {
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

// lookupLabel is name's label in labels, without case.
func lookupLabel(labels []forge.Label, name string) (forge.Label, bool) {
	i := slices.IndexFunc(labels, func(l forge.Label) bool { return strings.EqualFold(l.Name, name) })
	if i < 0 {
		return forge.Label{}, false
	}
	return labels[i], true
}

func sameColor(a, b string) bool { return strings.EqualFold(a, b) }

func sameDescription(a, b string) bool { return strings.TrimSpace(a) == strings.TrimSpace(b) }

// LabelChange is a label to make on a forge, or to update (Update) from
// Was.
type LabelChange struct {
	forge.Label
	Update bool
	Was    *forge.Label
}

// PlanLabels is what the forge of key needs to have desired: every
// missing label, made with its color and description, and an existing
// one updated only where the file sets its color or description and the
// forge's differs. A label the file says nothing about stays as the forge
// has it: an organization's labels keep their colors. A label held off
// the forge (Config.Held: its field lives elsewhere there) is never made.
func PlanLabels(key string, desired []DesiredLabel, existing []forge.Label, held func(key, label string) bool) []LabelChange {
	var res []LabelChange
	for _, want := range desired {
		got, ok := lookupLabel(existing, want.Name)
		if !ok && held(key, want.Name) {
			continue
		}
		if !ok {
			res = append(res, LabelChange{Label: want.Label})
			continue
		}
		c := LabelChange{Label: got, Update: true, Was: &got}
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

// LabelForge is a forge project lint --create-labels plans for.
type LabelForge struct {
	Repo  string // as lint names it: owner/repo, a GitLab project's path or id
	Key   string // host/project, as Config.Held takes it
	Table forge.Table
}

// ErrNoLabelForge: no forge project to make the labels on.
var ErrNoLabelForge = errors.New("no GitHub repository: set [repos] in .git-board.toml, or an origin on GitHub")

// LabelForges are the forge projects lint --create-labels plans for:
// [repos] list's GitHub repositories and the [forges] tables, each once;
// with neither, the forges origin's URLs name (FoundForges, nothing
// asked of a forge), as v1 fell back to origin's. say hears the URLs left
// out. None: ErrNoLabelForge.
func (b *Board) LabelForges(ctx context.Context, say func(string)) ([]LabelForge, error) {
	var tables []forge.Table
	gh := b.Config.githubForge()
	for _, r := range b.Config.Repos.List {
		tables = append(tables, forge.Table{Name: "gh", Kind: "github", Host: "github.com", Project: r, Login: gh.Login})
	}
	if !b.Config.Forges.Discover {
		tables = append(tables, b.Config.Forges.List...)
	}
	if len(tables) == 0 {
		found, err := b.FoundForges(ctx, say)
		if err != nil {
			return nil, err
		}
		tables = found
	}
	var out []LabelForge
	seen := map[string]bool{}
	for _, t := range tables {
		key := t.Host + "/" + t.Project
		if seen[strings.ToLower(key)] {
			continue
		}
		seen[strings.ToLower(key)] = true
		out = append(out, LabelForge{Repo: t.Project, Key: key, Table: t})
	}
	if len(out) == 0 {
		return nil, ErrNoLabelForge
	}
	return out, nil
}

// OpenLabeler opens a forge project's labels through its adapter, with
// the table's posting account's token: the environment's, else git's
// credential helper's (ADR 0010). Tests replace it with a fake.
var OpenLabeler = func(ctx context.Context, f forge.Table) (forge.Labeler, error) {
	switch f.Kind {
	case "github":
		api := ""
		if f.Host == "github.com" {
			api = GitHubAPI
		}
		return github.New(ctx, github.Options{Project: f.Project, Host: f.Host, Login: f.Login, API: api})
	case "gitlab":
		return gitlab.New(ctx, baseURL(f), f.Project, f.Login)
	}
	return nil, fmt.Errorf("kind %q: github or gitlab", f.Kind)
}

// OriginGitHub is the checkout's GitHub repository: the one origin's URL
// names, whose issue templates are this checkout's.
func (b *Board) OriginGitHub(ctx context.Context) (forge.Table, error) {
	url, err := gitRun(ctx, b.Dir, "remote", "get-url", NumbersRemote)
	if err != nil {
		return forge.Table{}, errors.New("no origin remote")
	}
	f, _ := forge.FromRemote(NoCredentials(strings.TrimSpace(url)))
	if f.Kind != "github" {
		return forge.Table{}, fmt.Errorf("origin is no GitHub repository: %s", NoCredentials(strings.TrimSpace(url)))
	}
	f.Login = b.Config.githubForge().Login
	return f, nil
}

// IssueTemplate is an issue template and the labels it applies.
type IssueTemplate struct {
	File   string
	Labels []string
}

// IssueTemplatesDir is the repository's GitHub issue templates' folder.
func (b *Board) IssueTemplatesDir() string {
	return filepath.Join(filepath.Dir(configPath(b.Dir)), ".github", "ISSUE_TEMPLATE")
}

// IssueTemplates reads the labels each issue template in dir applies
// (.github/ISSUE_TEMPLATE): an issue form's labels, or a Markdown
// template's front matter, as a list or as "a, b". GitHub applies only
// the labels the repository has and drops the others without a word.
// config.yml is the template chooser, not a template. No dir: none.
func IssueTemplates(dir string) ([]IssueTemplate, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var res []IssueTemplate
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
			res = append(res, IssueTemplate{File: name, Labels: labels})
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
