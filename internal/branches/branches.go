// Package branches is an issue's code: the branch it suggests creating,
// named from [branches], and the branches whose name names it. The issue
// is the unit of work; its code hangs off it by name, each branch in the
// issue's folder (fix/2518/1-…, fix/2518/2-…). Read-only: the server
// creates the branch (store.CreateBranch), from the name made here.
package branches

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
)

// defaultName puts each branch in the issue's folder, the first one too:
// 2518/1-other-equipment-shows-the-wrong-example (docs/design/issue-head,
// decision 3).
const defaultName = "{number}/{n}-{slug}"

// remote is where branches come from and go to.
// ponytail: origin only; a setting when a project's remote isn't origin
const remote = "origin"

// slugMax is about how long a slug gets, cut between words.
const slugMax = 50

// Name is the issue's first branch: NameAt with {n} at 1.
func Name(cfg config.Branches, is store.Issue) string {
	return NameAt(cfg, is, 1)
}

// NameAt is the branch an issue suggests: cfg.Name with {kind} (the
// issue's kind through cfg.Kinds; without one, "{kind}/" is dropped),
// {number} (its project number with its prefix in lower case, vm-62: a
// bare 62 is a forge's number to a forge; else its first forge number
// before the project numbers its issues, else its git-bug id's first 7
// characters), {n} (the branch's place in the issue's folder) and {slug}
// (its title), made a branch name git takes.
func NameAt(cfg config.Branches, is store.Issue, n int) string {
	tmpl := cmp.Or(cfg.Name, defaultName)
	kind := kindOf(cfg, is.Labels)
	if kind == "" {
		tmpl = strings.ReplaceAll(tmpl, "{kind}/", "")
	}
	return refSafe(strings.NewReplacer("{kind}", kind, "{number}", number(is), "{n}", strconv.Itoa(n), "{slug}", Slug(is.Title)).Replace(tmpl))
}

// nSentinel stands for {n} while the folder is found: digits no name holds.
const nSentinel = 987654321

// Folder is what the issue's branches share before {n}: "fix/2518/" for
// "{kind}/{number}/{n}-{slug}"; "" when the template has no {n}.
func Folder(cfg config.Branches, is store.Issue) string {
	name := NameAt(cfg, is, nSentinel)
	i := strings.Index(name, strconv.Itoa(nSentinel))
	if i < 0 || !strings.Contains(cmp.Or(cfg.Name, defaultName), "{n}") {
		return ""
	}
	return name[:i]
}

// Next is the {n} of the issue's next branch: one more than the highest
// found in its folder among the branch names and the subjects of merge
// commits (where a merged branch's name stays once it is deleted). A
// deleted branch never merged frees its number.
func Next(folder string, names, subjects []string) int {
	if folder == "" {
		return 1
	}
	at := regexp.MustCompile(`(?:^|[^A-Za-z0-9._-])` + regexp.QuoteMeta(folder) + `(\d+)(?:-|$|[^A-Za-z0-9])`)
	high := 0
	see := func(s string) {
		for _, m := range at.FindAllStringSubmatch(s, -1) {
			if n, err := strconv.Atoi(m[1]); err == nil && n > high && n < nSentinel {
				high = n
			}
		}
	}
	for _, name := range names {
		see(name)
	}
	for _, s := range subjects {
		see(s)
	}
	return high + 1
}

// Valid tells whether name is a branch name git takes as it is, and a
// word a command line takes as is: refSafe changes nothing in it.
func Valid(name string) bool {
	return name != "" && refSafe(name) == name
}

// kindOf is the issue's kind (cfg.Kind) as its branch says it, through
// cfg.Kinds.
func kindOf(cfg config.Branches, labels []string) string {
	k := cfg.Kind(labels)
	return cmp.Or(cfg.Kinds[k], k)
}

func number(is store.Issue) string {
	return cmp.Or(strings.ToLower(is.Number), is.BareNumber(), is.ID[:min(7, len(is.ID))])
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

// Names reports whether a branch's name names the issue: its number
// (BareNumber) as the name's first number, with or without the project's
// prefix (fix/vm-62-…, fix/62-…, 2518-…), or its git-bug id's
// first 7 characters or more as a word (abc1234-…).
// A folder named by an id (abc1234/1-…) holds no number: its {n} names
// no issue.
// ponytail: the first number only, so a number in a slug (#12's
// fix/12-upgrade-node-24) names no other issue; a template putting a
// number before {number} (a year) links nothing
func Names(branch string, is store.Issue) bool {
	first := true
	for _, w := range strings.FieldsFunc(branch, func(r rune) bool { return !isAlnum(r) }) {
		if len(w) >= 7 && strings.HasPrefix(is.ID, w) {
			return true
		}
		if first && len(w) >= 7 && isHexID(w) {
			first = false // an id's folder: the {n} after it is no issue's number
			continue
		}
		if first && strings.Trim(w, "0123456789") == "" {
			first = false
			if is.HasNumber(w) {
				return true
			}
		}
	}
	return false
}

// isHexID: a word as a git-bug id's prefix is written, hex digits with a
// letter among them (a number alone is a number).
func isHexID(w string) bool {
	return strings.Trim(w, "0123456789abcdef") == "" && strings.Trim(w, "0123456789") != ""
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
func Suggest(b *store.Backend, cfg config.Branches, is store.Issue) (Suggestion, error) {
	all, err := b.Branches()
	if err != nil {
		return Suggestion{}, err
	}
	has := func(r, name string) bool {
		return slices.ContainsFunc(all, func(br store.Branch) bool { return br.Remote == r && br.Name == name })
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
	n := 1
	if folder := Folder(cfg, is); folder != "" {
		names := make([]string, 0, len(all))
		for _, br := range all {
			names = append(names, br.Name)
		}
		n = Next(folder, names, b.MergeSubjects(start, mergesRead))
	}
	name := NameAt(cfg, is, n)
	return Suggestion{Suggested: name, Base: base, Start: start, Command: "git switch -c " + name + " " + start}, nil
}

// mergesRead is how many of the base's last merge commits Next reads.
const mergesRead = 500

// Branch is a branch naming the issue, against the base.
type Branch struct {
	Name   string `json:"name"`
	Remote string `json:"remote,omitempty"`
	Ahead  int    `json:"ahead"`  // its commits the base hasn't
	Behind int    `json:"behind"` // the base's commits it hasn't
	Merged bool   `json:"merged"` // its head is in the base
	// InBase: its changes are in the base, merged or squashed or rebased in
	InBase     bool         `json:"in_base"`
	LastCommit store.Commit `json:"last_commit"`
}

// Linked lists the branches naming the issue, local then the remotes',
// against start (Suggestion.Start); without start, all at zero.
// ponytail: a branch's whole history, read per branch: fine for the few
// naming an issue; git's commit-graph if a big repository makes it slow
func Linked(b *store.Backend, is store.Issue, start string) ([]Branch, error) {
	all, err := b.Branches()
	if err != nil {
		return nil, err
	}
	at := func(br store.Branch) string { return strings.TrimPrefix(br.Remote+"/"+br.Name, "/") }
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
