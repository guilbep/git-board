package ghproject

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/guilbep/git-board/domain/issues"
)

// The fit (docs/prd/config-fit.md): what .git-board.toml lacks for the
// Project a team already has, as proposals. A proposal adds: an option, a
// Project name, a kind's alias, a kind, a sprint, a value left out by
// choice. It removes nothing. One git-board derives with no guess is
// ticked; another is a question, and writes nothing until answered.

// What a proposal writes.
const (
	FitOption   = "option"   // a value of a single select, and its Project name
	FitAlias    = "alias"    // [fields.kind.aliases]: a Category is a kind
	FitAddKind  = "add-kind" // [branches] kinds: a Category is a new kind
	FitUnmapped = "unmapped" // [github-project.unmapped]: left out by choice
	FitSprint   = "sprint"   // a [[sprints.list]] entry
)

// The answers a question takes, beside "map=<value>".
const (
	AnswerAddKind  = "add-kind"
	AnswerUnmapped = "unmapped"
)

// Proposal is one change to the file.
type Proposal struct {
	ID    string `json:"id"`    // field:name: status:Tests Metier, category:FIX, sprint:Sprint 12
	Field string `json:"field"` // status, category, sprint
	Name  string `json:"name"`  // the Project's name: an option, a Category, an iteration's title
	// Ticked: it is written. A question isn't, until Answer.
	Ticked bool `json:"ticked"`
	// Choices are the answers a question offers, "map=<value>" first.
	Choices []string `json:"choices"`
	Clears  int      `json:"clears"` // the warnings it clears
	Says    string   `json:"says"`   // what it is, a line
	Note    string   `json:"note,omitempty"`

	Do    string `json:"do,omitempty"`    // Fit*: what it writes; "" while a question
	Value string `json:"value,omitempty"` // the option, the kind, the sprint's slug
	// After is the file's option an option goes after, "" for the end;
	// Prev are the proposals between the two, the nearest first: one of
	// them written, the option goes after its value.
	After string   `json:"after,omitempty"`
	Prev  []string `json:"prev,omitempty"`
	Start string   `json:"start,omitempty"` // a sprint's first day
}

// synonyms are the Category values that are a kind in another word.
var synonyms = map[string]string{
	"FIX": "bug", "BUG": "bug", "FEATURE": "feat", "DOC": "docs", "DOCUMENTATION": "docs",
	"PERFORMANCE": "perf", "TESTS": "test",
}

// Fit is what the file lacks for the Project, from both sides as a check
// read them and the plan it made.
func Fit(s State, plan Plan, c *Config) []Proposal {
	clears := map[string]int{}
	var categories []string
	for _, ch := range plan.Changes {
		for _, w := range ch.Warnings {
			if w.Kind != WarnNoOption && w.Kind != WarnNotAKind {
				continue
			}
			id := w.Field + ":" + w.Value
			if clears[id]++; w.Kind == WarnNotAKind && clears[id] == 1 {
				categories = append(categories, w.Value)
			}
		}
	}
	var out []Proposal
	for _, f := range c.synced() {
		pf, ok := s.Meta.field(f.column)
		if !ok {
			continue
		}
		switch {
		case isIteration(pf, f.name):
			if f.name == "sprint" {
				out = append(out, c.fitSprints(s, f, pf)...)
			}
		case pf.Type == "SINGLE_SELECT" && !f.readOnly:
			out = append(out, c.fitOptions(s, f, pf, clears)...)
		}
	}
	slices.Sort(categories)
	for _, name := range categories {
		out = append(out, c.fitCategory(name, clears[CategoryField+":"+name]))
	}
	// the options, the kinds, then the sprints: as the file lists them
	slices.SortStableFunc(out, func(a, b Proposal) int {
		rank := func(p Proposal) int {
			switch p.Field {
			case CategoryField:
				return 1
			case "sprint":
				return 2
			}
			return 0
		}
		return cmp.Compare(rank(a), rank(b))
	})
	return out
}

