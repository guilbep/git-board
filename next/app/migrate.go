package app

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/domain/people"
	"github.com/guilbep/git-board/next/forge"
)

// The forge-mapping migration, v1's internal/migrate
// (docs/prd/forge-field-mappings.md): when a field's home on a forge
// changes (a [forges.<name>.fields.<field>] mapping), a sync run writes
// the new home by itself; the migration adds the old labels' removal
// from the forge's issues, and the values' first write when the new
// home is the assignees, which the sync only maintains. Changing a
// mapping moves nothing by itself: the preview counts, the user runs it.

// MigratePlan is what migrating one field on one forge would do, or did.
type MigratePlan struct {
	Forge  string `json:"forge"`
	Repo   string `json:"repo"`   // owner/repo, or the numeric id
	Target string `json:"target"` // the field's home there now
	Issues int    `json:"issues"` // issues here that carry the field
	Labels int    `json:"labels"` // the forge's labels the move takes off the issues
	// Note says what a sync does instead, when migration has nothing of
	// its own to write.
	Note string `json:"note,omitempty"`
}

// stake is one issue's part in a migration: its forge ref, and the
// field's values here.
type stake struct {
	ref    forge.Ref
	values []string
}

// MigrateReq asks MigrateField to preview or run one field's migration.
type MigrateReq struct {
	Field  string
	DryRun bool                                                    // the preview: counts, nothing written
	Say    func(line string)                                       // each forge's line, and each refusal; nil hears nothing
	Open   func(context.Context, forge.Table) (forge.Forge, error) // nil: OpenForges
}

// MigrateField previews (r.DryRun) or runs the migration of r.Field on
// every forge .git-board.toml declares. A forge that refuses keeps its
// note and the others go on: the error is the first refusal, the plans
// carry what landed.
func (b *Board) MigrateField(ctx context.Context, r MigrateReq) ([]MigratePlan, error) {
	field, dry, say := r.Field, r.DryRun, r.Say
	if say == nil {
		say = func(string) {}
	}
	open := r.Open
	if open == nil {
		open = func(ctx context.Context, f forge.Table) (forge.Forge, error) {
			f.Sync = true // OpenForges opens those that sync
			fs, err := b.OpenForges(ctx, []forge.Table{f})
			if err != nil {
				return nil, err
			}
			return fs[0].Forge, nil
		}
	}
	s := b.Issues.Schema
	if _, ok := s.Fields[field]; !ok {
		return nil, &Error{Class: Usage, Err: fmt.Errorf("no field named %s in %s", field, ".git-board.toml")}
	}
	all, err := b.Issues.List(ctx, issues.ListReq{Filter: "is:open,closed"})
	if err != nil {
		return nil, err
	}
	gp, _ := b.Config.Tools.GithubProject.Get()
	inProject := func(name string) bool {
		return len(s.Fields[name].Project) > 0 || gp.Number != 0 && slices.Contains(projectFields, name)
	}
	var persons []people.Person
	if !dry {
		dir, err := people.Read(ctx, b.Kernel, b.Issues.Avatars)
		if err != nil {
			return nil, err
		}
		for _, p := range dir.ByID {
			persons = append(persons, p)
		}
		slices.SortFunc(persons, func(x, y people.Person) int { return strings.Compare(string(x.ID), string(y.ID)) })
	}
	var plans []MigratePlan
	var first error
	for _, f := range b.Config.Forges.List {
		target := forge.Target(s, inProject, f, field)
		st := stakes(s, f, field, all)
		p := MigratePlan{Forge: f.Name, Repo: f.Project, Target: target, Issues: len(st)}
		if dry {
			switch target {
			case forge.TargetLabel, forge.TargetScopedLabel:
				p.Note = "still a label there — nothing to migrate"
			case forge.TargetProject:
				p.Note = "a sync run writes the Project column; migration takes the old labels off the forge"
			case forge.TargetAssignees:
				p.Note = "migration sets the assignees (git-board's accounts) and takes the labels off the forge"
			default:
				p.Note = "a sync run writes the new home; migration takes the old labels off the forge"
			}
			plans = append(plans, p)
			continue
		}
		switch target {
		case forge.TargetLabel, forge.TargetScopedLabel:
			p.Note = "still a label there — nothing to migrate"
			plans = append(plans, p)
			continue
		}
		fw, err := open(ctx, f)
		if err != nil {
			p.Note = err.Error()
			plans = append(plans, p)
			if first == nil {
				first = err
			}
			continue
		}
		labels, moved := 0, 0
		var ferr error
		for _, one := range st {
			ok := true
			if target == forge.TargetAssignees {
				accounts := make([]forge.Account, 0, len(one.values))
				for _, v := range one.values {
					accounts = append(accounts, account(persons, f.Host, v))
				}
				if _, err := fw.SetAssignees(ctx, one.ref, accounts); err != nil {
					say(fmt.Sprintf("%s #%s: the assignees refused: %v", f.Name, one.ref.Number, err))
					ferr, ok = err, false
				}
			}
			// the labels leave the issue, wherever the values went: they
			// are the old home's spelling, held off this forge since the
			// mapping, and the forge keeps them defined
			if ok {
				if err := fw.ChangeLabels(ctx, one.ref, nil, nsLabels(field, one.values)); err != nil {
					say(fmt.Sprintf("%s #%s: the labels refused: %v", f.Name, one.ref.Number, err))
					ferr, ok = err, false
				}
			}
			if ok {
				moved++
				labels += len(one.values)
			}
		}
		p.Labels = labels
		switch {
		case ferr == nil:
			p.Note = fmt.Sprintf("%d issues moved to %s, %d labels off the issues", len(st), target, labels)
		case moved == 0:
			p.Note = fmt.Sprintf("nothing moved: %v", ferr)
		default:
			p.Note = fmt.Sprintf("%d of %d issues moved to %s; the rest refused: %v", moved, len(st), target, ferr)
		}
		if first == nil {
			first = ferr
		}
		say(fmt.Sprintf("%s (%s): %s", f.Name, f.Project, p.Note))
		plans = append(plans, p)
	}
	return plans, first
}

