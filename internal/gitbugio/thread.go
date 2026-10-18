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
	// Bridge, in a Thread, is the sync bridge whose project it is: what
	// link, unlink and take name. "" when no sync bridge is for it.
	Bridge string `json:"bridge,omitempty"`
	// Gone: "unlinked", or "deleted" on the forge. Not synced anymore.
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
	// comment already there: its latest edit hasn't.
	Pending     []string `json:"pending,omitempty"`
	EditPending []string `json:"edit_pending,omitempty"`
	// EditWas, with EditPending, is the text the forges have: the newest
	// version that reached them, for a diff of what the sync will change.
	EditWas string `json:"edit_was,omitempty"`

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

// Threads is Thread for each id prefix, reading once what they all need:
// the counter, and the sync bridges' projects.
func (b *Backend) Threads(idPrefixes []string) ([]Thread, error) {
	c, projects := b.counter(), b.syncProjects()
	res := make([]Thread, 0, len(idPrefixes))
	for _, p := range idPrefixes {
		th, err := b.thread(p, c, projects)
		if err != nil {
			return nil, err
		}
		res = append(res, th)
	}
	return res, nil
}

func (b *Backend) thread(idPrefix string, c *Counter, projects []syncProject) (Thread, error) {
	bug, err := b.c.Bugs().ResolvePrefix(idPrefix)
	if err != nil {
		return Thread{}, fmt.Errorf("resolving %s: %w", idPrefix, err)
	}
	excerpt, err := b.c.Bugs().ResolveExcerpt(bug.Id())
	if err != nil {
		return Thread{}, fmt.Errorf("resolving %s: %w", idPrefix, err)
	}
	snap := bug.Snapshot()
	issue := b.issueFrom(bug.Id().String(), excerpt, c)
	createMeta := snap.Operations[0].AllMetadata()
	_, remotes := forgesOf(createMeta)
	on := onForges(snap, remotes)
	forgeBridges(issue.Forges, createMeta, projects)
	th := Thread{
		Issue:     issue,
		Author:    person(snap.Author),
		CreatedAt: snap.CreateTime,
		EditedAt:  snap.EditTime(),
	}
	for _, p := range snap.Participants {
		th.Participants = append(th.Participants, person(p))
	}
	for _, item := range snap.Timeline {
		th.Events = append(th.Events, event(snap, item, on))
	}
	return th, nil
}

// forgeState tells, per operation, which forges a change hasn't reached.
type forgeState struct {
	pending  func(opID entity.Id) []string
	ops      map[entity.CombinedId]dag.Operation // by the timeline's ids
	lastEdit map[entity.Id]entity.Id             // comment's op → its latest edit
	edits    map[entity.Id][]dag.Operation       // comment's op → its edits, oldest first
}

// onForges works out where each operation is, as the sync engine records it
// (bridge/core synctags): an operation pulled from a forge or pushed to it
// carries sync:<remote>: keys, its own or set later by a SetMetadata. What
// came before the issue was created on a forge by a push went with it.
func onForges(snap *bugent.Snapshot, remotes map[string]string) forgeState {
	meta := map[entity.Id]map[string]string{}
	since := map[string]int{} // remote prefix → index from which operations must be pushed
	st := forgeState{ops: map[entity.CombinedId]dag.Operation{}, lastEdit: map[entity.Id]entity.Id{}, edits: map[entity.Id][]dag.Operation{}}
	index := map[entity.Id]int{}
	createID := snap.Operations[0].Id()
	for prefix := range remotes {
		if _, ok := snap.Operations[0].GetMetadata(prefix + "id"); ok {
			since[prefix] = 0 // created by the push itself, or pulled from it
		}
	}
	for i, op := range snap.Operations {
		index[op.Id()] = i
		st.ops[entity.CombineIds(snap.Id(), op.Id())] = op
		meta[op.Id()] = op.AllMetadata()
		switch o := op.(type) {
		case *dag.SetMetadataOperation[*bugent.Snapshot]:
			if meta[o.Target] == nil {
				meta[o.Target] = map[string]string{}
			}
			for k, v := range o.NewMetadata {
				meta[o.Target][k] = v
			}
			// the push that created the issue on a forge records it here
			for prefix := range remotes {
				if _, ok := o.NewMetadata[prefix+"id"]; ok && o.Target == createID {
					since[prefix] = i
				}
			}
		case *bugent.EditCommentOperation:
			st.lastEdit[o.Target] = op.Id()
			st.edits[o.Target] = append(st.edits[o.Target], op)
		}
	}
	st.pending = func(opID entity.Id) []string {
		var res []string
		for prefix, name := range remotes {
			if index[opID] <= since[prefix] {
				continue
			}
			there := false
			for k := range meta[opID] {
				if strings.HasPrefix(k, prefix) {
					there = true
					break
				}
			}
			if !there {
				res = append(res, name)
			}
		}
		sort.Strings(res)
		return res
	}
	return st
}

