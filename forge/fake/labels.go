package fake

import (
	"context"
	"slices"
	"strings"

	"github.com/guilbep/git-board/forge"
)

// The project's labels: what git board lint --create-labels reads and
// writes. Names ignore case, as GitHub's do.

var _ forge.Labeler = (*Forge)(nil)

// AddLabel gives the project a label, as its users would: one of the
// name already there is replaced.
func (f *Forge) AddLabel(l forge.Label) {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	if i := f.p.label(l.Name); i >= 0 {
		f.p.labels[i] = l
		return
	}
	f.p.labels = append(f.p.labels, l)
}

// Labels lists the project's labels, in the order they were made.
func (f *Forge) Labels(context.Context) ([]forge.Label, error) {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	return slices.Clone(f.p.labels), nil
}

// SetLabel makes the label, refused when the project has one of its
// name; with update, edits that one's color (unless l has none) and
// description, ErrNotFound when there is none.
func (f *Forge) SetLabel(_ context.Context, l forge.Label, update bool) error {
	op := "create-label"
	if update {
		op = "update-label"
	}
	return f.write(Write{Op: op, Args: []string{l.Name, l.Color, l.Description}}, func(p *project, _ int64) error {
		i := p.label(l.Name)
		switch {
		case !update && i >= 0:
			return &forge.Refused{Values: []string{l.Name}, Why: "the fake has this label already"}
		case !update:
			p.labels = append(p.labels, l)
		case i < 0:
			return forge.ErrNotFound
		default:
			if l.Color != "" {
				p.labels[i].Color = l.Color
			}
			p.labels[i].Description = l.Description
		}
		return nil
	})
}

// label is the index of the label of this name, -1 for none.
func (p *project) label(name string) int {
	return slices.IndexFunc(p.labels, func(l forge.Label) bool { return strings.EqualFold(l.Name, name) })
}
