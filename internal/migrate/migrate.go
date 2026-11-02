// Package migrate moves a field's values to the home its forge mapping
// names (docs/prd/forge-field-mappings.md): a run of sync writes the new
// target by itself; what migration adds is the old labels' removal from
// the forge, and the values' first write for a target the sync only
// maintains (the assignees). Changing a mapping moves nothing by itself —
// the preview counts, the user runs it.
package migrate

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/people"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
)

// Plan is what migrating one field on one forge would do, or did.
type Plan struct {
	Forge  string `json:"forge"`
	Repo   string `json:"repo"`   // owner/repo, or the numeric id
	Target string `json:"target"` // the field's home there now
	Issues int    `json:"issues"` // issues here that carry the field
	Labels int    `json:"labels"` // the forge's labels the move takes off the issues
	// Note says what a sync does instead, when migration has nothing of
	// its own to write.
	Note string `json:"note,omitempty"`
}

// Issue is one issue's stake in a migration: its forge ref, and the
// field's values here.
type stake struct {
	is     store.Issue
	ref    forge.Ref
	values []string
}

// refOf is the forge ref of a linked issue: its URL's last number
// (github.com/o/r/issues/2605 → 2605). No link, no stake.
func refOf(url string) forge.Ref {
	n := ""
	for i := len(url) - 1; i >= 0 && url[i] != '/'; i-- {
		if url[i] < '0' || url[i] > '9' {
			n = ""
			break
		}
		n = string(url[i]) + n
	}
	if n == "" {
		return forge.Ref{}
	}
	return forge.Ref{Number: n}
}

// urlOn says whether an issue's link points at this forge's project:
// the host, and on GitHub the owner/repo too — two forges of one host is
// the norm. A GitLab table names its project by numeric id, its issue
// URLs by path, so the host decides there (one project per table).
func urlOn(f config.Forge, url string) bool {
	u := strings.ToLower(url)
	if !strings.HasPrefix(u, "https://"+strings.ToLower(f.Host)+"/") {
		return false
	}
	if f.Kind == "github" {
		return strings.HasPrefix(u[len("https://"+f.Host+"/"):], strings.ToLower(f.Project)+"/")
	}
	return true
}

// stakes is, per forge, the issues carrying the field, with their values
// as the storage has them (namespaced labels; aliases folded).
func stakes(cfg *config.Config, f config.Forge, field string, issues []store.Issue) []stake {
	var out []stake
	for _, is := range issues {
		values := []string{}
		for _, l := range is.Labels {
			if ns, v, ok := strings.Cut(l, ":"); ok && strings.EqualFold(ns, field) && v != "" {
				values = append(values, cfg.Fields[field].AliasOf(v))
			}
		}
		if len(values) == 0 {
			continue
		}
		if !urlOn(f, is.GithubURL) {
			continue
		}
		ref := refOf(is.GithubURL)
		if ref.Number == "" {
			continue
		}
		out = append(out, stake{is: is, ref: ref, values: values})
	}
	return out
}

// Preview counts, per declared forge, what Migrate would do to field.
// No forge declares the field's home away from labels: nothing to
// migrate, the note says the sync carries the values.
func Preview(cfg *config.Config, field string, issues []store.Issue) []Plan {
	var plans []Plan
	for _, f := range cfg.Forges {
		target := cfg.FieldTarget(f, field)
		s := stakes(cfg, f, field, issues)
		p := Plan{Forge: f.Name, Repo: f.Project, Target: target, Issues: len(s)}
		switch target {
		case config.TargetLabel, config.TargetScopedLabel:
			p.Note = "still a label there — nothing to migrate"
		case config.TargetProject:
			p.Note = "a sync run writes the Project column; migration takes the old labels off the forge"
		case config.TargetAssignees:
			p.Note = "migration sets the assignees (git-board's accounts) and takes the labels off the forge"
		default:
			p.Note = "a sync run writes the new home; migration takes the old labels off the forge"
		}
		plans = append(plans, p)
	}
	return plans
}

