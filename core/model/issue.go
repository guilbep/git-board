// Package model is the issue domain's read model: the issue domain as
// every surface reads it. An issue as lists show it (Issue), with its
// whole history (Thread), the people and forges in it. Plain data:
// internal/store folds the core's operations into it; the CLI, the TUI,
// the web API, the filters and the inbox read it. The JSON tags are a
// stable format (docs/guide/cli.md, JSON output).
package model

import (
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
)

// Issue is an issue as every surface lists it, and as fields and filters
// read it: the flat projection of its operations, its people and its forges.
type Issue struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Status string   `json:"status"` // "open" or "closed"
	Labels []string `json:"labels"`
	// Inherited are the labels it takes from its project ([roles]) for the
	// roles it doesn't set itself; the board's listing sets them.
	Inherited []string `json:"inherited,omitempty"`
	GithubURL string   `json:"github_url,omitempty"` // "" if not linked to a GitHub issue
	Forges    []Forge  `json:"forges,omitempty"`     // where it is on forges
	// Shown is the title as the board shows it (ShownTitle).
	Shown string `json:"shown_title"`
	// Number is its project number, CO2-3075: "" until the counter hands it
	// one, and while the project doesn't number its issues (numbers.go).
	Number string `json:"number,omitempty"`
	// Counted tells the project numbers its issues: a bare number (3075,
	// fix/3075-…) then means a project number; #3075 is still a forge's.
	Counted bool `json:"counted,omitempty"`
	// UpdatedAt is its last change, for "changed since your last visit".
	UpdatedAt time.Time `json:"updated_at"`
	// CreatedAt is when it was opened: it breaks a tie between issues one
	// sync stamped at once.
	CreatedAt time.Time `json:"created_at"`
	// ClosedAt is when it was last closed, while it is: zero when open.
	ClosedAt time.Time `json:"closed_at,omitzero"`
	// Comments is how many comments it has, for comments:.
	Comments int `json:"comments"`
	// Body and CommentBodies are its texts, for free text without the
	// index (an issue as it was: ls --at).
	Body          string   `json:"-"`
	CommentBodies []string `json:"-"`
	// AuthorLogins are who opened it, for author:: name, login, forge
	// logins.
	AuthorLogins []string `json:"-"`
	// CommenterLogins are the logins of who commented, the author
	// excepted, for involves:.
	CommenterLogins []string `json:"-"`
	// AuthorID and CommenterIDs are the same people by identity: you are
	// your identity, with or without a login.
	AuthorID     string   `json:"-"`
	CommenterIDs []string `json:"-"`
}

// Forge is where an issue is on a forge.
type Forge struct {
	Name   string `json:"name"` // GitHub, GitLab, or the host
	URL    string `json:"url"`
	Number string `json:"number,omitempty"`
}

// BareNumber is the number a bare mention names it by (3075, fix/3075-…):
// its project number once the project numbers its issues, else its first
// forge number; "" for none. With HasNumber and HasForgeNumber, the one
// place a number is told to name an issue, for search, branch names and
// titles.
func (is Issue) BareNumber() string {
	if is.Counted {
		return is.Number[strings.LastIndexByte(is.Number, '-')+1:]
	}
	for _, f := range is.Forges {
		if f.Number != "" {
			return f.Number
		}
	}
	return ""
}

// HasNumber tells whether a bare number names the issue: its project
// number once the project numbers its issues, else any of its forge
// numbers.
func (is Issue) HasNumber(n string) bool {
	if is.Counted {
		return n != "" && is.BareNumber() == n
	}
	return is.HasForgeNumber(n)
}

// HasForgeNumber tells whether #n, a forge's own way to name an issue,
// names it.
func (is Issue) HasForgeNumber(n string) bool {
	return n != "" && slices.ContainsFunc(is.Forges, func(f Forge) bool { return f.Number == n })
}

// AllLabels are its own labels and those it inherits: what filters,
// groups and facets see. Edits change its own labels only.
func (is Issue) AllLabels() []string { return append(slices.Clip(is.Labels), is.Inherited...) }

// Headline is its shown title after its project number, as lists show it:
// "CO2-3075 Restarting Postgres empties the bouncer pool"; its title
// before it has one.
func (is Issue) Headline() string {
	if is.Number == "" {
		return is.Title
	}
	return is.Number + " " + is.ShownTitle()
}

var ownNumber = regexp.MustCompile(`\s*\(#(\d+)\)\s*$`) // (#2518)

// ShownTitle is the title without a trailing (#2518) that is one of the
// issue's own forge numbers: the forge links show it already. The title
// itself keeps it, and editing starts from the title.
func (is Issue) ShownTitle() string {
	if !strings.HasSuffix(strings.TrimRightFunc(is.Title, unicode.IsSpace), ")") { // most titles: no regexp
		return is.Title
	}
	if m := ownNumber.FindStringSubmatchIndex(is.Title); m != nil && is.HasForgeNumber(is.Title[m[2]:m[3]]) {
		return is.Title[:m[0]]
	}
	return is.Title
}