// forgeVersion is the text of a comment the forges have: the newest of its
// versions (the comment, then each edit) that reached every forge.
func (st forgeState) forgeVersion(target entity.Id, history []bugent.CommentHistoryStep) string {
	ops := []entity.Id{target}
	for _, e := range st.edits[target] {
		ops = append(ops, e.Id())
	}
	if len(ops) != len(history) {
		return "" // ponytail: versions and edits always pair up; no diff if not
	}
	for i := len(ops) - 1; i >= 0; i-- {
		if len(st.pending(ops[i])) == 0 {
			return history[i].Message
		}
	}
	return ""
}

func event(snap *bugent.Snapshot, item bugent.TimelineItem, on forgeState) Event {
	comment := func(kind string, c bugent.CommentTimelineItem) Event {
		e := Event{Kind: kind, ID: string(item.CombinedId()), Author: person(c.Author), At: at(c.CreatedAt), Body: c.Message, Edited: len(c.History) > 1}
		found, err := snap.SearchComment(item.CombinedId())
		if e.Edited {
			var edits []dag.Operation
			if err == nil {
				edits = on.edits[found.TargetId()]
			}
			for i, h := range c.History {
				// git-bug leaves out who wrote the first version, and an
				// edit's author: the comment's, and the edit operation's
				who := h.Author
				switch {
				case who != nil:
				case i == 0:
					who = c.Author
				case i-1 < len(edits):
					who = edits[i-1].Author()
				}
				e.History = append(e.History, Version{Author: person(who), At: at(h.UnixTime), Body: h.Message})
			}
		}
		if err == nil {
			e.Pending = on.pending(found.TargetId())
			if edit, ok := on.lastEdit[found.TargetId()]; ok && len(e.Pending) == 0 {
				e.EditPending = on.pending(edit)
				if len(e.EditPending) > 0 {
					e.EditWas = on.forgeVersion(found.TargetId(), c.History)
				}
			}
		}
		return e
	}
	change := func(e Event) Event {
		if op, ok := on.ops[item.CombinedId()]; ok {
			e.Pending = on.pending(op.Id())
		}
		return e
	}
	switch it := item.(type) {
	case *bugent.CreateTimelineItem:
		return comment("created", it.CommentTimelineItem)
	case *bugent.AddCommentTimelineItem:
		return comment("comment", it.CommentTimelineItem)
	case *bugent.LabelChangeTimelineItem:
		return change(Event{Kind: "labels", ID: string(it.CombinedId()), Author: person(it.Author), At: at(it.UnixTime), Added: labelStrings(it.Added), Removed: labelStrings(it.Removed)})
	case *bugent.SetStatusTimelineItem:
		status := "open"
		if it.Status == common.ClosedStatus {
			status = "closed"
		}
		return change(Event{Kind: "status", ID: string(it.CombinedId()), Author: person(it.Author), At: at(it.UnixTime), Status: status})
	case *bugent.SetTitleTimelineItem:
		return change(Event{Kind: "title", ID: string(it.CombinedId()), Author: person(it.Author), At: at(it.UnixTime), Title: it.Title, Was: it.Was})
	}
	return Event{Kind: "unknown", ID: string(item.CombinedId())}
}

// forgesOf reads where an issue is from its create operation's metadata:
// the sync engine's sync:<remote>:url (and :number), then the older
// bridges' <kind>-url (not gitlab-base-url); each gone when its project's
// sync:<remote>:gone says so, the one place that tells. remotes maps each
// sync remote's key prefix ("sync:github.com/o/r:") to its forge's name,
// for the comments to say which forges they haven't reached; not an
// unlinked one.
func forgesOf(meta map[string]string) (forges []Forge, remotes map[string]string) {
	remotes = map[string]string{}
	seen := map[string]int{} // URL → its forge's index
	keys := make([]string, 0, len(meta))
	for k := range meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// add lists the forge of a URL once, and says where it is
	add := func(u, number string) int {
		if i, ok := seen[u]; ok {
			return i
		}
		if number == "" {
			number = lastNumber(u)
		}
		seen[u] = len(forges)
		forges = append(forges, Forge{Name: forgeName(u), URL: u, Number: number})
		return seen[u]
	}
	for _, k := range keys {
		if prefix, ok := strings.CutSuffix(k, ":url"); ok && strings.HasPrefix(k, "sync:") {
			i := add(meta[k], meta[prefix+":number"])
			if g := gone(meta[prefix+":gone"]); g != "" {
				forges[i].Gone = g
			} else if _, onForge := meta[prefix+":id"]; onForge {
				remotes[prefix+":"] = forges[i].Name
			}
		}
	}
	for _, k := range keys {
		if kind, ok := strings.CutSuffix(k, "-url"); ok && !strings.HasSuffix(kind, "-base") && !strings.Contains(k, ":") {
			i := add(meta[k], "")
			if key := legacyKey(kind, meta); key != "" {
				if g := gone(meta["sync:"+key+":gone"]); g != "" {
					forges[i].Gone = g
				}
			}
		}
	}
	return forges, remotes
}