// account is value's account on the forge's server: a login's, else a
// person name's — the sync's way (core/syncer's assignee). GitLab assigns
// by user id, so the migration resolves the same people the sync does;
// one no person carries goes as the login alone, and the forge refuses it
// where a login isn't enough.
func account(persons []people.Person, server, value string) (forge.Account, bool) {
	for _, byName := range []bool{false, true} {
		for _, pe := range persons {
			for _, a := range pe.Accounts {
				if !strings.EqualFold(a.Server, server) {
					continue
				}
				if (!byName && strings.EqualFold(a.Login, value)) || (byName && strings.EqualFold(pe.Name, value)) {
					return forge.Account{ID: a.ID, Login: a.Login}, true
				}
			}
		}
	}
	return forge.Account{}, false
}

// Migrate writes what Preview counts, through the forge adapters with
// their token rule: per issue carrying the field, the values to the
// target (the assignees alone, the one native slot the adapters set per
// issue, resolved through the persons' accounts), then the field's labels
// off the issue. say reports each forge. A forge that refuses keeps its
// note and the others go on; the error is the first refusal, the plans
// carry what landed.
func Migrate(ctx context.Context, cfg *config.Config, field string, issues []store.Issue, persons []people.Person, open func(config.Forge) (forge.Forge, error), say func(string)) ([]Plan, error) {
	var plans []Plan
	var firstErr error
	for _, f := range cfg.Forges {
		target := cfg.FieldTarget(f, field)
		s := stakes(cfg, f, field, issues)
		p := Plan{Forge: f.Name, Repo: f.Project, Target: target, Issues: len(s)}
		switch target {
		case config.TargetLabel, config.TargetScopedLabel:
			p.Note = "still a label there — nothing to migrate"
			plans = append(plans, p)
			continue
		}
		fw, err := open(f)
		if err != nil {
			p.Note = err.Error()
			plans = append(plans, p)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		labels, moved := 0, 0
		var ferr error
		for _, st := range s {
			ok := true
			if target == config.TargetAssignees {
				accounts := make([]forge.Account, 0, len(st.values))
				for _, v := range st.values {
					if a, resolved := account(persons, f.Host, v); resolved {
						accounts = append(accounts, a)
					} else {
						accounts = append(accounts, forge.Account{Login: v})
					}
				}
				if _, err := fw.SetAssignees(ctx, st.ref, accounts); err != nil {
					say(fmt.Sprintf("%s #%s: the assignees refused: %v", f.Name, st.ref.Number, err))
					ferr, ok = err, false
				}
			}
			// the labels leave the issue, wherever the values went: they
			// are the old home's spelling, held off this forge since the
			// mapping, and the forge keeps them defined
			if ok {
				if err := fw.ChangeLabels(ctx, st.ref, nil, nsLabels(field, st.values)); err != nil {
					say(fmt.Sprintf("%s #%s: the labels refused: %v", f.Name, st.ref.Number, err))
					ferr, ok = err, false
				}
			}
			if ok {
				moved++
				labels += len(st.values)
			}
		}
		p.Labels = labels
		switch {
		case ferr == nil:
			p.Note = fmt.Sprintf("%d issues moved to %s, %d labels off the issues", len(s), target, labels)
		case moved == 0:
			p.Note = fmt.Sprintf("nothing moved: %v", ferr)
		default:
			p.Note = fmt.Sprintf("%d of %d issues moved to %s; the rest refused: %v", moved, len(s), target, ferr)
		}
		say(fmt.Sprintf("%s (%s): %s", f.Name, f.Project, p.Note))
		plans = append(plans, p)
	}
	return plans, firstErr
}

// nsLabels is the field's labels as the forge has them, the namespaced
// spelling the storage keeps (status:ready), each once.
func nsLabels(field string, values []string) []string {
	var out []string
	for _, v := range values {
		label := field + ":" + v
		if !slices.Contains(out, label) {
			out = append(out, label)
		}
	}
	return out
}
