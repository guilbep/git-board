// Package branches is an issue's code: the branch it suggests creating,
// named from [branches], and the branches whose name names it. The issue
// is the unit of work; its code hangs off it by name. Read-only: nothing
// here creates or pushes a branch.
package branches

import (
	"cmp"
	"regexp"
	"slices"
	"strings"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// defaultName is GitHub's: 2518-other-equipment-shows-the-wrong-example.
const defaultName = "{number}-{slug}"

// remote is where branches come from and go to.
// ponytail: origin only; a setting when a project's remote isn't origin
const remote = "origin"

// slugMax is about how long a slug gets, cut between words.
const slugMax = 50

// Name is the branch an issue suggests: cfg.Name with {kind} (the issue's
// kind through cfg.Kinds; without one, "{kind}/" is dropped), {number}
// (its first forge number, else its git-bug id's first 7 characters) and
// {slug} (its title), made a branch name git takes.
func Name(cfg config.Branches, is gitbugio.Issue) string {
	tmpl := cmp.Or(cfg.Name, defaultName)
	kind := kindOf(cfg, is.Labels)
	if kind == "" {
		tmpl = strings.ReplaceAll(tmpl, "{kind}/", "")
	}
	return refSafe(strings.NewReplacer("{kind}", kind, "{number}", number(is), "{slug}", Slug(is.Title)).Replace(tmpl))
}

// kindOf is the issue's kind: its kind: field, else the first of its flat
// labels that cfg.Kinds lists, through cfg.Kinds.
func kindOf(cfg config.Branches, labels []string) string {
	for _, l := range labels {
		if v, ok := strings.CutPrefix(l, "kind:"); ok {
			return cmp.Or(cfg.Kinds[v], v)
		}
	}
	for _, l := range slices.Sorted(slices.Values(labels)) {
		if k, ok := cfg.Kinds[l]; ok {
			return k
		}
	}
	return ""
}

func number(is gitbugio.Issue) string {
	if ns := is.Numbers(); len(ns) > 0 {
		return ns[0]
	}
	return is.ID[:min(7, len(is.ID))]
}

var (
	titlePrefix = regexp.MustCompile(`^\s*\[[^\]]*\][\s:-]*(\([^)]*\))?`) // [BUG](Equipment), [Bug]: (Planner)
	titleNumber = regexp.MustCompile(`\(#\d+\)\s*$`)                      // (#2518)
)

// Slug is a title as a branch says it: lower-case ASCII words between
// dashes, without a leading [PREFIX](Scope) or a trailing (#1234), cut
// between words at about 50 characters.
func Slug(title string) string {
	s := config.Slug(titleNumber.ReplaceAllString(titlePrefix.ReplaceAllString(title, ""), ""))
	if len(s) > slugMax {
		cut := strings.LastIndexByte(s[:slugMax+1], '-')
		if cut <= 0 {
			cut = slugMax
		}
		s = strings.TrimRight(s[:cut], "-")
	}
	return s
}

var refBad = regexp.MustCompile(`[^A-Za-z0-9._/-]+`)

// refSafe makes name a branch git takes (git check-ref-format --branch),
// and a word a shell takes as is: other characters become -, and no part
// starts or ends with . or -, holds .. or ends with .lock.
func refSafe(name string) string {
	var parts []string
	for _, p := range strings.Split(refBad.ReplaceAllString(name, "-"), "/") {
		for strings.Contains(p, "..") {
			p = strings.ReplaceAll(p, "..", ".")
		}
		p = strings.Trim(p, ".-")
		for strings.HasSuffix(p, ".lock") {
			p = strings.Trim(strings.TrimSuffix(p, ".lock"), ".-")
		}
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, "/")
}

// Names reports whether a branch's name names the issue: its forge number
// as the name's first number (fix/2518-…, 2518-…), or its git-bug id's
// first 7 characters or more as a word (abc1234-…).
// ponytail: the first number only, so a number in a slug (#12's
// fix/12-upgrade-node-24) names no other issue; a template putting a
// number before {number} (a year) links nothing
func Names(branch string, is gitbugio.Issue) bool {
	first := true
	for _, w := range strings.FieldsFunc(branch, func(r rune) bool { return !isAlnum(r) }) {
		if first && strings.Trim(w, "0123456789") == "" {
			first = false
			if is.HasNumber(w) {
				return true
			}
		}
		if len(w) >= 7 && strings.HasPrefix(is.ID, w) {
			return true
		}
	}
	return false
}

func isAlnum(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

// Suggestion is the branch to create for an issue, and how.
type Suggestion struct {
	Suggested string `json:"suggested"`
	Base      string `json:"base"`
	Start     string `json:"start"` // where it starts: origin/<base>, or <base> when only that exists
	Command   string `json:"command"`
}

// Suggest names the issue's branch, from the base: cfg.Base, else the
// remote's default branch, else the first of main, master and trunk there
// is, else main.
func Suggest(b *gitbugio.Backend, cfg config.Branches, is gitbugio.Issue) (Suggestion, error) {
	all, err := b.Branches()
	if err != nil {
		return Suggestion{}, err
	}
	has := func(r, name string) bool {
		return slices.ContainsFunc(all, func(br gitbugio.Branch) bool { return br.Remote == r && br.Name == name })
	}
	base := cmp.Or(cfg.Base, b.RemoteHead(remote))
	if base == "" {
		base = "main"
		for _, c := range []string{"main", "master", "trunk"} {
			if has("", c) || has(remote, c) {
				base = c
				break
			}
		}
	}
	start := remote + "/" + base
	if !has(remote, base) && has("", base) {
		start = base
	}
	name := Name(cfg, is)
	return Suggestion{Suggested: name, Base: base, Start: start, Command: "git switch -c " + name + " " + start}, nil
}

// Branch is a branch naming the issue, against the base.
type Branch struct {
	Name   string `json:"name"`
	Remote string `json:"remote,omitempty"`
	Ahead  int    `json:"ahead"`  // its commits the base hasn't
	Behind int    `json:"behind"` // the base's commits it hasn't
	Merged bool   `json:"merged"` // its head is in the base
	// InBase: its changes are in the base, merged or squashed or rebased in
	InBase     bool            `json:"in_base"`
	LastCommit gitbugio.Commit `json:"last_commit"`
}

// Linked lists the branches naming the issue, local then the remotes',
// against start (Suggestion.Start); without start, all at zero.
// ponytail: a branch's whole history, read per branch: fine for the few
// naming an issue; git's commit-graph if a big repository makes it slow
func Linked(b *gitbugio.Backend, is gitbugio.Issue, start string) ([]Branch, error) {
	all, err := b.Branches()
	if err != nil {
		return nil, err
	}
	at := func(br gitbugio.Branch) string { return strings.TrimPrefix(br.Remote+"/"+br.Name, "/") }
	startHash := ""
	for _, br := range all {
		if at(br) == start {
			startHash = br.Hash
		}
	}
	var base map[string]bool // read at the first branch naming the issue: most issues have none
	res := []Branch{}
	for _, br := range all {
		if at(br) == start || !Names(br.Name, is) {
			continue
		}
		if base == nil {
			base = map[string]bool{}
			if startHash != "" {
				if base, err = b.Ancestors(startHash); err != nil {
					return nil, err
				}
			}
		}
		last, err := b.ReadCommit(br.Hash)
		if err != nil {
			return nil, err
		}
		l := Branch{Name: br.Name, Remote: br.Remote, Merged: base[br.Hash], LastCommit: last}
		if len(base) > 0 {
			mine, err := b.Ancestors(br.Hash)
			if err != nil {
				return nil, err
			}
			for h := range mine {
				if !base[h] {
					l.Ahead++
				}
			}
			l.Behind = len(base) - (len(mine) - l.Ahead)
		}
		l.InBase = l.Merged || l.Ahead > 0 && startHash != "" && b.ChangesIn(startHash, br.Hash)
		res = append(res, l)
	}
	return res, nil
}
