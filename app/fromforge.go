package app

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/guilbep/git-board/conf"
	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/github"
	"github.com/guilbep/git-board/tools/ghproject"
)

// git board init from origin's forge, as v1's (7a546cc:
// internal/fromforge): a board's .git-board.toml starts as its forge's
// labels and first GitHub Project, written over a template, which fills
// only what the forge doesn't have (docs/prd/board-template.md). The
// forge wins where both have something: its names, colors,
// descriptions, its fields' values in order. Read-only against the
// forge. GitLab's labels too: GitLab has no Projects.

// FromForge is what the forge has: its labels and its first Project, nil
// for none.
type FromForge struct {
	Labels  []forge.Label
	Project *github.Project
}

// projectLister is a forge that lists its repository's GitHub Projects:
// GitHub's adapter.
type projectLister interface {
	Projects(ctx context.Context) ([]github.Project, error)
}

// ReadForge reads the labels and the Projects of the forge project f
// names, through its adapter (OpenLabeler) and the forge sync's token
// rule (ADR 0010). What it couldn't read is in notes: the file is the
// template's there.
func ReadForge(ctx context.Context, t forge.Table) (f FromForge, notes []string) {
	repo := t.Project
	l, err := OpenLabeler(ctx, t)
	if err != nil {
		return f, []string{fmt.Sprintf("%s not read (%v): the template's labels and fields", repo, err)}
	}
	labels, err := l.Labels(ctx)
	if err != nil {
		notes = append(notes, fmt.Sprintf("%s's labels not read (%v): the template's", repo, err))
	}
	f.Labels = labels
	pl, ok := l.(projectLister)
	if !ok {
		if t.Kind == "gitlab" {
			notes = append(notes, fmt.Sprintf("%s is on GitLab, which has no Projects: the template's fields", repo))
		}
		return f, notes
	}
	projects, err := pl.Projects(ctx)
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

// ProjectName is the Project as init's line names it: owner/number,
// title; "none".
func (f FromForge) ProjectName() string {
	if f.Project == nil {
		return "none"
	}
	return fmt.Sprintf("%s/%d, %s", f.Project.Owner, f.Project.Number, f.Project.Title)
}

// plainValue is a value the forge's name can be as it is: P0, Must.
var plainValue = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// checkText reads text as the project file: what a write checks.
func checkText(text string) (*Config, error) {
	f, err := conf.Parse(conf.FileName, []byte(text))
	if err != nil {
		return nil, err
	}
	return readConfig(f, nil)
}

// MergeForge is template with the forge written over it, and notes on
// what went where. It checks, or errs.
func MergeForge(template string, f FromForge) (string, []string, error) {
	cfg, err := checkText(template)
	if err != nil {
		return "", nil, fmt.Errorf("the template: %w", err)
	}
	s := cfg.Schema
	if s == nil {
		s = &issues.Schema{}
	}
	fields := maps.Clone(s.Fields)
	if fields == nil {
		fields = map[string]issues.Field{}
	}
	text, notes := template, []string(nil)
	set := map[string]issues.Field{}
	fromProject := map[string]bool{}
	if p := f.Project; p != nil {
		text = conf.ReplaceTable(text, "github-project", fmt.Sprintf("[github-project]\nowner = %q\nnumber = %d\n", p.Owner, p.Number))
		gp := ghproject.Config{Owner: p.Owner, Number: p.Number, Schema: s, Forge: cfg.githubForge()}
		columns := gp.Columns()
		for _, pf := range p.Fields {
			name := cmp.Or(columns[pf.Name], ghproject.Slug(pf.Name))
			fl, known := fields[name]
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
				fl = issues.Field{Single: true, Numeric: true}
			case "ITERATION":
				fl.Single, fl.Type, fl.Options = true, issues.TypeIteration, nil
				if pf.Duration > 0 && s.Sprints.LengthDays != pf.Duration {
					text = conf.SetKeys(text, "sprints", []string{"length_days"}, map[string]string{"length_days": strconv.Itoa(pf.Duration)})
				}
				// ponytail: startDay's weekday unconfirmed (GitHub's
				// encoding): the template's start_weekday stays
			case "DATE":
				if known {
					continue
				}
				fl = issues.Field{Single: true, Type: issues.TypeDate}
			default: // GitHub's own: title, assignees, labels, sub-issues…
				continue
			}
			set[name], fromProject[name] = fl, true
			fields[name] = fl
		}
	}

	kinds := issues.Field{Colors: map[string]string{}, Descriptions: map[string]string{}}
	plain := s.Labels
	for _, l := range f.Labels {
		color := "#" + strings.ToLower(l.Color)
		field, value, scoped := strings.Cut(l.Name, ":")
		switch {
		case scoped && fromProject[field]:
			notes = append(notes, fmt.Sprintf("the label %s left out: %s lives in the Project", l.Name, field))
		case scoped:
			fl, ok := set[field]
			if !ok {
				fl = fields[field]
				fl.Single = fl.Single || len(fl.Options) == 0
			}
			same := func(o string) bool { return ghproject.Slug(o) == ghproject.Slug(value) }
			if !slices.ContainsFunc(fl.Options, same) {
				fl.Options = append(fl.Options, value)
			}
			opt := fl.Options[slices.IndexFunc(fl.Options, same)]
			fl.Colors, fl.Descriptions = with(fl.Colors, opt, color), with(fl.Descriptions, opt, l.Description)
			set[field] = fl
		case s.Kinds[l.Name] != "":
			kinds.Colors[l.Name], kinds.Descriptions[l.Name] = color, l.Description
		default:
			if !slices.Contains(plain.List, l.Name) {
				plain.List = append(plain.List, l.Name)
			}
			plain.Colors, plain.Descriptions = with(plain.Colors, l.Name, color), with(plain.Descriptions, l.Name, l.Description)
		}
	}
	if text, err = setFieldsText(text, set); err != nil {
		return "", nil, err
	}
	if len(kinds.Colors) > 0 {
		old := fields["kind"]
		text = conf.SetSubTable(text, "fields.kind", "colors", merged(old.Colors, kinds.Colors))
		text = conf.SetSubTable(text, "fields.kind", "descriptions", merged(old.Descriptions, kinds.Descriptions))
	}
	if len(plain.List) > len(s.Labels.List) || !maps.Equal(plain.Colors, s.Labels.Colors) {
		text = setLabelsText(text, plain)
	}
	if _, err := checkText(text); err != nil {
		return "", nil, fmt.Errorf("the file from the forge: %w", err)
	}
	return text, notes, nil
}

