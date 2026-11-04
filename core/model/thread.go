package model

import "time"

// Thread is an issue with its whole history.
type Thread struct {
	Issue
	Author       Person    `json:"author"`
	EditedAt     time.Time `json:"edited_at"`
	Participants []Person  `json:"participants"`
	Events       []Event   `json:"events"`
}

// Person is a person, as the page shows them.
type Person struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Login     string `json:"login,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// Event is one item of an issue's history: created (with the description),
// comment, labels, status or title.
type Event struct {
	Kind   string    `json:"kind"`
	ID     string    `json:"id"` // created, comment: what EditComment takes
	Author Person    `json:"author"`
	At     time.Time `json:"at"`

	Body    string    `json:"body,omitempty"` // created, comment: Markdown
	HTML    string    `json:"html,omitempty"` // filled by the server
	Edited  bool      `json:"edited,omitempty"`
	History []Version `json:"history,omitempty"`
	// Pending are the forges the issue is on that this change hasn't
	// reached yet: the next forge sync takes it there. EditPending, for a
	// comment already there: its latest edit hasn't. EditWas, with
	// EditPending, is the text the forges have. All three need the sync's
	// records: empty until M5's second phase.
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
	ID     string    `json:"id"` // the text's own, then each edit-text's
	Author Person    `json:"author"`
	At     time.Time `json:"at"`
	Body   string    `json:"body"`
	HTML   string    `json:"html,omitempty"` // rendered by the server, the description's
}

// MetaSet is one set-meta operation on a text, oldest first.
type MetaSet struct {
	ID     string            `json:"id"`
	Author Person            `json:"author"`
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
