// Package fromforge starts a board's .git-board.toml from its forge:
// origin's GitHub labels and Project, written over a template, which fills
// only what the forge doesn't have (docs/prd/board-template.md). The forge
// wins where both have something: its names, colors, descriptions, its
// fields' values in order. Read-only against GitHub.
package fromforge

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/guilbep/git-board/core/fields"
	"github.com/guilbep/git-board/core/forge/github"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghbridge"
	"github.com/guilbep/git-board/internal/ghlabels"
)

// Forge is what the forge has: its labels and its first Project, nil for
// none.
type Forge struct {
	Labels  []ghlabels.Label
	Project *github.Project
}

// Read reads repo's labels and Projects (owner/name), through ghlabels.API
// and the forge sync's token rule. What it couldn't read is in notes: the
// file is the template's there.
func Read(ctx context.Context, repo string) (f Forge, notes []string) {
	g, err := github.New(ctx, github.Options{Project: repo, API: ghlabels.API})
	if err != nil {
		return f, []string{fmt.Sprintf("%s not read (%v): the template's labels and fields", repo, err)}
	}
	labels, err := g.Labels(ctx)
	if err != nil {
		notes = append(notes, fmt.Sprintf("%s's labels not read (%v): the template's", repo, err))
	}
	for _, l := range labels {
		f.Labels = append(f.Labels, ghlabels.Label{Name: l.Name, Color: l.Color, Description: l.Description})
	}
	projects, err := g.Projects(ctx)
	switch {
	case err != nil:
		notes = append(notes, fmt.Sprintf("%s's Projects not read (%v): the template's fields", repo, err))
	case len(projects) > 1:
		notes = append(notes, fmt.Sprintf("%s has %d Projects: fields from the first, %s", repo, len(projects), projects[0].Title))
		fallthrough
	case len(projects) == 1:
		f.Project = &projects[0]
	}
	return f, notes
}

// plainValue is a value the forge's name can be as it is: P0, Must.
var plainValue = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// colorOf is a Project color as the file writes it: GRAY is gray.
func colorOf(c string) string { return strings.ToLower(c) }

