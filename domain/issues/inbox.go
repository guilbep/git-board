package issues

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/guilbep/git-board/domain/people"
	"github.com/guilbep/git-board/kernel"
)

// The inbox: what one person follows and has seen, a file of the clone
// (v1's core/inbox, the same file), and the rows, what happened that
// concerns them.

// Window is how far back the inbox looks: issues updated since, events
// dated since.
const Window = 14 * 24 * time.Hour

// Inbox is one person's file.
type Inbox struct {
	Seen     map[string][]string `json:"seen,omitempty"`     // issue id → the event ids seen
	Follow   []string            `json:"follow,omitempty"`   // issues followed by hand
	Unfollow []string            `json:"unfollow,omitempty"` // and unfollowed
	Views    []string            `json:"views,omitempty"`    // views followed, by name
	Fields   bool                `json:"fields,omitempty"`   // also field changes on issues you're in
}

// inboxPath is a person's file in the clone's folder, where v1 keeps it.
func inboxPath(folder string, id kernel.ID) string {
	return filepath.Join(folder, "git-board", "inbox", string(id)+".json")
}

// InboxStamp names the state of id's inbox file (what they saw, what
// they follow): its size and time, "none" when there is none; "" when it
// can't be told.
func InboxStamp(folder string, id kernel.ID) string {
	if inboxID(id) != nil {
		return ""
	}
	st, err := os.Stat(inboxPath(folder, id))
	if errors.Is(err, fs.ErrNotExist) {
		return "none"
	}
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d %d", st.Size(), st.ModTime().UnixNano())
}

// inboxID refuses an id that isn't 40 hex digits: it names a file.
func inboxID(id kernel.ID) error {
	if len(id) != 40 || strings.Trim(string(id), "0123456789abcdef") != "" {
		return fmt.Errorf("inbox: %q isn't a person's id", id)
	}
	return nil
}

// LoadInbox reads id's file in folder; none yet is an empty Inbox.
func LoadInbox(folder string, id kernel.ID) (Inbox, error) {
	if err := inboxID(id); err != nil {
		return Inbox{}, err
	}
	data, err := os.ReadFile(inboxPath(folder, id))
	if errors.Is(err, fs.ErrNotExist) {
		return Inbox{}, nil
	}
	if err != nil {
		return Inbox{}, err
	}
	var in Inbox
	if err := json.Unmarshal(data, &in); err != nil {
		rel := filepath.Join("git-board", "inbox", string(id)+".json")
		return Inbox{}, fmt.Errorf("reading %s: %w (deleting it starts over, your follows with it)", rel, err)
	}
	return in, nil
}

