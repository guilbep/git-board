package gitbugio

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/git-bug/git-bug/cache"
	bugent "github.com/git-bug/git-bug/entities/bug"
	"github.com/git-bug/git-bug/entities/common"
	"github.com/git-bug/git-bug/entities/identity"
	"github.com/git-bug/git-bug/entity"
	"github.com/git-bug/git-bug/entity/dag"
	"github.com/git-bug/git-bug/util/timestamp"
)

// ErrNoSuchComment is returned when a comment id isn't one of the issue's.
var ErrNoSuchComment = errors.New("no such comment on this issue")

// ErrChangedMeanwhile is returned when a comment changed since the edit
// started: saving would silently replace someone else's text.
var ErrChangedMeanwhile = errors.New("someone changed this comment meanwhile")

// Person is a git-bug identity, as the page shows it.
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
	// Pending are the forges the issue is on that this text hasn't reached
	// yet: the next forge sync posts it.
	Pending []string `json:"pending,omitempty"`

	Added   []string `json:"added,omitempty"` // labels
	Removed []string `json:"removed,omitempty"`
	Status  string   `json:"status,omitempty"` // status: open or closed
	Title   string   `json:"title,omitempty"`  // title: the new one
	Was     string   `json:"was,omitempty"`    // title: the old one
}

// Thread is an issue with its whole history.
type Thread struct {
	Issue
	Author       Person    `json:"author"`
	CreatedAt    time.Time `json:"created_at"`
	EditedAt     time.Time `json:"edited_at"`
	Forges       []Forge   `json:"forges"`
	Participants []Person  `json:"participants"`
	Events       []Event   `json:"events"`
}

// Thread returns an issue and its history, by id prefix.
func (b *Backend) Thread(idPrefix string) (Thread, error) {
	bug, err := b.c.Bugs().ResolvePrefix(idPrefix)
	if err != nil {
		return Thread{}, fmt.Errorf("resolving %s: %w", idPrefix, err)
	}
	snap := bug.Snapshot()
	issue, err := b.GetIssue(bug.Id().String())
	if err != nil {
		return Thread{}, err
	}
	createMeta := snap.Operations[0].AllMetadata()
	forges, remotes := forgesOf(createMeta)

	ops := map[entity.Id]dag.Operation{}
	for _, op := range snap.Operations {
		ops[op.Id()] = op
	}
	th := Thread{
		Issue:     issue,
		Author:    person(snap.Author),
		CreatedAt: snap.CreateTime,
		EditedAt:  snap.EditTime(),
		Forges:    forges,
	}
	for _, p := range snap.Participants {
		th.Participants = append(th.Participants, person(p))
	}
	for _, item := range snap.Timeline {
		th.Events = append(th.Events, event(snap, item, ops, remotes))
	}
	return th, nil
}

func event(snap *bugent.Snapshot, item bugent.TimelineItem, ops map[entity.Id]dag.Operation, remotes map[string]string) Event {
	comment := func(kind string, c bugent.CommentTimelineItem) Event {
		e := Event{Kind: kind, ID: string(item.CombinedId()), Author: person(c.Author), At: at(c.CreatedAt), Body: c.Message, Edited: len(c.History) > 1}
		if e.Edited {
			for _, h := range c.History {
				e.History = append(e.History, Version{Author: person(h.Author), At: at(h.UnixTime), Body: h.Message})
			}
		}
		if found, err := snap.SearchComment(item.CombinedId()); err == nil {
			if op, ok := ops[found.TargetId()]; ok {
				for key, name := range remotes {
					if _, posted := op.GetMetadata(key + "id"); !posted {
						e.Pending = append(e.Pending, name)
					}
				}
				sort.Strings(e.Pending)
			}
		}
		return e
	}
	switch it := item.(type) {
	case *bugent.CreateTimelineItem:
		return comment("created", it.CommentTimelineItem)
	case *bugent.AddCommentTimelineItem:
		return comment("comment", it.CommentTimelineItem)
	case *bugent.LabelChangeTimelineItem:
		return Event{Kind: "labels", ID: string(it.CombinedId()), Author: person(it.Author), At: at(it.UnixTime), Added: labelStrings(it.Added), Removed: labelStrings(it.Removed)}
	case *bugent.SetStatusTimelineItem:
		status := "open"
		if it.Status == common.ClosedStatus {
			status = "closed"
		}
		return Event{Kind: "status", ID: string(it.CombinedId()), Author: person(it.Author), At: at(it.UnixTime), Status: status}
	case *bugent.SetTitleTimelineItem:
		return Event{Kind: "title", ID: string(it.CombinedId()), Author: person(it.Author), At: at(it.UnixTime), Title: it.Title, Was: it.Was}
	}
	return Event{Kind: "unknown", ID: string(item.CombinedId())}
}