// Merge is template with the forge written over it. It checks, or errs.
func Merge(template string, f Forge) (string, []string, error) {
	cfg, _, err := config.Check([]byte(template))
	if err != nil {
		return "", nil, fmt.Errorf("the template: %w", err)
	}
	text, notes := template, []string(nil)
	set := map[string]fields.Field{}
	fromProject := map[string]bool{}
	if p := f.Project; p != nil {
		cfg.GithubProject = config.GithubProject{Owner: p.Owner, Number: p.Number}
		text = config.WithGithubProject(text, cfg.GithubProject)
		columns := ghbridge.ProjectColumns(cfg)
		for _, pf := range p.Fields {
			name := cmp.Or(columns[pf.Name], config.Slug(pf.Name))
			fl, known := cfg.Fields[name]
			switch pf.DataType {
			case "SINGLE_SELECT":
				fl = selectFrom(fl, pf)
				if !known && len(fl.Project) == 0 {
					notes = append(notes, fmt.Sprintf("the Project's %s is the field %s, kept in labels: Settings › Fields moves it to the Project", pf.Name, name))
				}
			case "NUMBER":
				if known {
					continue
				}
				fl = fields.Field{Single: true, Numeric: true}
			case "ITERATION":
				fl.Single, fl.Type, fl.Options = true, "iteration", nil
				if pf.Duration > 0 && cfg.Sprints.LengthDays != pf.Duration {
					s := cfg.Sprints
					s.LengthDays = pf.Duration
					if text, err = config.SetSprintsText(text, s); err != nil {
						return "", nil, err
					}
				}
				// ponytail: startDay's weekday unconfirmed (GitHub's encoding):
				// the template's start_weekday stays
			case "DATE":
				if known {
					continue
				}
				fl = fields.Field{Single: true, Type: "date"}
			default: // GitHub's own: title, assignees, labels, sub-issues…
				continue
			}
			set[name], fromProject[name] = fl, true
			cfg.Fields[name] = fl
		}
	}

	kinds := fields.Field{Colors: map[string]string{}, Descriptions: map[string]string{}}
	plain := cfg.Labels
	for _, l := range f.Labels {
		color := "#" + strings.ToLower(l.Color)
		field, value, scoped := strings.Cut(l.Name, ":")
		switch {
		case scoped && fromProject[field]:
			notes = append(notes, fmt.Sprintf("the label %s left out: %s lives in the Project", l.Name, field))
		case scoped:
			fl := set[field]
			if _, ok := set[field]; !ok {
				fl = cfg.Fields[field]
				fl.Single = fl.Single || len(fl.Options) == 0
			}
			if !slices.ContainsFunc(fl.Options, func(o string) bool { return config.Slug(o) == config.Slug(value) }) {
				fl.Options = append(fl.Options, value)
			}
			opt := fl.Options[slices.IndexFunc(fl.Options, func(o string) bool { return config.Slug(o) == config.Slug(value) })]
			fl.Colors, fl.Descriptions = with(fl.Colors, opt, color), with(fl.Descriptions, opt, l.Description)
			set[field] = fl
		case cfg.Branches.Kinds[l.Name] != "":
			kinds.Colors[l.Name], kinds.Descriptions[l.Name] = color, l.Description
		default:
			if !slices.Contains(plain.List, l.Name) {
				plain.List = append(plain.List, l.Name)
			}
			plain.Colors, plain.Descriptions = with(plain.Colors, l.Name, color), with(plain.Descriptions, l.Name, l.Description)
		}
	}
	if text, err = config.SetFieldsText(text, set, nil); err != nil {
		return "", nil, err
	}
	if len(kinds.Colors) > 0 {
		old := cfg.Fields["kind"]
		text = config.SetFieldTable(text, "kind", "colors", merged(old.Colors, kinds.Colors))
		text = config.SetFieldTable(text, "kind", "descriptions", merged(old.Descriptions, kinds.Descriptions))
	}
	if len(plain.List) > len(cfg.Labels.List) || !maps.Equal(plain.Colors, cfg.Labels.Colors) {
		if text, err = config.SetLabelsText(text, plain); err != nil {
			return "", nil, err
		}
	}
	if _, _, err := config.Check([]byte(text)); err != nil {
		return "", nil, fmt.Errorf("the file from the forge: %w", err)
	}
	return text, notes, nil
}

// selectFrom is fl with the Project field's options, in its order: one the
// field has by its slug, or by its rename, keeps the field's spelling
// (Wont for Won't); a new one is the forge's name, slugged when it can't
// be a value as it is (In progress is in-progress). The forge's colors
// and descriptions win; an option the Project lacks goes.
func selectFrom(fl fields.Field, pf github.ProjectField) fields.Field {
	out := fields.Field{Single: true, Aliases: fl.Aliases, Colors: map[string]string{}, Descriptions: map[string]string{}}
	for _, o := range pf.Options {
		opt := ""
		for _, have := range fl.Options {
			if config.Slug(have) == config.Slug(o.Name) || config.Slug(fl.Project[have]) == config.Slug(o.Name) {
				opt = have
				break
			}
		}
		if opt == "" {
			opt = o.Name
			if !plainValue.MatchString(opt) {
				opt = config.Slug(opt)
			}
		}
		out.Options = append(out.Options, opt)
		if config.Slug(opt) != config.Slug(o.Name) {
			out.Project = with(out.Project, opt, o.Name)
		}
		out.Colors[opt] = cmp.Or(colorOf(o.Color), fl.Colors[opt])
		out.Descriptions[opt] = cmp.Or(o.Description, fl.Descriptions[opt])
	}
	return out
}

func with(m map[string]string, k, v string) map[string]string {
	if m == nil {
		m = map[string]string{}
	}
	if v != "" {
		m[k] = v
	}
	return m
}

func merged(a, b map[string]string) map[string]string {
	res := maps.Clone(a)
	if res == nil {
		res = map[string]string{}
	}
	maps.Copy(res, b)
	return res
}
