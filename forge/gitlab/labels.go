package gitlab

import (
	"cmp"
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/guilbep/git-board/forge"
)

// The project's labels (git board lint --create-labels), as GitHub's
// adapter has them: colors hex without #, which GitLab writes with one.

var _ forge.Labeler = (*Forge)(nil)

// defaultColor is a label's color when none is asked: GitLab requires
// one, and this is the grey GitHub gives a label made without.
const defaultColor = "ededed"

type glLabel struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

func (l glLabel) key() int64 { return l.ID }

// Labels lists the labels the project's issues can take: its own, and
// its groups'.
func (f *Forge) Labels(ctx context.Context) ([]forge.Label, error) {
	var out []forge.Label
	err := each(ctx, f, f.p.api+"/labels", nil, func(l glLabel) (bool, error) {
		out = append(out, forge.Label{Name: l.Name, Color: strings.TrimPrefix(l.Color, "#"), Description: l.Description})
		return true, nil
	})
	return out, err
}

// SetLabel creates the label, or with update edits the one of its name:
// its color and description. A label created without a color gets
// defaultColor.
func (f *Forge) SetLabel(ctx context.Context, l forge.Label, update bool) error {
	in := map[string]string{"description": l.Description}
	if update {
		if l.Color != "" {
			in["color"] = "#" + l.Color
		}
		_, err := f.call(ctx, http.MethodPut, f.p.api+"/labels/"+url.PathEscape(l.Name), nil, in, nil)
		return err
	}
	in["name"], in["color"] = l.Name, "#"+cmp.Or(l.Color, defaultColor)
	_, err := f.call(ctx, http.MethodPost, f.p.api+"/labels", nil, in, nil)
	return err
}