// forgesOf reads where an issue is from its create operation's metadata:
// the sync engine's sync:<remote>:url (and :number), then the older
// bridges' <kind>-url. remotes maps each sync remote's key prefix
// ("sync:github.com/o/r:") to its forge's name, for the comments to say
// which forges they haven't reached.
func forgesOf(meta map[string]string) (forges []Forge, remotes map[string]string) {
	remotes = map[string]string{}
	seen := map[string]bool{}
	keys := make([]string, 0, len(meta))
	for k := range meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	add := func(u, number string) string {
		name := forgeName(u)
		if !seen[u] {
			seen[u] = true
			if number == "" {
				number = lastNumber(u)
			}
			forges = append(forges, Forge{Name: name, URL: u, Number: number})
		}
		return name
	}
	for _, k := range keys {
		if prefix, ok := strings.CutSuffix(k, ":url"); ok && strings.HasPrefix(k, "sync:") {
			name := add(meta[k], meta[prefix+":number"])
			if _, onForge := meta[prefix+":id"]; onForge {
				remotes[prefix+":"] = name
			}
		}
	}
	for _, k := range keys {
		if strings.HasSuffix(k, "-url") && !strings.Contains(k, ":") {
			add(meta[k], "")
		}
	}
	return forges, remotes
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
	case strings.HasPrefix(host, "gitea.") || strings.HasPrefix(host, "codeberg."):
		return "Gitea"
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

func person(i identity.Interface) Person {
	if i == nil {
		return Person{}
	}
	return Person{ID: i.Id().String(), Name: i.DisplayName(), Login: i.Login(), AvatarURL: i.AvatarUrl()}
}

func at(t timestamp.Timestamp) time.Time { return time.Unix(int64(t), 0) }

func labelStrings(labels []common.Label) []string {
	res := make([]string, len(labels))
	for i, l := range labels {
		res[i] = l.String()
	}
	return res
}

// Me returns who writes from git-board: the repository's git-bug user
// identity. false when none is set yet.
func (b *Backend) Me() (Person, bool, error) {
	set, err := b.c.IsUserIdentitySet()
	if err != nil || !set {
		return Person{}, false, err
	}
	i, err := b.c.GetUserIdentity()
	if err != nil {
		return Person{}, false, err
	}
	return person(i), true, nil
}

// NewIssue creates an issue as the user identity, with labels, and returns
// its id.
func (b *Backend) NewIssue(title, body string, labels []string) (string, error) {
	bug, _, err := b.c.Bugs().New(title, body)
	if err != nil {
		return "", fmt.Errorf("creating the issue: %w", err)
	}
	if len(labels) > 0 {
		if _, _, err := bug.ChangeLabels(labels, nil); err != nil {
			return "", fmt.Errorf("labelling the issue: %w", err)
		}
		if err := bug.Commit(); err != nil {
			return "", err
		}
	}
	return bug.Id().String(), nil
}

// AddComment comments on an issue as the user identity, and returns the
// comment's id.
func (b *Backend) AddComment(idPrefix, body string) (string, error) {
	var id entity.CombinedId
	err := b.write(idPrefix, func(bug *cache.BugCache) (err error) {
		id, _, err = bug.AddComment(body)
		return err
	})
	return string(id), err
}

// EditComment replaces the text of one of an issue's comments, or of its
// description (the created event's id). base is the text the edit started
// from: if the comment holds another one now, the edit is refused ("" skips
// the check).
func (b *Backend) EditComment(idPrefix, commentID, body, base string) error {
	return b.write(idPrefix, func(bug *cache.BugCache) error {
		c, err := bug.Snapshot().SearchComment(entity.CombinedId(commentID))
		if err != nil {
			return ErrNoSuchComment
		}
		if base != "" && c.Message != base {
			return ErrChangedMeanwhile
		}
		_, err = bug.EditComment(c.CombinedId(), body)
		return err
	})
}

// SetTitle renames an issue.
func (b *Backend) SetTitle(idPrefix, title string) error {
	return b.write(idPrefix, func(bug *cache.BugCache) error {
		if bug.Snapshot().Title == title {
			return nil
		}
		_, err := bug.SetTitle(title)
		return err
	})
}

// SetStatus opens or closes an issue.
func (b *Backend) SetStatus(idPrefix, status string) error {
	return b.write(idPrefix, func(bug *cache.BugCache) error {
		var err error
		switch {
		case status == "open" && bug.Snapshot().Status != common.OpenStatus:
			_, err = bug.Open()
		case status == "closed" && bug.Snapshot().Status != common.ClosedStatus:
			_, err = bug.Close()
		case status != "open" && status != "closed":
			err = fmt.Errorf("status %q: want open or closed", status)
		}
		return err
	})
}

// write runs change on an issue and commits what it did.
func (b *Backend) write(idPrefix string, change func(*cache.BugCache) error) error {
	bug, err := b.c.Bugs().ResolvePrefix(idPrefix)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", idPrefix, err)
	}
	if err := change(bug); err != nil {
		return err
	}
	return bug.CommitAsNeeded()
}
