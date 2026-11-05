// Package branches is an issue's code: the branch it suggests creating,
// named from [branches], and the branches whose name names it. The issue
// is the unit of work; its code hangs off it by name, each branch in the
// issue's folder (fix/2518/1-…, fix/2518/2-…). Read-only: the server
// creates the branch (code.Repo.CreateBranch), from the name made here.
package branches

import (
	"cmp"
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/tools/code"
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
func Name(cfg issues.Branches, is issues.Issue) string {
	return NameAt(cfg, is, 1)
}

// NameAt is the branch an issue suggests: cfg.Name with {kind} (the
// issue's kind through cfg.Kinds; without one, "{kind}/" is dropped),
// {number} (its project number with its prefix in lower case, vm-62: a
// bare 62 is a forge's number to a forge; else its first forge number
// before the project numbers its issues, else its git-bug id's first 7
// characters), {n} (the branch's place in the issue's folder) and {slug}
// (its title), made a branch name git takes.
func NameAt(cfg issues.Branches, is issues.Issue, n int) string {
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
func Folder(cfg issues.Branches, is issues.Issue) string {
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
func kindOf(cfg issues.Branches, labels []string) string {
	k := Kind(cfg, labels)
	return cmp.Or(cfg.Kinds[k], k)
}

func number(is issues.Issue) string {
	return cmp.Or(strings.ToLower(is.Number), is.BareNumber(), string(is.ID[:min(7, len(is.ID))]))
}

var (
	titlePrefix = regexp.MustCompile(`^\s*\[[^\]]*\][\s:-]*(\([^)]*\))?`) // [BUG](Equipment), [Bug]: (Planner)
	titleNumber = regexp.MustCompile(`\(#\d+\)\s*$`)                      // (#2518)
)

// Slug is a title as a branch says it: lower-case ASCII words between
// dashes, without a leading [PREFIX](Scope) or a trailing (#1234), cut
// between words at about 50 characters.
func Slug(title string) string {
	s := slug(titleNumber.ReplaceAllString(titlePrefix.ReplaceAllString(title, ""), ""))
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
func Names(branch string, is issues.Issue) bool {
	first := true
	for _, w := range strings.FieldsFunc(branch, func(r rune) bool { return !isAlnum(r) }) {
		if len(w) >= 7 && strings.HasPrefix(string(is.ID), w) {
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
func Suggest(ctx context.Context, b code.Repo, cfg issues.Branches, is issues.Issue) (Suggestion, error) {
	all, err := b.Branches(ctx)
	if err != nil {
		return Suggestion{}, err
	}
	base, start := baseOf(ctx, b, cfg, all)
	n := 1
	if folder := Folder(cfg, is); folder != "" {
		names := make([]string, 0, len(all))
		for _, br := range all {
			names = append(names, br.Name)
		}
		n = Next(folder, names, b.MergeSubjects(ctx, start, mergesRead))
	}
	name := NameAt(cfg, is, n)
	return Suggestion{Suggested: name, Base: base, Start: start, Command: "git switch -c " + shellWord(name) + " " + shellWord(start)}, nil
}

// Start is where an issue's branch starts, Suggest's Start without the
// name: the same for every issue, what the cycle reads their branches
// against.
func Start(ctx context.Context, b code.Repo, cfg issues.Branches) (string, error) {
	all, err := b.Branches(ctx)
	if err != nil {
		return "", err
	}
	_, start := baseOf(ctx, b, cfg, all)
	return start, nil
}

// baseOf is the base and where a branch starts on it.
func baseOf(ctx context.Context, b code.Repo, cfg issues.Branches, all []code.Branch) (base, start string) {
	has := func(r, name string) bool {
		return slices.ContainsFunc(all, func(br code.Branch) bool { return br.Remote == r && br.Name == name })
	}
	base = cmp.Or(cfg.Base, b.RemoteHead(ctx, remote))
	if base == "" {
		base = "main"
		for _, c := range []string{"main", "master", "trunk"} {
			if has("", c) || has(remote, c) {
				base = c
				break
			}
		}
	}
	start = remote + "/" + base
	if !has(remote, base) && has("", base) {
		start = base
	}
	return base, start
}

// shellWord is s as one shell word: as is when it's plain, else single
// quoted. The base comes from the committed [branches] base, and the
// command is copied into a shell.
func shellWord(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._/@+-=:,") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
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
	InBase     bool        `json:"in_base"`
	LastCommit code.Commit `json:"last_commit"`
}

// Ancestry is commits' ancestors, each commit's read once: the Work page
// lists many issues' branches against one base, whose whole history was
// read again for each issue. One is a request's: a commit's ancestors
// don't change, but a later request's branches may have moved. Safe for
// concurrent use; the sets it gives are shared, to read only.
type Ancestry struct {
	b     code.Repo
	mu    sync.Mutex
	sets  map[string]map[string]bool
	reads int // histories read, for the tests
}

// NewAncestry is an Ancestry of b's commits, empty.
func NewAncestry(b code.Repo) *Ancestry {
	return &Ancestry{b: b, sets: map[string]map[string]bool{}}
}

// Repo is the repository whose commits it reads.
func (a *Ancestry) Repo() code.Repo { return a.b }

// Of is the commit hash and every commit before it.
func (a *Ancestry) Of(ctx context.Context, hash string) (map[string]bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if set, ok := a.sets[hash]; ok {
		return set, nil
	}
	set, err := a.b.Ancestors(ctx, hash)
	if err != nil {
		return nil, err
	}
	a.sets[hash], a.reads = set, a.reads+1
	return set, nil
}

// Linked lists the branches naming the issue, local then the remotes',
// against start (Suggestion.Start); without start, all at zero.
// ponytail: a branch's whole history, read per branch: fine for the few
// naming an issue; git's commit-graph if a big repository makes it slow
func Linked(ctx context.Context, b code.Repo, is issues.Issue, start string) ([]Branch, error) {
	return LinkedIn(ctx, NewAncestry(b), is, start)
}

// LinkedIn is Linked, reading the histories through anc: one Ancestry for
// many issues reads their base's once.
func LinkedIn(ctx context.Context, anc *Ancestry, is issues.Issue, start string) ([]Branch, error) {
	b := anc.b
	all, err := b.Branches(ctx)
	if err != nil {
		return nil, err
	}
	at := func(br code.Branch) string { return strings.TrimPrefix(br.Remote+"/"+br.Name, "/") }
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
				if base, err = anc.Of(ctx, startHash); err != nil {
					return nil, err
				}
			}
		}
		last, err := b.ReadCommit(ctx, br.Hash)
		if err != nil {
			return nil, err
		}
		l := Branch{Name: br.Name, Remote: br.Remote, Merged: base[br.Hash], LastCommit: last}
		if len(base) > 0 {
			mine, err := anc.Of(ctx, br.Hash)
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
		l.InBase = l.Merged || l.Ahead > 0 && startHash != "" && b.ChangesIn(ctx, startHash, br.Hash)
		res = append(res, l)
	}
	return res, nil
}

// Kind is an issue's kind as [branches] reads it: a kind:<k> label, else
// the first of its labels, in order, that [branches] kinds names.
func Kind(cfg issues.Branches, labels []string) string {
	for _, l := range labels {
		if v, ok := strings.CutPrefix(l, "kind:"); ok {
			return v
		}
	}
	for _, l := range slices.Sorted(slices.Values(labels)) {
		if _, ok := cfg.Kinds[l]; ok {
			return l
		}
	}
	return ""
}

var slugNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// slug is a value in lower-case ASCII words between dashes, its accents
// dropped.
func slug(value string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(strings.TrimSpace(value))) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return strings.Trim(slugNonAlnum.ReplaceAllString(b.String(), "-"), "-")
}