// stakes are the issues carrying the field and linked on forge f, with
// their values as the storage has them (aliases folded).
func stakes(s *issues.Schema, f forge.Table, field string, all []issues.Issue) []stake {
	var out []stake
	for _, is := range all {
		var values []string
		for _, l := range is.Labels {
			if ns, v, ok := strings.Cut(l, ":"); ok && strings.EqualFold(ns, field) && v != "" {
				values = append(values, s.Fields[field].AliasOf(v))
			}
		}
		if len(values) == 0 {
			continue
		}
		// its link on this forge: v1 read the GitHub one alone
		i := slices.IndexFunc(is.Forges, func(l issues.Forge) bool { return urlOn(f, l.URL) })
		if i < 0 {
			continue
		}
		if ref := refOf(is.Forges[i].URL); ref.Number != "" {
			out = append(out, stake{ref: ref, values: values})
		}
	}
	return out
}

// refOf is the forge ref of a linked issue: its URL's last number
// (github.com/o/r/issues/2605 → 2605). No link, no ref.
func refOf(url string) forge.Ref {
	i := strings.LastIndexByte(url, '/')
	n := url[i+1:]
	if n == "" || strings.Trim(n, "0123456789") != "" {
		return forge.Ref{}
	}
	return forge.Ref{Number: n}
}

// urlOn says whether an issue's link points at forge f's project: the
// host, and on GitHub the owner/repo too, two forges of one host being
// the norm. A GitLab table names its project by numeric id, its issue
// URLs by path, so the host decides there (one project per table).
func urlOn(f forge.Table, url string) bool {
	u := strings.ToLower(url)
	host := "https://" + strings.ToLower(f.Host) + "/"
	if !strings.HasPrefix(u, host) {
		return false
	}
	if f.Kind == "github" {
		return strings.HasPrefix(u[len(host):], strings.ToLower(f.Project)+"/")
	}
	return true
}

// account is value's account on the forge's server: a login's, else a
// person name's, the sync's way. One no person carries goes as the
// login alone, and the forge refuses it where a login isn't enough.
func account(persons []people.Person, server, value string) forge.Account {
	for _, byName := range []bool{false, true} {
		for _, p := range persons {
			for _, a := range p.Accounts {
				if !strings.EqualFold(a.Server, server) {
					continue
				}
				if !byName && strings.EqualFold(a.Login, value) || byName && strings.EqualFold(p.Name, value) {
					return forge.Account{ID: a.ID, Login: a.Login}
				}
			}
		}
	}
	return forge.Account{Login: value}
}

// nsLabels is the field's labels as the forge has them, the namespaced
// spelling the storage keeps (status:ready), each once.
func nsLabels(field string, values []string) []string {
	var out []string
	for _, v := range values {
		if l := field + ":" + v; !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}