// legacyKey is the sync key of the project an older bridge's issue is on,
// as the engine makes it (Key of bridge/{github,gitlab}/remote.go): where
// Unlink, or a push that found it deleted, marks it gone. "" for a kind the
// engine doesn't sync.
func legacyKey(kind string, meta map[string]string) string {
	u, err := url.Parse(meta[kind+"-url"])
	if err != nil {
		return ""
	}
	switch kind {
	case "github": // https://github.com/<owner>/<project>/issues/<n>
		owner, rest, _ := strings.Cut(strings.TrimPrefix(u.Path, "/"), "/")
		project, _, _ := strings.Cut(rest, "/")
		return "github.com/" + strings.ToLower(owner+"/"+project)
	case "gitlab":
		// older exports left the base URL empty
		if base, err := url.Parse(meta["gitlab-base-url"]); err == nil && base.Host != "" {
			u = base
		}
		return u.Host + "/" + meta["gitlab-project-id"]
	}
	return ""
}

// gone reads the engine's sync:<remote>:gone mark: Unlink's, or a push's
// that found the issue deleted.
func gone(mark string) string {
	switch mark {
	case "":
		return ""
	case "unlinked":
		return "unlinked"
	}
	return "deleted"
}

// syncProject is a sync bridge's project, as forgeBridges finds an issue's
// forge in it.
type syncProject struct {
	bridge, target string
	key            string // the engine's: sync:<key>:url
	project        string // github, gitea: owner/project, lowercased
	id, baseURL    string // gitlab
}

// syncProjects reads the sync bridges' projects from git-bug's config.
//
// ponytail: mirrors Key and Legacy of bridge/{github,gitlab,gitea}/remote.go,
// which need the forge's API; git-bug exporting them would replace this.
func (b *Backend) syncProjects() []syncProject {
	bridges, err := b.SyncBridges()
	if err != nil {
		return nil
	}
	cfg, err := b.c.Config().Read()
	if err != nil {
		return nil
	}
	var res []syncProject
	for _, br := range bridges {
		get := func(k string) string { return cfg.Value("git-bug.bridge."+br.Name+"."+k, "") }
		host := ""
		if u, err := url.Parse(get("base-url")); err == nil {
			host = u.Host
		}
		p := syncProject{bridge: br.Name, target: br.Target, project: strings.ToLower(get("owner") + "/" + get("project")), id: get("project-id"), baseURL: get("base-url")}
		switch br.Target {
		case "github":
			p.key = "github.com/" + p.project
		case "gitlab":
			p.key = host + "/" + p.id
		case "gitea":
			p.key = host + "/" + p.project
		}
		res = append(res, p)
	}
	return res
}

// forgeBridges names the sync bridge of each forge an issue is on, as the
// engine finds a bug's issue: by the sync keys of the bridge's project, or
// the metadata an older bridge left (an issue it pulled).
func forgeBridges(forges []Forge, meta map[string]string, projects []syncProject) {
	for _, p := range projects {
		var legacy string
		switch p.target {
		case "github":
			if _, ok := meta["github-id"]; ok && strings.HasPrefix(strings.ToLower(meta["github-url"]), "https://github.com/"+p.project+"/issues/") {
				legacy = meta["github-url"]
			}
		case "gitlab":
			base := strings.TrimSuffix(meta["gitlab-base-url"], "/")
			if _, ok := meta["gitlab-id"]; ok && meta["gitlab-project-id"] == p.id && (base == "" || base == strings.TrimSuffix(p.baseURL, "/")) {
				legacy = meta["gitlab-url"]
			}
		}
		u := meta["sync:"+p.key+":url"]
		if u == "" {
			u = legacy
		}
		for i := range forges {
			if u != "" && forges[i].URL == u {
				forges[i].Bridge = p.bridge
			}
		}
	}
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
