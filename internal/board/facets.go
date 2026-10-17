package board

import (
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/filter"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// None is the value of an issue without the field, as BucketBy names it.
const None = "(none)"

// Facet is one section of the web page's filters: git-bug's open/closed
// ("is"), a field, or the plain labels ("label").
type Facet struct {
	Key string `json:"key"`
	// Options are the values on the issues the rest of the filter matches,
	// counted, and the values its own terms name.
	Options []Option `json:"options"`
	// Selected are the values the filter's own terms let through; none
	// when it has no term on this facet.
	Selected []string `json:"selected"`
	// Rest is the filter without the facet's own terms, the others as
	// typed: a tick writes the facet's new term after it.
	Rest string `json:"rest"`
}

// Option is one value of a facet, and how many issues have it.
type Option struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// Facets is the filters panel for filterStr: each facet counted against
// the other terms (OR within a facet, AND across), so the page never
// matches an issue itself. A facet with a single value says nothing and is
// left out, unless the filter picks values in it.
func Facets(b issueBackend, cfg *config.Config, filterStr, me string) ([]Facet, error) {
	conditions, me, err := prepare(b, filterStr, me)
	if err != nil {
		return nil, err
	}
	issues, err := b.ListIssues()
	if err != nil {
		return nil, err
	}
	today := time.Now()
	var out []Facet
	for _, key := range facetKeys(cfg, issues) {
		var own, rest []filter.Condition
		for _, c := range conditions {
			if owns(key, c) {
				own = append(own, c)
			} else {
				rest = append(rest, c)
			}
		}
		counts := map[string]int{}
		for _, issue := range issues {
			if filter.Matches(issue, rest, cfg, me, today) {
				for _, v := range valuesOf(issue, key) {
					counts[v]++
				}
			}
		}
		for _, c := range own {
			if c.Kind == filter.KindNo {
				continue
			}
			for _, v := range strings.Split(c.Value, ",") {
				v = filter.Resolve(v, cfg, me, today) // @current: today's sprint
				if _, ok := counts[v]; !ok && v != "" && !strings.HasPrefix(v, "\x00") {
					counts[v] = 0
				}
			}
		}
		f := Facet{Key: key, Selected: []string{}, Rest: filter.Without(filterStr, func(c filter.Condition) bool { return owns(key, c) })}
		for v, n := range counts {
			f.Options = append(f.Options, Option{v, n})
		}
		sort.Slice(f.Options, func(i, j int) bool {
			a, b := f.Options[i], f.Options[j]
			return a.Count > b.Count || a.Count == b.Count && a.Value < b.Value
		})
		// a value is picked when an issue with only it would get through
		for _, o := range f.Options {
			if len(own) > 0 && filter.Matches(only(key, o.Value), own, cfg, me, today) {
				f.Selected = append(f.Selected, o.Value)
			}
		}
		if len(f.Options) > 1 || len(f.Selected) > 0 {
			out = append(out, f)
		}
	}
	return out, nil
}

// facetKeys: is, the configured fields, the other label namespaces found
// on the issues (sprint, mod…), then plain labels.
func facetKeys(cfg *config.Config, issues []gitbugio.Issue) []string {
	var configured []string
	if cfg != nil {
		for name := range cfg.Fields {
			configured = append(configured, name)
		}
	}
	sort.Strings(configured)
	var found []string
	for _, issue := range issues {
		for _, l := range issue.Labels {
			if ns, _, ok := strings.Cut(l, ":"); ok && !slices.Contains(configured, ns) && !slices.Contains(found, ns) {
				found = append(found, ns)
			}
		}
	}
	sort.Strings(found)
	return append(append(append([]string{"is"}, configured...), found...), "label")
}

// owns: the terms that are a facet's, which a tick rewrites.
func owns(key string, c filter.Condition) bool {
	switch c.Kind {
	case filter.KindIs:
		return key == "is"
	case filter.KindNo:
		return c.Value == key // no:label is the plain labels'
	case filter.KindField:
		return c.Field == key
	case filter.KindLabel:
		return key == "label" && !strings.Contains(c.Value, ":")
	}
	return false
}

func valuesOf(issue gitbugio.Issue, key string) []string {
	if key == "is" {
		return []string{issue.Status}
	}
	var values []string
	for _, l := range issue.Labels {
		ns, v, ok := strings.Cut(l, ":")
		switch {
		case key == "label" && !ok:
			values = append(values, l)
		case ok && ns == key:
			values = append(values, v)
		}
	}
	if len(values) == 0 {
		return []string{None}
	}
	return values
}

// only is an issue with just value for key.
func only(key, value string) gitbugio.Issue {
	switch {
	case key == "is":
		return gitbugio.Issue{Status: value}
	case value == None:
		return gitbugio.Issue{}
	case key == "label":
		return gitbugio.Issue{Labels: []string{value}}
	}
	return gitbugio.Issue{Labels: []string{key + ":" + value}}
}
