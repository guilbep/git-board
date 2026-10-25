package gitbugio

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/core/people"
	"github.com/guilbep/git-board/core/syncer"
)

// ErrNoSuchComment is returned when a comment id isn't one of the issue's.
var ErrNoSuchComment = errors.New("no such comment on this issue")

// ErrChangedMeanwhile is returned when a comment changed since the edit
// started: saving would silently replace someone else's text.
var ErrChangedMeanwhile = errors.New("someone changed this comment meanwhile")

// Person is a person, as the page shows them.
type Person struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Login     string `json:"login,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// Forge is where an issue is on a forge.
type Forge struct {
	Name   string `json:"name"` // GitHub, GitLab, or the host
	URL    string `json:"url"`
	Number string `json:"number,omitempty"`
	// Gone is never set since unlink went (M5); the web app still reads it.
	Gone string `json:"gone,omitempty"`
}

// Version is one text of an edited comment, oldest first.
type Version struct {
	Author Person    `json:"author"`
	At     time.Time `json:"at"`
	Body   string    `json:"body"`
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

	Added   []string `json:"added,omitempty"` // labels
	Removed []string `json:"removed,omitempty"`
	Status  string   `json:"status,omitempty"` // status: open or closed
	Title   string   `json:"title,omitempty"`  // title: the new one
	Was     string   `json:"was,omitempty"`    // title: the old one

	Review *Review `json:"review,omitempty"` // comment: a line comment, a reply or a review
}

// Thread is an issue with its whole history.
type Thread struct {
	Issue
	Author       Person    `json:"author"`
	EditedAt     time.Time `json:"edited_at"`
	Participants []Person  `json:"participants"`
	Events       []Event   `json:"events"`
}

// Thread returns an issue and its history, by id prefix.
func (b *Backend) Thread(idPrefix string) (Thread, error) {
	th, err := b.Threads([]string{idPrefix})
	if err != nil {
		return Thread{}, err
	}
	return th[0], nil
}

// Threads is Thread for each id prefix, reading once what they all need.
func (b *Backend) Threads(idPrefixes []string) ([]Thread, error) {
	v, err := b.view()
	if err != nil {
		return nil, err
	}
	res := make([]Thread, 0, len(idPrefixes))
	for _, p := range idPrefixes {
		is, l, err := b.issue(p)
		if err != nil {
			return nil, err
		}
		res = append(res, b.thread(is, l, v))
	}
	return res, nil
}

// thread replays an issue's log into its history, as issue.Apply folds it.
func (b *Backend) thread(is *issue.Issue, l *oplog.Log, v view) Thread {
	who := func(id string) Person { return personOf(id, v.persons[id]) }
	th := Thread{Issue: b.issueFrom(is, v), Author: who(is.Author), EditedAt: time.Unix(is.Updated, 0)}
	th.Participants = append(th.Participants, th.Author)
	for _, c := range is.Comments {
		if !slices.ContainsFunc(th.Participants, func(p Person) bool { return p.ID == c.Author }) {
			th.Participants = append(th.Participants, who(c.Author))
		}
	}
	texts := map[string]int{}         // a text's operation id -> its event
	origins := map[oplog.Origin]int{} // a comment's origin -> its event
	labels := map[string]bool{}
	title := ""
	for _, e := range l.Entries {
		var d struct {
			Title, Body, Target string
			Status              issue.Status
			Add, Remove         []string
			Meta                map[string]string
		}
		_ = json.Unmarshal(e.Data, &d) // Apply read it already
		ev := Event{ID: e.ID, Author: who(e.Author), At: time.Unix(e.Time, 0)}
		switch e.Kind {
		case "create", "comment":
			if e.Origin != nil {
				if i, dup := origins[*e.Origin]; dup { // the same comment, pulled again
					texts[e.ID] = i
					continue
				}
				origins[*e.Origin] = len(th.Events)
			}
			ev.Kind, ev.Body, ev.Meta = "comment", d.Body, maps.Clone(d.Meta)
			if e.Kind == "create" {
				ev.Kind, title = "created", d.Title
			}
			ev.History = []Version{{Author: ev.Author, At: ev.At, Body: d.Body}}
			texts[e.ID] = len(th.Events)
		case "edit-text":
			if i, ok := texts[d.Target]; ok {
				t := &th.Events[i]
				t.Body = d.Body
				t.History = append(t.History, Version{Author: ev.Author, At: ev.At, Body: d.Body})
			}
			continue
		case "set-meta":
			if i, ok := texts[d.Target]; ok {
				t := &th.Events[i]
				if t.Meta == nil {
					t.Meta = map[string]string{}
				}
				maps.Copy(t.Meta, d.Meta)
			}
			continue
		case "set-title":
			ev.Kind, ev.Title, ev.Was, title = "title", d.Title, title, d.Title
		case "set-status":
			ev.Kind, ev.Status = "status", string(d.Status)
		case "change-labels":
			ev.Added, ev.Removed = applyLabels(labels, d.Add, d.Remove)
			if len(ev.Added) == 0 && len(ev.Removed) == 0 {
				continue
			}
			ev.Kind, ev.Added, ev.Removed = "labels", b.shown(ev.Added), b.shown(ev.Removed)
		default:
			continue
		}
		th.Events = append(th.Events, ev)
	}
	for i := range th.Events {
		e := &th.Events[i]
		if e.Kind != "created" && e.Kind != "comment" {
			continue
		}
		e.Edited = len(e.History) > 1
		if !e.Edited {
			e.History = nil
		}
		e.Review = reviewOf(e.Meta)
		if len(e.Meta) == 0 {
			e.Meta = nil
		}
	}
	return th
}

// forgesOf is where an issue is, from its links: each forge by its URL.
func forgesOf(links []syncer.Link) []Forge {
	var res []Forge
	for _, l := range links {
		n := l.Number
		if n == "" {
			n = lastNumber(l.URL)
		}
		res = append(res, Forge{Name: forgeName(l.URL), URL: l.URL, Number: n})
	}
	return res
}

func forgeName(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return u
	}
	switch host := parsed.Hostname(); {
	case host == "github.com":
		return "GitHub"
	case strings.HasPrefix(host, "gitlab."):
		return "GitLab"
	default:
		return host
	}
}

// lastNumber is the issue number ending a forge URL, if it ends with one.
func lastNumber(u string) string {
	last := path.Base(u)
	if last == "" || strings.Trim(last, "0123456789") != "" {
		return ""
	}
	return last
}

// personOf is a person as the page shows them: their first forge login.
// One not fetched yet shows by id.
func personOf(id string, p *people.Person) Person {
	if p == nil {
		return Person{ID: id, Name: id[:min(7, len(id))]}
	}
	res := Person{ID: p.ID, Name: p.Name}
	if ls := logins(p); len(ls) > 0 {
		res.Login = ls[0]
	}
	return res
}

// Me returns who writes from git-board: the person board.me names. false
// when none is chosen yet.
func (b *Backend) Me() (Person, bool, error) {
	id, err := b.me()
	if errors.Is(err, ErrNoMe) {
		return Person{}, false, nil
	}
	if err != nil {
		return Person{}, false, err
	}
	persons, err := b.allPersons()
	if err != nil {
		return Person{}, false, err
	}
	p, ok := persons[id]
	if !ok {
		return Person{}, false, fmt.Errorf("%w: you, person %s", ErrNotFound, id)
	}
	return personOf(id, p), true, nil
}

// NewIssue creates an issue as you, with labels, and returns its id.
func (b *Backend) NewIssue(title, body string, labels []string) (string, error) {
	me, err := b.me()
	if err != nil {
		return "", err
	}
	ops := []oplog.Op{issue.NewCreate(me, title, body)}
	if labels, _ = b.stored(labels, nil, nil); len(labels) > 0 {
		ops = append(ops, issue.ChangeLabels(me, labels, nil))
	}
	is, err := issue.Create(ctx, b.s, ops...)
	if err != nil {
		return "", fmt.Errorf("creating the issue: %w", err)
	}
	return is.ID, nil
}

// AddComment comments on an issue as you, and returns the comment's id.
func (b *Backend) AddComment(idPrefix, body string) (string, error) {
	return b.AddCommentMeta(idPrefix, body, nil)
}

// AddCommentMeta is AddComment with metadata: what the cycle's sign-offs,
// approvals and runs are, and a review comment's anchor.
func (b *Backend) AddCommentMeta(idPrefix, body string, meta map[string]string) (string, error) {
	is, err := b.do(idPrefix, func(me string, _ *issue.Issue, _ *oplog.Log) ([]oplog.Op, error) {
		if len(meta) == 0 {
			return []oplog.Op{issue.NewComment(me, body)}, nil
		}
		return []oplog.Op{issue.NewCommentMeta(me, body, meta)}, nil
	})
	if err != nil {
		return "", err
	}
	return is.Comments[len(is.Comments)-1].ID, nil
}

var textID = regexp.MustCompile(`^[0-9a-f]{40}:[0-9]+$`)

// text finds one of an issue's texts by id: its body (the created event's
// id) or a comment, as Apply knows them: a duplicate's id names its
// comment.
func text(is *issue.Issue, l *oplog.Log, id string) (target, body string, meta map[string]string, err error) {
	if !textID.MatchString(id) {
		return "", "", nil, ErrNoSuchComment
	}
	if id == is.BodyOp {
		return id, is.Body, is.Meta, nil
	}
	if i := slices.IndexFunc(is.Comments, func(c issue.Comment) bool { return c.ID == id }); i >= 0 {
		return id, is.Comments[i].Body, is.Comments[i].Meta, nil
	}
	// another id of a comment: a copy of it pulled again
	for _, e := range l.Entries {
		if e.ID == id && e.Kind == "comment" && e.Origin != nil {
			for _, c := range is.Comments {
				if c.Origin != nil && *c.Origin == *e.Origin {
					return c.ID, c.Body, c.Meta, nil
				}
			}
		}
	}
	return "", "", nil, ErrNoSuchComment
}

// EditComment replaces the text of one of an issue's comments, or of its
// description (the created event's id). base is the text the edit started
// from: if the comment holds another one now, the edit is refused ("" skips
// the check).
// ponytail: the check reads, then the write lands; two edits in that
// instant both land, the later winning
func (b *Backend) EditComment(idPrefix, commentID, body, base string) error {
	_, err := b.do(idPrefix, func(me string, is *issue.Issue, l *oplog.Log) ([]oplog.Op, error) {
		target, now, _, err := text(is, l, commentID)
		if err != nil {
			return nil, err
		}
		if base != "" && now != base {
			return nil, ErrChangedMeanwhile
		}
		if now == body {
			return nil, nil
		}
		return []oplog.Op{issue.EditText(me, target, body)}, nil
	})
	return err
}

// SetTitle renames an issue.
func (b *Backend) SetTitle(idPrefix, title string) error {
	_, err := b.do(idPrefix, func(me string, is *issue.Issue, _ *oplog.Log) ([]oplog.Op, error) {
		if is.Title == title {
			return nil, nil
		}
		return []oplog.Op{issue.SetTitle(me, title)}, nil
	})
	return err
}

// SetStatus opens or closes an issue.
func (b *Backend) SetStatus(idPrefix, status string) error {
	if status != "open" && status != "closed" {
		return fmt.Errorf("status %q: want open or closed", status)
	}
	_, err := b.do(idPrefix, func(me string, is *issue.Issue, _ *oplog.Log) ([]oplog.Op, error) {
		if string(is.Status) == status {
			return nil, nil
		}
		return []oplog.Op{issue.SetStatus(me, issue.Status(status))}, nil
	})
	return err
}