// fitOptions are the Project's options of a single select that no option
// of the file matches, in the Project's order.
func (c *Config) fitOptions(s State, f synced, pf Field, clears map[string]int) []Proposal {
	fl := c.field(f.name)
	if len(fl.Options) == 0 { // every value is one
		return nil
	}
	// the stray values: what the board's labels carry that is no option
	var stray []string
	for _, is := range s.Issues {
		for _, l := range is.Labels {
			if v, ok := strings.CutPrefix(l, f.name+":"); ok && !fl.KnownValue(v) && !slices.Contains(stray, v) {
				stray = append(stray, v)
			}
		}
	}
	slices.Sort(stray)
	var out []Proposal
	after, prev, seen := "", []string(nil), map[string]bool{}
	for _, ch := range pf.Choices {
		name := ch.Name
		if f.name == "mod" {
			name = normalizeModule(name)
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		if v, ok := c.toLabel(pf, f.name, name); ok {
			after, prev = v, nil
			continue
		}
		slug := Slug(name)
		if slug == "" || c.unmapped(f.name, name) {
			continue
		}
		p := Proposal{ID: f.name + ":" + name, Field: f.name, Name: name, Clears: clears[f.name+":"+name],
			After: after, Prev: slices.Clone(prev)}
		if slices.Contains(stray, slug) || len(stray) == 0 {
			p.Ticked, p.Do, p.Value = true, FitOption, slug
		}
		out = append(out, p)
		prev = slices.Insert(prev, 0, p.ID)
	}
	// a question offers the slug, then the stray values no ticked one took
	for i, p := range out {
		if !p.Ticked {
			out[i].Choices = []string{"map=" + Slug(p.Name)}
			for _, v := range stray {
				if !slices.ContainsFunc(out, func(q Proposal) bool { return q.Ticked && q.Value == v }) {
					out[i].Choices = append(out[i].Choices, "map="+v)
				}
			}
			out[i].Choices = append(out[i].Choices, AnswerUnmapped)
		}
		out[i].describe()
	}
	return out
}

// fitCategory is a Category that is no kind: an alias when it is a kind
// in another word, else a question.
func (c *Config) fitCategory(name string, clears int) Proposal {
	p := Proposal{ID: CategoryField + ":" + name, Field: CategoryField, Name: name, Clears: clears}
	kind := synonyms[strings.ToUpper(name)]
	if _, ok := c.kinds()[kind]; !ok {
		kind = cmp.Or(c.kindNamed(name+"s"), c.kindNamed(strings.TrimSuffix(strings.ToLower(name), "s")))
	}
	if kind != "" {
		p.Ticked, p.Do, p.Value = true, FitAlias, kind
	} else {
		for _, k := range c.field("kind").Options {
			p.Choices = append(p.Choices, "map="+k)
		}
		p.Choices = append(p.Choices, AnswerAddKind, AnswerUnmapped)
	}
	p.describe()
	return p
}

// fitSprints are the Project's iterations with no [[sprints.list]]
// entry, by start date: each with the slug its issues' labels carry.
func (c *Config) fitSprints(s State, f synced, pf Field) []Proposal {
	labels := map[string][]string{}
	for _, is := range s.Issues {
		labels[is.URL] = is.Labels
	}
	var out []Proposal
	for _, ch := range pf.Choices {
		title := strings.TrimSpace(ch.Name)
		listed := c.Schema != nil && slices.ContainsFunc(c.Schema.Sprints.List, func(sp issues.Sprint) bool {
			return strings.EqualFold(strings.TrimSpace(sp.Title), title)
		})
		if listed || !dateRE.MatchString(ch.Start) || Slug(title) == "" {
			continue
		}
		// the slugs its issues carry, the most carried first
		count := map[string]int{}
		var slugs []string
		for _, it := range s.Items {
			if it.URL == "" || strings.TrimSpace(f.value(it)) != title {
				continue
			}
			if v, ok := localValue(labels[it.URL], "sprint"); ok && v != "" {
				if count[v]++; count[v] == 1 {
					slugs = append(slugs, v)
				}
			}
		}
		slices.SortStableFunc(slugs, func(a, b string) int { return cmp.Compare(count[b], count[a]) })
		p := Proposal{ID: "sprint:" + title, Field: "sprint", Name: title, Start: ch.Start}
		switch len(slugs) {
		case 0:
			p.Ticked, p.Do, p.Value = true, FitSprint, Slug(title)
		case 1:
			p.Ticked, p.Do, p.Value = true, FitSprint, slugs[0]
		default:
			renamed := 0
			for _, v := range slugs[1:] {
				p.Choices = append(p.Choices, "map="+v)
				renamed += count[v]
			}
			p.Choices = slices.Insert(p.Choices, 0, "map="+slugs[0])
			p.Note = fmt.Sprintf("%s renames the label on %d issue(s)", slugs[0], renamed)
		}
		p.describe()
		out = append(out, p)
	}
	slices.SortStableFunc(out, func(a, b Proposal) int { return strings.Compare(a.Start, b.Start) })
	return out
}

// describe writes what the proposal is, as it stands.
func (p *Proposal) describe() {
	switch {
	case p.Do == FitOption && Slug(p.Value) == Slug(p.Name):
		p.Says = fmt.Sprintf("%s: add the option %q, the Project's %q", p.Field, p.Value, p.Name)
	case p.Do == FitOption:
		p.Says = fmt.Sprintf("%s: add the option %q, named %q on the Project", p.Field, p.Value, p.Name)
	case p.Do == FitAlias:
		p.Says = fmt.Sprintf("kind: the Category %q is the kind %s", p.Name, p.Value)
	case p.Do == FitAddKind:
		p.Says = fmt.Sprintf("kind: add the kind %s, the Category %q", p.Value, p.Name)
	case p.Do == FitUnmapped:
		p.Says = fmt.Sprintf("%s: leave the Project's %q unmapped", p.Field, p.Name)
	case p.Do == FitSprint:
		p.Says = fmt.Sprintf("sprint: add %q (sprint:%s), from %s", p.Name, p.Value, p.Start)
	case p.Field == CategoryField:
		p.Says = fmt.Sprintf("kind: the Category %q is no kind", p.Name)
	case p.Field == "sprint":
		p.Says = fmt.Sprintf("sprint: the issues of %q carry several sprint labels", p.Name)
	default:
		p.Says = fmt.Sprintf("%s: the Project's %q has no option", p.Field, p.Name)
	}
}

// Answer settles a question: "map=<value>" (the option's value, the kind,
// the sprint's slug), "add-kind" for a Category, or "unmapped". A ticked
// proposal takes an answer too: the answer wins.
func (c *Config) Answer(p Proposal, answer string) (Proposal, error) {
	refuse := func(why string) (Proposal, error) {
		return p, fmt.Errorf("%s: %s (its answers: %s)", p.ID, why, strings.Join(p.Choices, ", "))
	}
	value, mapped := strings.CutPrefix(answer, "map=")
	value = strings.TrimSpace(value)
	switch {
	case answer == AnswerUnmapped && p.Field != "sprint":
		p.Do, p.Value = FitUnmapped, ""
	case answer == AnswerAddKind && p.Field == CategoryField:
		if p.Do, p.Value = FitAddKind, Slug(p.Name); p.Value == "" || c.kindNamed(p.Value) != "" {
			return refuse("no kind to add")
		}
	case !mapped || value == "" || strings.ContainsAny(value, ": \t"):
		return refuse(fmt.Sprintf("answer %q isn't one", answer))
	case p.Field == CategoryField:
		if p.Do, p.Value = FitAlias, c.kindNamed(value); p.Value == "" {
			return refuse(fmt.Sprintf("%q is none of [branches] kinds", value))
		}
	case p.Field == "sprint":
		p.Do, p.Value = FitSprint, value
	default:
		// an option that has its Project name already can't take another:
		// folding two of the Project's values into one isn't proposed
		if name, has := c.values(p.Field)[value]; has && name != p.Name {
			return refuse(fmt.Sprintf("%q is the Project's %q already: edit .git-board.toml to fold both", value, name))
		}
		p.Do, p.Value = FitOption, value
	}
	p.Ticked = true
	p.describe()
	return p, nil
}