// selectFrom is fl with the Project field's options, in its order: one the
// field has by its slug, or by its rename, keeps the field's spelling
// (Wont for Won't); a new one is the forge's name, slugged when it can't
// be a value as it is (In progress is in-progress). The forge's colors
// and descriptions win; an option the Project lacks goes.
func selectFrom(fl issues.Field, pf github.ProjectField) issues.Field {
	out := issues.Field{Single: true, Aliases: fl.Aliases, Colors: map[string]string{}, Descriptions: map[string]string{}}
	for _, o := range pf.Options {
		opt := ""
		for _, have := range fl.Options {
			if ghproject.Slug(have) == ghproject.Slug(o.Name) || ghproject.Slug(fl.Project[have]) == ghproject.Slug(o.Name) {
				opt = have
				break
			}
		}
		if opt == "" {
			opt = o.Name
			if !plainValue.MatchString(opt) {
				opt = ghproject.Slug(opt)
			}
		}
		out.Options = append(out.Options, opt)
		if ghproject.Slug(opt) != ghproject.Slug(o.Name) {
			out.Project = with(out.Project, opt, o.Name)
		}
		out.Colors[opt] = cmp.Or(strings.ToLower(o.Color), fl.Colors[opt])
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

// setFieldsText is text with each field of set holding its value, by
// name, one table at a time: [fields.<name>]'s keys in place (a new
// table after the last [fields.*] one), then its aliases, project,
// colors and descriptions sub-tables. Every other field keeps its lines.
func setFieldsText(text string, set map[string]issues.Field) (string, error) {
	keys := []string{"single", "options", "numeric", "type", "unit"}
	for _, name := range slices.Sorted(maps.Keys(set)) {
		if !conf.SimpleName(name) {
			return "", fmt.Errorf("a field's name is letters, digits, - and _, not %q", name)
		}
		f := set[name]
		// every key: "" drops one the field hasn't
		vals := map[string]string{"single": "", "options": conf.List(f.Options), "numeric": "", "type": "", "unit": ""}
		if f.Single {
			vals["single"] = "true"
		}
		if f.Numeric {
			vals["numeric"] = "true"
		}
		if f.Type != "" {
			vals["type"] = conf.Quote(f.Type)
		}
		if f.Unit != "" {
			vals["unit"] = conf.Quote(f.Unit)
		}
		text = conf.SetKeysAfter(text, "fields."+name, conf.LastTable(text, "fields"), keys, vals)
		text = conf.SetSubTable(text, "fields."+name, "aliases", f.Aliases)
		text = conf.SetSubTable(text, "fields."+name, "project", f.Project)
		text = conf.SetSubTable(text, "fields."+name, "colors", f.Colors)
		text = conf.SetSubTable(text, "fields."+name, "descriptions", f.Descriptions)
	}
	return text, nil
}

// setLabelsText is text with [labels] holding l: its list, explicit even
// when empty (an empty list allows none), then its colors and
// descriptions. Without a table and nothing to hold, text as it is.
func setLabelsText(text string, l issues.Labels) string {
	if !l.Defined && len(l.List) == 0 && len(l.Colors) == 0 && len(l.Descriptions) == 0 {
		return text
	}
	list := conf.List(l.List)
	if list == "" {
		list = "[]"
	}
	text = conf.SetKeys(text, "labels", []string{"list"}, map[string]string{"list": list})
	text = conf.SetSubTable(text, "labels", "colors", l.Colors)
	return conf.SetSubTable(text, "labels", "descriptions", l.Descriptions)
}
