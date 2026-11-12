package app

import (
	"fmt"
	"slices"

	"github.com/guilbep/git-board/conf"
	"github.com/guilbep/git-board/tools/ghproject"
)

// The fit (docs/prd/config-fit.md): the proposals of a Project check
// (ghproject.Fit) written to .git-board.toml. Each adds its lines with a
// conf editor, so every other line, comment and order stays, and the
// whole goes through SaveConfig: checked, and refused when the file
// moved since the check.

// Fitted is what a fit writes, or would.
type Fitted struct {
	Text    string               `json:"-"`
	Diff    string               `json:"diff"`    // diff -u of the file
	Changes int                  `json:"changes"` // the proposals written
	Clears  int                  `json:"clears"`  // the warnings they clear
	Open    []ghproject.Proposal `json:"open"`    // the questions left: nothing written for them
	Written bool                 `json:"written"`
}

// Fit answers the proposals of a check and writes those taken to the
// file: every ticked or answered one, or of them those take names (nil:
// all). answers are by proposal id (ghproject.Answer). With dry nothing
// is written. The file must still be the one the check read.
func (b *Board) Fit(r ghproject.Report, take []string, answers map[string]string, dry bool) (Fitted, error) {
	c, err := b.Config.Project()
	if err != nil {
		return Fitted{}, err
	}
	known := map[string]bool{}
	var ps []ghproject.Proposal
	res := Fitted{Open: []ghproject.Proposal{}}
	for _, p := range r.Fit {
		known[p.ID] = true
		if a, ok := answers[p.ID]; ok {
			if p, err = c.Answer(p, a); err != nil {
				return Fitted{}, usage(err)
			}
		}
		switch {
		case !p.Ticked:
			res.Open = append(res.Open, p)
		case take == nil || slices.Contains(take, p.ID):
			ps = append(ps, p)
			res.Changes, res.Clears = res.Changes+1, res.Clears+p.Clears
		}
	}
	for id := range answers {
		if !known[id] {
			return Fitted{}, usage(fmt.Errorf("%s: the check proposes nothing for it", id))
		}
	}
	cur, _, err := ConfigText(b.Dir)
	if err != nil {
		return Fitted{}, err
	}
	if res.Text, err = fitText(string(cur), c, ps); err != nil {
		return Fitted{}, usage(err)
	}
	res.Diff = conf.Diff(conf.FileName, conf.FileName+" (fitted)", string(cur), res.Text)
	if err := SaveConfig(b.Dir, []byte(res.Text), r.ConfigVersion, dry); err != nil {
		return res, err
	}
	res.Written = !dry && res.Text != string(cur)
	return res, nil
}

// fitText is text with each proposal written, in their order.
func fitText(text string, c *ghproject.Config, ps []ghproject.Proposal) (string, error) {
	written := map[string]string{} // an option's value, by its proposal
	var err error
	for _, p := range ps {
		switch p.Do {
		case ghproject.FitOption:
			after := p.After
			for _, id := range p.Prev {
				if v, ok := written[id]; ok {
					after = v
					break
				}
			}
			written[p.ID] = p.Value
			table := "fields." + p.Field
			if !slices.Contains(c.Schema.Fields[p.Field].Options, p.Value) {
				if text, err = conf.InsertInList(text, table, "options", p.Value, after); err != nil {
					return "", err
				}
			}
			if ghproject.Slug(p.Value) == ghproject.Slug(p.Name) {
				continue
			}
			// the Project's name, in the form the field's mapping has
			if len(c.Forge.Fields[p.Field].Values) > 0 {
				table = "forges." + c.Forge.Name + ".fields." + p.Field
				text = conf.AddInline(text, table, "values", p.Value, conf.Quote(p.Name))
				continue
			}
			text = conf.AddInline(text, table, "project", p.Value, conf.Quote(p.Name))
		case ghproject.FitAlias:
			text = conf.AddInline(text, "fields.kind", "aliases", p.Name, conf.Quote(p.Value))
		case ghproject.FitAddKind:
			text = conf.AddInline(text, "branches", "kinds", p.Value, conf.Quote(p.Value))
		case ghproject.FitUnmapped:
			if text, err = conf.InsertInList(text, "github-project.unmapped", p.Field, p.Name, ""); err != nil {
				return "", err
			}
		case ghproject.FitSprint:
			block := fmt.Sprintf("[[sprints.list]]\nslug = %s\ntitle = %s\nstart = %s\n", conf.Quote(p.Value), conf.Quote(p.Name), p.Start)
			text = conf.AddArrayTable(text, "sprints.list", "sprints", block)
		}
	}
	return text, nil
}
