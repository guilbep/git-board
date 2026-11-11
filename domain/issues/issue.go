package issues

import (
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/domain/people"
	"github.com/guilbep/git-board/kernel"
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
	// Author is who opened it, by name: the table's Author column.
	Author string `json:"opened_by,omitempty"`
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
	// Version is its entity's version, as read: what a write may expect
	// it at (kernel.Expect). Not in the CLI's JSON: the API adds it.
	Version kernel.Version `json:"-"`
}

// Forge is where an issue is on a forge.
type Forge struct {
	Name   string `json:"name"` // GitHub, GitLab, or the host
	URL    string `json:"url"`
	Number string `json:"number,omitempty"`
	// Sync is the sync entity the link is in: sync.StateID of the forge
	// project's key. It tells two projects of one host apart, which the
	// URL doesn't for a GitLab table named by its numeric id.
	Sync kernel.ID `json:"-"`
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

// ShownTitle is the title without a trailing (#2518) that is one of the
// issue's own forge numbers: the forge links show it already. The title
// itself keeps it, and editing starts from the title. It reads as v1's
// regexp `\s*\(#(\d+)\)\s*$` did, by hand: every list asks it of every
// issue.
func (is Issue) ShownTitle() string {
	if len(is.Forges) == 0 {
		return is.Title
	}
	t := strings.TrimRight(is.Title, asciiSpace)
	if !strings.HasSuffix(t, ")") {
		return is.Title
	}
	i := strings.LastIndex(t, "(#")
	n := t[min(i+2, len(t)-1) : len(t)-1]
	if i < 0 || n == "" || strings.Trim(n, "0123456789") != "" || !is.HasForgeNumber(n) {
		return is.Title
	}
	return strings.TrimRight(t[:i], asciiSpace)
}

// asciiSpace is what a regexp's \s is.
const asciiSpace = "\t\n\f\r "

// Thread is an issue with its whole history.
type Thread struct {
	Issue
	Author       people.Who   `json:"author"`
	EditedAt     time.Time    `json:"edited_at"`
	Participants []people.Who `json:"participants"`
	Events       []Event      `json:"events"`
}

// Event is one item of an issue's history: created (with the description),
// comment, labels, status or title.
type Event struct {
	Kind   string     `json:"kind"`
	ID     string     `json:"id"` // created, comment: what EditComment takes
	Author people.Who `json:"author"`
	At     time.Time  `json:"at"`

	Body    string    `json:"body,omitempty"` // created, comment: Markdown
	HTML    string    `json:"html,omitempty"` // filled by the server
	Edited  bool      `json:"edited,omitempty"`
	History []Version `json:"history,omitempty"`
	// Pending are the forges the issue is on that this change hasn't
	// reached yet: the next forge sync takes it there. EditPending, for a
	// comment already there: its latest edit hasn't. EditWas, with
	// EditPending, is the text the forges have. All three are read from
	// the sync's records.
	Pending     []string `json:"pending,omitempty"`
	EditPending []string `json:"edit_pending,omitempty"`
	EditWas     string   `json:"edit_was,omitempty"`
	// Meta is a comment's metadata: what the cycle's sign-offs, approvals
	// and runs are, and a review comment's anchor.
	Meta map[string]string `json:"meta,omitempty"`
	// MetaHistory is each set-meta on the text, merged into Meta: who
	// resolved a thread, when, and that "" reopened it.
	MetaHistory []MetaSet `json:"meta_history,omitempty"`

	Added   []string `json:"added,omitempty"` // labels
	Removed []string `json:"removed,omitempty"`
	Status  string   `json:"status,omitempty"` // status: open or closed
	Title   string   `json:"title,omitempty"`  // title: the new one
	Was     string   `json:"was,omitempty"`    // title: the old one

	Review *Review `json:"review,omitempty"` // comment: a line comment, a reply or a review
}

// Version is one text of an edited comment, oldest first.
type Version struct {
	ID     string     `json:"id"` // the text's own, then each edit-text's
	Author people.Who `json:"author"`
	At     time.Time  `json:"at"`
	Body   string     `json:"body"`
	HTML   string     `json:"html,omitempty"` // rendered by the server, the description's
}

// MetaSet is one set-meta operation on a text, oldest first.
type MetaSet struct {
	ID     string            `json:"id"`
	Author people.Who        `json:"author"`
	At     time.Time         `json:"at"`
	Meta   map[string]string `json:"meta"`
}

// Review is a comment's review metadata, its review:* keys
// (docs/prd/work-unit.md, M4): a line comment (path, line, side, commit), a
// reply to one (reply-to and its anchor), a review (state). They stay in
// git-board: mirroring them to pull requests is out of version 1, and a
// pr is only what the fork's sync left. nil for a plain comment.
type Review struct {
	Path     string `json:"path,omitempty"`
	Line     int    `json:"line,omitempty"`
	Side     string `json:"side,omitempty"`   // old | new
	Commit   string `json:"commit,omitempty"` // the full SHA the comment was written on
	PR       string `json:"pr,omitempty"`     // github.com/o/r#26
	ReplyTo  string `json:"reply_to,omitempty"`
	State    string `json:"state,omitempty"`    // APPROVE | REQUEST_CHANGES | COMMENT
	Resolved bool   `json:"resolved,omitempty"` // the last review:resolved set on it
	// filled by the server: the issue's branch whose head has Commit, and,
	// from that head, whether a change since covers the line, where the
	// line is at head, and its text at Commit
	Branch   string `json:"branch,omitempty"`
	Outdated bool   `json:"outdated,omitempty"`
	HeadLine int    `json:"head_line,omitempty"`
	Text     string `json:"text,omitempty"`
}

// mentionRe is an @login (web/src/richtext.ts has it too) not inside an
// email, a path or code, ending on a letter, a digit or _. Its second
// group is the mention, @ included.
var mentionRe = regexp.MustCompile(`(^|[^\w.\-/@` + "`" + `])(@[A-Za-z0-9_](?:[A-Za-z0-9_.\-]*[A-Za-z0-9_])?)`)

// Mentions reports whether text mentions login (@login), ignoring case.
func Mentions(text, login string) bool {
	if login == "" {
		return false
	}
	for _, m := range mentionRe.FindAllStringSubmatch(text, -1) {
		if strings.EqualFold(m[2][1:], login) {
			return true
		}
	}
	return false
}