// SaveInbox writes id's file through a temporary file and a rename: a
// crash leaves the old file or the new one.
func SaveInbox(folder string, id kernel.ID, in Inbox) error {
	if err := inboxID(id); err != nil {
		return err
	}
	p := inboxPath(folder, id)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(in)
	if err != nil {
		return err
	}
	if err := os.WriteFile(p+".tmp", append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(p+".tmp", p)
}

// MarkSeen takes every event th has now as seen, counted or not.
func (in *Inbox) MarkSeen(th Thread) {
	if in.Seen == nil {
		in.Seen = map[string][]string{}
	}
	ids := make([]string, len(th.Events))
	for i, e := range th.Events {
		ids[i] = e.ID
	}
	in.Seen[th.ID] = ids
}

// SetFollow is Follow (true), Unfollow (false), or back to automatic (nil).
func (in *Inbox) SetFollow(id string, follow *bool) {
	other := func(x string) bool { return x == id }
	in.Follow = slices.DeleteFunc(in.Follow, other)
	in.Unfollow = slices.DeleteFunc(in.Unfollow, other)
	switch {
	case follow == nil:
	case *follow:
		in.Follow = append(in.Follow, id)
	default:
		in.Unfollow = append(in.Unfollow, id)
	}
}

// Set is what was chosen for an issue: true, false, or nil (automatic).
func (in Inbox) Set(id string) *bool {
	yes, no := true, false
	switch {
	case slices.Contains(in.Follow, id):
		return &yes
	case slices.Contains(in.Unfollow, id):
		return &no
	}
	return nil
}

// Prune forgets what was seen of the issues not in keep.
func (in *Inbox) Prune(keep map[string]bool) {
	for id := range in.Seen {
		if !keep[id] {
			delete(in.Seen, id)
		}
	}
}

// InboxMe is who an inbox is for: the person who writes, and @me's login.
type InboxMe struct {
	ID    string `json:"id"`
	Login string `json:"login"`
}

// Is says p is you: your person, or any carrying your login.
func (m InboxMe) Is(p people.Who) bool {
	return string(p.ID) == m.ID || (m.Login != "" && strings.EqualFold(p.Login, m.Login))
}

// InboxRow is one issue with something new for you.
type InboxRow struct {
	Issue   Issue        `json:"issue"`
	Reasons []string     `json:"reasons"` // why you follow it; none: addressed to you only
	News    []string     `json:"news"`    // "2 comments", "closed", "added you as reviewer"…
	Who     []people.Who `json:"who"`
	Latest  time.Time    `json:"latest"`
	First   string       `json:"first"` // the oldest of those events
}

// projectRole marks a reason that's a role held for the whole project.
const projectRole = " (project)"

// Follows is whether you follow th, why, and what you chose (nil:
// automatic). views are the followed views th is in.
func Follows(me InboxMe, in Inbox, th Thread, views []string, s *Schema) (bool, []string, *bool) {
	reasons := Involvement(involvedIn(me, th), me.Login, me.ID, s)
	set := in.Set(th.ID)
	if set != nil && *set {
		reasons = append(reasons, "followed")
	}
	for _, v := range views {
		reasons = append(reasons, "view: "+v)
	}
	following := len(reasons) > 0 && (set == nil || *set)
	if !following && set == nil {
		for _, l := range th.Inherited {
			if f, ok := PersonField(l, me.Login, s); ok {
				reasons = append(reasons, f+projectRole)
			}
		}
	}
	return following, reasons, set
}

// involvedIn reads th as Involvement reads a listed issue; you, by
// person or by login, count as your login.
func involvedIn(me InboxMe, th Thread) Involved {
	login := func(p people.Who) string {
		if me.Is(p) {
			return me.Login
		}
		return p.Login
	}
	in := Involved{AuthorLogins: []string{login(th.Author)}, AuthorID: string(th.Author.ID), Labels: th.Labels}
	for _, e := range th.Events {
		if e.Kind == "comment" && e.Author.ID != th.Author.ID {
			in.CommenterLogins = append(in.CommenterLogins, login(e.Author))
			in.CommenterIDs = append(in.CommenterIDs, string(e.Author.ID))
		}
		if (e.Kind == "created" || e.Kind == "comment") && Mentions(e.Body, me.Login) {
			in.Mentioned = true
		}
	}
	return in
}

// Rows is what happened that concerns you on threads, newest first:
// events dated since, unseen, not yours. inViews are the followed views
// each issue is in, by id.
func Rows(me InboxMe, in Inbox, threads []Thread, inViews map[string][]string, s *Schema, since time.Time) []InboxRow {
	var rows []InboxRow
	for _, th := range threads {
		if r, ok := rowOf(me, in, th, inViews[th.ID], s, since); ok {
			rows = append(rows, r)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Latest.After(rows[j].Latest) })
	return rows
}

func rowOf(me InboxMe, in Inbox, th Thread, views []string, s *Schema, since time.Time) (InboxRow, bool) {
	following, reasons, _ := Follows(me, in, th, views, s)
	watching := !following && slices.ContainsFunc(reasons, func(r string) bool { return strings.HasSuffix(r, projectRole) })
	inField := slices.ContainsFunc(th.Labels, func(l string) bool {
		_, ok := PersonField(l, me.Login, s)
		return ok
	})
	seen := map[string]bool{}
	for _, id := range in.Seen[th.ID] {
		seen[id] = true
	}
	row := InboxRow{Issue: th.Issue, Reasons: reasons}
	var comments, changes int
	var status string
	var notes []string // once each: new issue, added you as…, removed you as…, mentioned you
	note := func(n string) {
		if !slices.Contains(notes, n) {
			notes = append(notes, n)
		}
	}
	for _, e := range th.Events {
		if e.At.Before(since) || seen[e.ID] || me.Is(e.Author) {
			continue
		}
		addressed := false // to you: shows followed or not, unfollowed included
		for _, l := range e.Added {
			if f, ok := PersonField(l, me.Login, s); ok {
				note("added you as " + f)
				addressed = true
			}
		}
		for _, l := range e.Removed {
			if f, ok := PersonField(l, me.Login, s); ok {
				note("removed you as " + f)
				addressed = true
			}
		}
		if (e.Kind == "created" || e.Kind == "comment") && Mentions(e.Body, me.Login) {
			note("mentioned you")
			addressed = true
		}
		counted := addressed
		if watching { // a project role: its creation and closing only
			switch e.Kind {
			case "created":
				note("new issue")
				counted = true
			case "status":
				status = e.Status
				counted = true
			}
		}
		if following {
			switch e.Kind {
			case "comment":
				comments++
				counted = true
			case "created":
				note("new issue")
				counted = true
			case "status":
				status = e.Status
				counted = true
			case "labels":
				if in.Fields && inField && !addressed {
					changes++
					counted = true
				}
			}
		}
		if !counted {
			continue
		}
		if row.First == "" {
			row.First = e.ID
		}
		if e.At.After(row.Latest) {
			row.Latest = e.At
		}
		if !slices.ContainsFunc(row.Who, func(p people.Who) bool { return p.ID == e.Author.ID }) {
			row.Who = append(row.Who, e.Author)
		}
	}
	if row.First == "" {
		return InboxRow{}, false
	}
	if comments > 0 {
		row.News = append(row.News, plural(comments, "comment"))
	}
	switch status {
	case "closed":
		row.News = append(row.News, "closed")
	case "open":
		row.News = append(row.News, "reopened")
	}
	row.News = append(row.News, notes...)
	if changes > 0 {
		row.News = append(row.News, plural(changes, "field change"))
	}
	return row, true
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}

// inboxMe is who an inbox is for: the person who writes, and @me's login
// (login given, else their forge login or name).
// InboxMe is who reads: the person who writes, and their login (login,
// else their name). people.ErrNoMe when no one writes.
func (s *Service) InboxMe(ctx context.Context, login string) (InboxMe, error) {
	return s.inboxMe(ctx, login)
}

func (s *Service) inboxMe(ctx context.Context, login string) (InboxMe, error) {
	me, err := s.writer()
	if err != nil {
		return InboxMe{}, err
	}
	if login == "" {
		login = s.myName(ctx)
	}
	return InboxMe{ID: string(me), Login: login}, nil
}

// followedViews is, by issue id, the followed views among list each is
// in. A view matches without its is: (a close still shows) and its
// follows: (it can't follow itself).
func (s *Service) followedViews(ctx context.Context, dir people.Directory, views []string, me string, list []Issue, today time.Time) (map[string][]string, error) {
	inViews := map[string][]string{}
	for _, name := range views {
		v, ok := s.Views[name]
		if !ok {
			continue
		}
		text := Without(v.Filter, func(c Condition) bool { return c.Kind == QIs || c.Kind == QFollows })
		conds, me, err := s.prepare(ctx, dir, ListReq{Filter: text, Me: me, Today: today}, true)
		if err != nil {
			return nil, err
		}
		for _, is := range list {
			if Matches(is, conds, s.Schema, me, today) {
				inViews[is.ID] = append(inViews[is.ID], name)
			}
		}
	}
	return inViews, nil
}

// following is follows:@me's: what the inbox's file followed and
// unfollowed, and the issues of the views it follows.
func (s *Service) following(ctx context.Context, dir people.Directory, me string, list []Issue, today time.Time) (*Followed, error) {
	if s.Me == "" {
		return nil, &FilterError{fmt.Errorf("follows:@me: %w", people.ErrNoMe)}
	}
	in, err := LoadInbox(s.Folder, s.Me)
	if err != nil {
		return nil, err
	}
	inViews, err := s.followedViews(ctx, dir, in.Views, me, list, today)
	if err != nil {
		return nil, err
	}
	f := &Followed{Follow: map[string]bool{}, Unfollow: map[string]bool{}, InViews: map[string]bool{}}
	for _, id := range in.Follow {
		f.Follow[id] = true
	}
	for _, id := range in.Unfollow {
		f.Unfollow[id] = true
	}
	for id := range inViews {
		f.InViews[id] = true
	}
	return f, nil
}

// FollowState is whether you follow an issue, and why.
type FollowState struct {
	Following bool     `json:"following"`
	Reasons   []string `json:"reasons"`
	Set       *bool    `json:"set"` // what you chose; null: automatic
}

// String is the state as follow prints it: "following (author,
// assignee)", "not following, by choice".
func (f FollowState) String() string {
	s := "not following"
	if f.Following {
		s = "following"
	}
	if len(f.Reasons) > 0 {
		s += " (" + strings.Join(f.Reasons, ", ") + ")"
	}
	switch {
	case f.Set == nil:
		s += ", automatic"
	case !*f.Set:
		s += ", by choice"
	}
	return s
}

// Follow sets whether who writes follows an issue (nil: automatic), in
// their inbox's file, and answers whether they follow it now, and why.
// login is @me's: "" for their forge login or name.
func (s *Service) Follow(ctx context.Context, name string, set *bool, login string) (string, FollowState, error) {
	return s.follow(ctx, name, true, set, login)
}

// Following is whether who writes follows an issue, and why: Follow
// without a change.
func (s *Service) Following(ctx context.Context, name, login string) (FollowState, error) {
	_, f, err := s.follow(ctx, name, false, nil, login)
	return f, err
}

func (s *Service) follow(ctx context.Context, name string, write bool, set *bool, login string) (string, FollowState, error) {
	ctx = s.Board.Snapshot(ctx)
	m, err := s.inboxMe(ctx, login)
	if err != nil {
		return "", FollowState{}, err
	}
	th, err := s.Show(ctx, name)
	if err != nil {
		return "", FollowState{}, err
	}
	in, err := LoadInbox(s.Folder, kernel.ID(m.ID))
	if err != nil {
		return "", FollowState{}, err
	}
	if write {
		in.SetFollow(th.ID, set)
		if err := SaveInbox(s.Folder, kernel.ID(m.ID), in); err != nil {
			return "", FollowState{}, err
		}
	}
	dir, err := people.Read(ctx, s.Board, s.Avatars)
	if err != nil {
		return "", FollowState{}, err
	}
	th.Inherited = s.Schema.RolesFor(th.Labels)
	inViews, err := s.followedViews(ctx, dir, in.Views, m.Login, []Issue{th.Issue}, time.Now())
	if err != nil {
		return "", FollowState{}, err
	}
	following, reasons, chosen := Follows(m, in, th, inViews[th.ID], s.Schema)
	if reasons == nil {
		reasons = []string{}
	}
	return th.ID, FollowState{Following: following, Reasons: reasons, Set: chosen}, nil
}

// StatusReport is what status answers: your open issues, and your inbox
// in two parts.
type StatusReport struct {
	Me       InboxMe    `json:"me"`
	Assigned []Issue    `json:"assigned"`
	Opened   []InboxRow `json:"opened"`
	Follows  []InboxRow `json:"follows"`
}

// Status is what needs who writes: the open issues assigned to them, by
// update, then their inbox's rows, the issues they opened apart. With
// markRead, what it answers is marked read.
func (s *Service) Status(ctx context.Context, login string, markRead bool, now time.Time) (StatusReport, error) {
	ctx = s.Board.Snapshot(ctx)
	m, err := s.inboxMe(ctx, login)
	if err != nil {
		return StatusReport{}, err
	}
	if m.Login == "" {
		return StatusReport{}, errors.New("your person has neither a name nor a forge login, so no field and no mention can name you: give them a name, or pass --me")
	}
	in, err := LoadInbox(s.Folder, kernel.ID(m.ID))
	if err != nil {
		return StatusReport{}, err
	}
	all, err := s.List(ctx, ListReq{Me: m.Login, Today: now})
	if err != nil {
		return StatusReport{}, err
	}
	rep := StatusReport{Me: m, Assigned: []Issue{}, Opened: []InboxRow{}, Follows: []InboxRow{}}
	dir, err := people.Read(ctx, s.Board, s.Avatars)
	if err != nil {
		return StatusReport{}, err
	}
	conds, me, err := s.prepare(ctx, dir, ListReq{Filter: "assignee:@me is:open", Me: m.Login, Today: now}, true)
	if err != nil {
		return StatusReport{}, err
	}
	for _, is := range all {
		if Matches(is, conds, s.Schema, me, now) {
			rep.Assigned = append(rep.Assigned, is)
		}
	}
	sort.SliceStable(rep.Assigned, func(i, j int) bool { return rep.Assigned[i].UpdatedAt.After(rep.Assigned[j].UpdatedAt) })

	since := now.Add(-Window)
	var recent []Issue
	for _, is := range all {
		if !is.UpdatedAt.Before(since) {
			recent = append(recent, is)
		}
	}
	// the threads in one read: the people, the forges' records and the
	// logs once for all, where a Show each read them each time
	threads := make([]Thread, len(recent))
	errs := make([]error, len(recent))
	if len(recent) > 0 {
		if err := s.ThreadsOf(ctx, recent, func(i int, th Thread, err error) { threads[i], errs[i] = th, err }); err != nil {
			return StatusReport{}, err
		}
	}
	if err := errors.Join(errs...); err != nil {
		return StatusReport{}, err
	}
	for i := range threads {
		threads[i].Inherited = s.Schema.RolesFor(threads[i].Labels)
	}
	inViews, err := s.followedViews(ctx, dir, in.Views, m.Login, all, now)
	if err != nil {
		return StatusReport{}, err
	}
	for _, r := range Rows(m, in, threads, inViews, s.Schema, since) {
		if slices.Contains(r.Reasons, "author") {
			rep.Opened = append(rep.Opened, r)
		} else {
			rep.Follows = append(rep.Follows, r)
		}
	}
	if !markRead || len(rep.Opened)+len(rep.Follows) == 0 {
		return rep, nil
	}
	rows := append(slices.Clone(rep.Opened), rep.Follows...)
	keep := map[string]bool{}
	for _, th := range threads {
		keep[th.ID] = true
		if slices.ContainsFunc(rows, func(r InboxRow) bool { return r.Issue.ID == th.ID }) {
			in.MarkSeen(th)
		}
	}
	in.Prune(keep)
	return rep, SaveInbox(s.Folder, kernel.ID(m.ID), in)
}

// Seen takes every event of the issues names give as seen by who writes,
// in their inbox's file. login is @me's: "" for their forge login or name.
func (s *Service) Seen(ctx context.Context, names []string, login string) error {
	ctx = s.Board.Snapshot(ctx)
	m, err := s.inboxMe(ctx, login)
	if err != nil {
		return err
	}
	in, err := LoadInbox(s.Folder, kernel.ID(m.ID))
	if err != nil {
		return err
	}
	for _, name := range names {
		th, err := s.Show(ctx, name)
		if err != nil {
			return err
		}
		in.MarkSeen(th)
	}
	return SaveInbox(s.Folder, kernel.ID(m.ID), in)
}

// FollowViews sets what who writes follows beyond their issues: views by
// name, and the field changes on the issues they are in. A view
// .git-board.toml doesn't name is refused.
func (s *Service) FollowViews(ctx context.Context, views []string, fields bool, login string) error {
	for _, v := range views {
		if _, ok := s.Views[v]; !ok {
			return usage{fmt.Errorf("no view %q in .git-board.toml", v)}
		}
	}
	m, err := s.inboxMe(s.Board.Snapshot(ctx), login)
	if err != nil {
		return err
	}
	in, err := LoadInbox(s.Folder, kernel.ID(m.ID))
	if err != nil {
		return err
	}
	in.Views, in.Fields = slices.Clone(views), fields
	return SaveInbox(s.Folder, kernel.ID(m.ID), in)
}
