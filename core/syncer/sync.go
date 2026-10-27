package syncer

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
)

// Options are git-board's settings for a run.
type Options struct {
	// PublishPrivate lets what was pulled from a project that isn't public
	// go to one that is.
	PublishPrivate bool
	// Issue, when set, syncs that issue alone.
	Issue string
	// Warn hears each refusal once, when it is recorded. Nil drops them.
	Warn func(string)
	// Local keeps a comment in git-board: one whose metadata it says true
	// for is never posted. Nil posts every comment.
	Local func(meta map[string]string) bool
	// Carry is what a comment's marker carries after its id: "k=v" pairs,
	// space-separated, no value holding a space or a ">". It is rebuilt
	// from the comment's metadata each run. Nil carries none.
	Carry func(meta map[string]string) string
	// Restore is the metadata a forge edit of a comment sync posted sets,
	// from its marker's pairs (marked), or with the marker deleted (not
	// marked); a key given "" is cleared. Nil or empty restores nothing.
	Restore func(carried string, marked bool, meta map[string]string) map[string]string
	// Quote is the line a comment is posted under, after its header: a
	// Markdown quote, one line starting "> ". It is the one posted from
	// then on, as the header is. Nil or "" posts none.
	Quote func(issueID string, c issue.Comment) string
	// Published is the note sync posts after pushing a new body: before is
	// the forge's last body, after git-board's; id names the version (an
	// operation's id), posted once. id "" posts none.
	Published func(issueID, before, after string) (id, note string)
	// ReplyTo is the comment a comment replies to, "" for none: on a forge
	// with threads (forge.Replier), it is posted in its root's. Nil is none.
	ReplyTo func(meta map[string]string) string
	// Held keeps a label in git-board on the project named by key: never
	// sent there, and the project's own copy of it neither pulled nor
	// removed. Nil holds none.
	Held func(key, label string) bool
	// Did hears each issue the run wrote for, as Report counts them, on the
	// project named by its key: "created" and "updated" with the number of
	// operations pulled, "linked" for a forge issue found by its marker or
	// its origin, "joined" for a mirror the fork posted, "posted" and
	// "pushed". Nil hears nothing.
	Did func(forge, what, issue string, ops int)
	// Mirrors are forge issues the fork made one issue, from its own data,
	// each group's places as "<key>#<number>", keys in any case (sync.md's
	// "Switching from the fork"). Nil leaves the fork's texts to say.
	Mirrors [][]string
	// Create, when set, is asked before the push creates issues on a
	// project: its key, how many it would create, and whether the run
	// started with no sync entity for it (first). False creates none of
	// them there, and the rest of the push goes on. Nil creates them.
	Create func(key string, n int, first bool) bool
	// Left hears each project left out of the run, by its key, with why:
	// its visibility, its posting account or its issues couldn't be read.
	// The run goes on with the others. Nil returns them all, joined, as
	// the run's error, after the others are synced.
	Left func(key string, why error)
}

// Report counts what a run did. Created, Updated and Unchanged count the
// forge issues the pull read: made into issues, written to, or left as they
// were. Posted counts the issues created on forges, and Pushed the linked
// forge issues the push wrote to.
type Report struct{ Created, Updated, Unchanged, Posted, Pushed int }

// ErrLocked is returned when another run holds the lock.
var ErrLocked = errors.New("syncer: another sync runs")

// Sync pulls from every forge, then pushes to every forge, as sync.md's "A
// run" says. A failed write stops it with the error; what was written stays
// written, and recorded.
func Sync(ctx context.Context, s *gitstore.Store, opt Options, forges ...forge.Forge) (Report, error) {
	return start(ctx, s, opt, forges, true, true)
}

// Pull is a run's pull alone.
func Pull(ctx context.Context, s *gitstore.Store, opt Options, forges ...forge.Forge) (Report, error) {
	return start(ctx, s, opt, forges, true, false)
}

// Push is a run's push alone. It reads each forge issue it writes to, and
// takes nothing from it.
func Push(ctx context.Context, s *gitstore.Store, opt Options, forges ...forge.Forge) (Report, error) {
	return start(ctx, s, opt, forges, false, true)
}

// run is one sync run's working state.
type run struct {
	ctx        context.Context
	s          *gitstore.Store
	opt        Options
	now        int64
	au         *authors
	projects   []*project
	states     map[string]*state // by forge key, the run's projects' and others read for a header or a reference
	issues     map[string]*issue.Issue
	pull, push bool
	report     Report
	mirrors    map[string][]string // a place, in lower case -> the others of its group
	later      []listed            // the fork's mirrors, pulled once every original is
	forkIDs    map[string]string   // the fork's marker id -> the issue it went to
}

// listed is a forge issue a project listed.
type listed struct {
	p  *project
	fi forge.Issue
}

// project is one forge project in a run.
type project struct {
	f      forge.Forge
	key    string
	vis    forge.Visibility
	me     forge.Account
	st     *state
	first  bool                     // no sync entity when the run started
	list   []forge.Issue            // the pull's listing, read before any write
	listed map[string]forge.Issue   // forge id -> as read this run
	tokens map[string]forge.Forge   // account id -> its view, nil when it has no token
	acc    map[string]forge.Account // person id -> their account on this server
}

func start(ctx context.Context, s *gitstore.Store, opt Options, forges []forge.Forge, pull, push bool) (_ Report, err error) {
	unlock, err := lock(s)
	if err != nil {
		return Report{}, err
	}
	defer unlock()
	var unread error // the projects left out of the run, when no one hears them (Left)
	defer func() { err = errors.Join(unread, err) }()
	leave := func(key string, why error) error {
		if ctx.Err() != nil {
			return ctx.Err() // stopped, not unreadable
		}
		if opt.Left != nil {
			opt.Left(key, why)
		} else {
			unread = errors.Join(unread, fmt.Errorf("syncer: %s: %w", key, why))
		}
		return nil
	}
	r := &run{ctx: ctx, s: s, opt: opt, now: time.Now().Unix(), states: map[string]*state{}, pull: pull, push: push,
		mirrors: map[string][]string{}, forkIDs: map[string]string{}}
	for _, g := range opt.Mirrors {
		for _, place := range g {
			for _, other := range g {
				if other != place {
					r.mirrors[strings.ToLower(place)] = append(r.mirrors[strings.ToLower(place)], other)
				}
			}
		}
	}
	// ponytail: the directory loads every person once per run; the index's ForAccount when people get many
	if r.au, err = loadAuthors(s); err != nil {
		return r.report, err
	}
	for _, f := range forges {
		// a project that can't be read is left out before anything is
		// written: as not public, it would take what is private, and a
		// listing missed would make its issues look new
		vis, verr := f.Visibility(ctx)
		if verr != nil {
			if err := leave(f.Key(), fmt.Errorf("visibility: %w", verr)); err != nil {
				return r.report, err
			}
			continue
		}
		p := &project{f: f, key: f.Key(), vis: vis, listed: map[string]forge.Issue{}, tokens: map[string]forge.Forge{}, acc: map[string]forge.Account{}}
		var merr error
		if p.me, merr = f.Me(ctx); merr != nil {
			if err := leave(p.key, fmt.Errorf("posting account: %w", merr)); err != nil {
				return r.report, err
			}
			continue
		}
		if p.st, err = loadState(s, p.key); err != nil {
			return r.report, err
		}
		p.first = p.st.log == nil
		{
			var lerr error
			if p.list, lerr = r.listing(p); lerr != nil {
				if err := leave(p.key, fmt.Errorf("its issues: %w", lerr)); err != nil {
					return r.report, err
				}
				continue
			}
		}
		r.states[p.key] = p.st
		r.projects = append(r.projects, p)
	}
	if err := r.loadIssues(); err != nil {
		return r.report, err
	}
	if pull {
		for _, p := range r.projects {
			if err := r.pullFrom(p); err != nil {
				return r.report, err
			}
		}
		for _, x := range r.later {
			if err := r.pullOne(x.p, x.fi, true); err != nil {
				return r.report, err
			}
		}
	}
	if push {
		for _, p := range r.projects {
			if !pull {
				if err := r.relink(p); err != nil {
					return r.report, err
				}
			}
			if err := r.pushTo(p); err != nil {
				return r.report, err
			}
		}
	}
	if !pull || !push || opt.Issue != "" {
		return r.report, nil
	}
	for _, p := range r.projects {
		if !p.st.wrote {
			continue // a quiet run records nothing, and the entity doesn't grow
		}
		who, err := r.au.person(ctx, p.f.Server(), p.me)
		if err != nil {
			return r.report, err
		}
		if err := p.st.write(ctx, s, p.key, []oplog.Op{runOp(who.ID, time.Now().Unix(), r.now)}); err != nil {
			return r.report, err
		}
	}
	return r.report, nil
}

// lock takes the run's lock file, made only if it doesn't exist.
func lock(s *gitstore.Store) (func(), error) {
	dir := filepath.Join(s.CommonDir(), "board")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "sync.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("%w: %s exists; remove it if no sync runs", ErrLocked, path)
	}
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(f, "%d\n", os.Getpid())
	f.Close()
	return func() { os.Remove(path) }, nil
}

// loadIssues reads every issue.
// ponytail: every run loads every issue; the index when repositories get big.
func (r *run) loadIssues() error {
	ids, err := issue.List(r.s)
	if err != nil {
		return err
	}
	r.issues = map[string]*issue.Issue{}
	for _, id := range ids {
		is, err := issue.Load(r.s, id)
		if err != nil {
			return fmt.Errorf("syncer: issue %s: %w", id, err)
		}
		r.issues[id] = is
	}
	return nil
}

// oldestFirst is the issues' ids, by creation time, then id.
func (r *run) oldestFirst() []string {
	ids := slices.Collect(maps.Keys(r.issues))
	slices.SortFunc(ids, func(a, b string) int {
		return cmp.Or(cmp.Compare(r.issues[a].Created, r.issues[b].Created), strings.Compare(a, b))
	})
	return ids
}

func (r *run) did(key, what, id string, ops int) {
	if r.opt.Did != nil {
		r.opt.Did(key, what, id, ops)
	}
}

func (r *run) warn(msg string) {
	if r.opt.Warn != nil {
		r.opt.Warn(msg)
	}
}

// stateOf is a project's sync state, read once.
func (r *run) stateOf(key string) *state {
	if st, ok := r.states[key]; ok {
		return st
	}
	st, err := loadState(r.s, key)
	if err != nil {
		st = emptyState() // ponytail: an unreadable project translates nothing
	}
	r.states[key] = st
	return st
}

// isPublic says whether a project of this run is public. Others aren't,
// nor is an internal one: everyone may not read what came from it.
func (r *run) isPublic(key string) bool {
	for _, p := range r.projects {
		if p.key == key {
			return p.vis == forge.Public
		}
	}
	return false
}

// blocked says whether an operation from origin may not go to p: p is read
// beyond its members, as a public or an internal project is, and origin
// names a project that isn't public.
func (r *run) blocked(p *project, origins ...*oplog.Origin) bool {
	if p.vis == forge.Private || r.opt.PublishPrivate {
		return false
	}
	for _, o := range origins {
		if o != nil && !r.isPublic(o.Forge) {
			return true
		}
	}
	return false
}

// token is p through the account's own token, or nil.
func (r *run) token(p *project, a forge.Account) forge.Forge {
	if f, ok := p.tokens[a.ID]; ok {
		return f
	}
	f, err := p.f.As(r.ctx, a)
	if err != nil {
		f = nil // ponytail: any failure to get a token counts as none
	}
	p.tokens[a.ID] = f
	return f
}

// isSync says whether an account is one of sync's on p: the posting
// account, or one with a token.
func (r *run) isSync(p *project, a forge.Account) bool {
	return a.ID == p.me.ID || r.token(p, a) != nil
}

// account is a person's account on p's server.
func (r *run) account(p *project, person string) (forge.Account, bool) {
	if a, ok := p.acc[person]; ok {
		return a, a.ID != ""
	}
	var a forge.Account
	if pe := r.au.byID(person); pe != nil {
		for _, x := range pe.Accounts {
			if x.Server == p.f.Server() {
				a = forge.Account{ID: x.ID, Login: x.Login}
				break
			}
		}
	}
	p.acc[person] = a
	return a, a.ID != ""
}

// writer is who writes a change by person on p: the person, with their own
// token, or the posting account. direct is the former.
func (r *run) writer(p *project, person string) (f forge.Forge, direct bool) {
	if a, ok := r.account(p, person); ok {
		if f := r.token(p, a); f != nil {
			return f, true
		}
	}
	return p.f, false
}

// form puts a text written on project from in p's form. A text written in
// git-board (from "") and one going back to its own project stay as they
// are.
func (r *run) form(p *project, text, from string, title bool) string {
	if from == "" || from == p.key {
		return text
	}
	q := r.stateOf(from)
	return translate(text, title, func(n string) string {
		id, ok := q.numbers[n]
		if ok {
			if l, ok := p.st.link[id]; ok {
				return "#" + l.Number
			}
			if !title {
				return "[" + from + "#" + n + "](" + q.link[id].URL + ")"
			}
		}
		return "`#" + n + "`"
	})
}

// header names a text's author, its day, and where it was written: in
// git-board, or on the forge of origin, linking the original issue.
func (r *run) header(author string, written int64, origin *oplog.Origin, issueID string) string {
	name := author
	if pe := r.au.byID(author); pe != nil {
		name = pe.Name
	}
	where := "in git-board"
	if origin != nil {
		where = "on " + origin.Forge
		if l, ok := r.stateOf(origin.Forge).link[issueID]; ok {
			where = "on [" + origin.Forge + "#" + l.Number + "](" + l.URL + ")"
		}
	}
	return header(name, written, where)
}

func origin(c issue.Change) string {
	if c.Origin == nil {
		return ""
	}
	return c.Origin.Forge
}

// listing is p's issues a run reads: those updated since its last run, or
// those of the issue synced alone.
func (r *run) listing(p *project) ([]forge.Issue, error) {
	if r.opt.Issue != "" {
		return r.alone(p)
	}
	return p.f.Issues(r.ctx, p.st.lastRun)
}

// pullFrom reads p's issues and pulls each, as sync.md's pull says.
func (r *run) pullFrom(p *project) error {
	for _, fi := range p.list {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		if fi.Status != "open" && fi.Status != "closed" {
			return fmt.Errorf("syncer: forge issue %s: bad status %q", fi.ID, fi.Status)
		}
		p.listed[fi.ID] = fi
		if err := r.pullOne(p, fi, r.opt.Issue != ""); err != nil {
			return err
		}
	}
	return nil
}

// pullOne pulls a listed forge issue. A mirror the fork posted waits for
// every original to be pulled, unless final.
func (r *run) pullOne(p *project, fi forge.Issue, final bool) error {
	id, err := r.match(p, fi, final)
	if err != nil {
		return err
	}
	if r.opt.Issue != "" && id != r.opt.Issue {
		r.report.Unchanged++
		return nil
	}
	switch id {
	case "":
		r.report.Unchanged++
		return nil
	case "later":
		r.later = append(r.later, listed{p, fi})
		return nil
	case "new":
		id, err := r.pullNew(p, fi)
		if err != nil {
			return err
		}
		if w := unwrap(fi.Body); w.forkID != "" && r.isSync(p, fi.Author) {
			r.forkIDs[w.forkID] = id
		}
		r.report.Created++
		return nil
	}
	wrote, _, pulled, err := r.reconcile(p, id, fi, true, false)
	if err != nil {
		return err
	}
	if wrote {
		r.report.Updated++
		r.did(p.key, "updated", id, pulled)
	} else {
		r.report.Unchanged++
	}
	return nil
}

// alone lists what concerns the issue synced alone on p: its forge issue
// where it is linked, else the issues listed since the last run that carry
// its marker or are its origin.
func (r *run) alone(p *project) ([]forge.Issue, error) {
	id := r.opt.Issue
	if l, ok := p.st.link[id]; ok {
		fi, err := p.f.Issue(r.ctx, forge.Ref{ID: l.ForgeID, Number: l.Number})
		if errors.Is(err, forge.ErrNotFound) || errors.Is(err, forge.ErrConfidential) {
			return nil, nil
		}
		return []forge.Issue{fi}, err
	}
	list, err := p.f.Issues(r.ctx, p.st.lastRun)
	if err != nil {
		return nil, err
	}
	is := r.issues[id]
	return slices.DeleteFunc(list, func(fi forge.Issue) bool {
		w := unwrap(fi.Body)
		mine := w.kind == "issue" && w.id == id && r.isSync(p, fi.Author)
		from := is != nil && is.Origin != nil && *is.Origin == oplog.Origin{Forge: p.key, Event: "issue:" + fi.ID}
		return !mine && !from
	}), nil
}

// match finds the issue a listed forge issue is: its link, else the issue
// its marker names (a push that stopped before its link), else the issue
// pulled from it (a pull that stopped before its link), else the issue a
// mirror the fork posted mirrors. It links the last three. It returns "new"
// for none, "" for one to skip, and "later" for a mirror that waits.
func (r *run) match(p *project, fi forge.Issue, final bool) (string, error) {
	if id, ok := p.st.links[fi.ID]; ok {
		if _, here := r.issues[id]; !here {
			return "", nil // the issue may arrive with a fetch, or isn't one
		}
		return id, nil
	}
	if id, ok, err := r.byMarker(p, fi); ok || err != nil {
		return id, err
	}
	from := oplog.Origin{Forge: p.key, Event: "issue:" + fi.ID}
	for _, id := range slices.Sorted(maps.Keys(r.issues)) { // ponytail: a scan per unlinked forge issue
		is := r.issues[id]
		if is.Origin != nil && *is.Origin == from && !p.st.anyLink[id] {
			r.did(p.key, "linked", id, 0)
			return id, p.st.write(r.ctx, r.s, p.key, []oplog.Op{linkOp(is.Author, r.now, id, fi.ID, fi.Number, fi.URL)})
		}
	}
	return r.mirror(p, fi, final)
}

// byMarker links an unlinked forge issue whose marker, from sync's
// accounts, names an issue: a push stopped before its link. Last seen is
// what the push sent. ok says it has such a marker; id is "" when its
// issue isn't here, or is linked already.
func (r *run) byMarker(p *project, fi forge.Issue) (id string, ok bool, err error) {
	w := unwrap(fi.Body)
	if w.kind != "issue" || !r.isSync(p, fi.Author) {
		return "", false, nil
	}
	is, here := r.issues[w.id]
	if !here || p.st.anyLink[w.id] {
		return "", true, nil
	}
	_, direct := r.writer(p, is.Author)
	title, body := r.sent(p, is, direct)
	ops := append([]oplog.Op{linkOp(is.Author, r.now, is.ID, fi.ID, fi.Number, fi.URL)}, createdSeen(is.Author, r.now, is.ID, title, body)...)
	r.did(p.key, "linked", is.ID, 0)
	return is.ID, true, p.st.write(r.ctx, r.s, p.key, ops)
}

// relink is a push alone's first step: it links, by their marker, the
// forge issues a push created and stopped before linking, so that it
// never creates one twice. It takes nothing else from the forge.
func (r *run) relink(p *project) error {
	for _, fi := range p.list {
		if _, linked := p.st.links[fi.ID]; linked {
			continue
		}
		if _, _, err := r.byMarker(p, fi); err != nil {
			return err
		}
	}
	return nil
}

// mirror finds the issue a forge issue mirrors, as sync.md's "Switching
// from the fork" says: a text in the fork's form waits until final, so
// that the originals come first; its header's link and its marker id join
// from sync's accounts only (trusted); the fork's data, Mirrors, names the
// others of its group, mirror or original. It joins the first issue found
// that isn't linked to p yet.
func (r *run) mirror(p *project, fi forge.Issue, final bool) (string, error) {
	w := unwrap(fi.Body)
	trusted := w.forked() && r.isSync(p, fi.Author)
	places := slices.Clone(r.mirrors[strings.ToLower(p.key+"#"+fi.Number)])
	switch {
	case !trusted && len(places) == 0:
		return "new", nil
	case w.forked() && !final:
		return "later", nil
	}
	var found []string
	if trusted {
		found = append(found, r.linkedAt(p, w.link), r.forkIDs[w.forkID])
	}
	for _, place := range places {
		i := strings.LastIndex(place, "#")
		for _, q := range r.projects {
			if i > 0 && q != p && strings.EqualFold(q.key, place[:i]) {
				found = append(found, q.st.numbers[place[i+1:]])
			}
		}
	}
	for _, id := range found {
		if _, here := r.issues[id]; here && !p.st.anyLink[id] {
			if trusted && w.forkID != "" {
				r.forkIDs[w.forkID] = id
			}
			return id, r.join(p, fi, id)
		}
	}
	return "new", nil
}

// linkedAt is the issue linked, on a project of the run other than p, to
// the forge issue at u, a comment's fragment aside; "" for none.
// ponytail: a scan of every link per mirror, for a switch's one pull
func (r *run) linkedAt(p *project, u string) string {
	u, _, _ = strings.Cut(u, "#")
	for _, q := range r.projects {
		for id, l := range q.st.link {
			if q != p && u != "" && l.URL == u {
				return id
			}
		}
	}
	return ""
}

// join links a mirror the fork posted to the issue it mirrors. Its title,
// body and status are last seen as the forge has them, so that git-board's,
// the original's, win. Labels and assignees aren't, so that both sides'
// sets merge.
func (r *run) join(p *project, fi forge.Issue, id string) error {
	a := r.issues[id].Author
	r.did(p.key, "joined", id, 0)
	return p.st.write(r.ctx, r.s, p.key, []oplog.Op{linkOp(a, r.now, id, fi.ID, fi.Number, fi.URL),
		seenOp(a, r.now, id, "title", fi.Title), seenOp(a, r.now, id, "body", fi.Body), seenOp(a, r.now, id, "status", fi.Status)})
}

// createdSeen are the seen values of a forge issue sync just created.
func createdSeen(author string, now int64, id, title, body string) []oplog.Op {
	return []oplog.Op{
		seenOp(author, now, id, "title", title),
		seenOp(author, now, id, "body", body),
		seenOp(author, now, id, "status", "open"),
		seenOp(author, now, id, "labels", ""),
		seenOp(author, now, id, "assignees", ""),
	}
}

// pushTo sends git-board's changes to p: it creates the issues p hasn't,
// oldest first, when Create lets it, then writes to each linked one
// git-board changed.
func (r *run) pushTo(p *project) error {
	ids := r.oldestFirst()
	if r.opt.Issue != "" {
		ids = []string{r.opt.Issue}
	}
	var todo []*issue.Issue
	for _, id := range ids {
		if is, ok := r.issues[id]; ok && r.mayCreate(p, is) {
			todo = append(todo, is)
		}
	}
	if len(todo) > 0 && (r.opt.Create == nil || r.opt.Create(p.key, len(todo), p.first)) {
		for _, is := range todo {
			if err := r.create(p, is); err != nil {
				return err
			}
		}
	}
	for _, id := range ids {
		l, ok := p.st.link[id]
		if _, here := r.issues[id]; !ok || !here {
			continue
		}
		fi, listed := p.listed[l.ForgeID]
		if !listed {
			virtual := r.virtual(p, id)
			pl, err := r.plan(p, r.issues[id], virtual, nil, false, true)
			if err != nil {
				return err
			}
			if len(pl.pushes()) == 0 {
				continue
			}
			got, err := p.f.Issue(r.ctx, forge.Ref{ID: l.ForgeID, Number: l.Number})
			if errors.Is(err, forge.ErrNotFound) || errors.Is(err, forge.ErrConfidential) {
				continue // gone or confidential: nothing goes to it
			}
			if err != nil {
				return err
			}
			fi = got
			p.listed[fi.ID] = fi
		}
		_, pushed, _, err := r.reconcile(p, id, fi, r.pull, true)
		if pushed {
			r.report.Pushed++
			r.did(p.key, "pushed", id, 0)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// virtual is the forge issue as last seen: what the forge shows when it
// changed nothing. Comparing with it tells what the push would send.
func (r *run) virtual(p *project, id string) forge.Issue {
	seen := p.st.seen[id]
	fi := forge.Issue{ID: p.st.link[id].ForgeID, Number: p.st.link[id].Number, Title: seen["title"], Body: seen["body"], Status: seen["status"],
		Labels: split(seen["labels"])}
	for _, a := range split(seen["assignees"]) {
		fi.Assignees = append(fi.Assignees, forge.Account{ID: a})
	}
	for _, field := range slices.Sorted(maps.Keys(seen)) {
		if cid, ok := strings.CutPrefix(field, "comment:"); ok {
			fi.Comments = append(fi.Comments, forge.Comment{ID: cid, Body: seen[field]})
		}
	}
	return fi
}

// mayCreate says whether an issue not linked to p goes there, as sync.md's
// "A new forge issue" says. Its title and body go with it, so their
// origins count as its create's.
func (r *run) mayCreate(p *project, is *issue.Issue) bool {
	if p.st.anyLink[is.ID] || p.st.refused[[3]string{is.ID, "issue", ""}] {
		return false
	}
	if o := is.Origin; o != nil {
		if o.Forge == p.key {
			return false
		}
		if fid, ok := strings.CutPrefix(o.Event, "issue:"); ok {
			if other, ok := r.stateOf(o.Forge).links[fid]; ok && other != is.ID {
				return false // a duplicate
			}
		}
	}
	return !r.blocked(p, is.Origin, is.Changes["title"].Origin, is.Changes["body"].Origin)
}

// sent is what creating the issue on p sends: its title and body in p's
// form, the body with its marker, and with a header unless its author
// posts it with their own token (direct).
func (r *run) sent(p *project, is *issue.Issue, direct bool) (title, body string) {
	w := wrapped{body: r.form(p, is.Body, origin(is.Changes["body"]), false), marker: marker("issue", is.ID, false, "")}
	if !direct {
		w.header = r.header(is.Author, is.Created, is.Origin, is.ID)
	}
	return r.form(p, is.Title, origin(is.Changes["title"]), true), w.String()
}

// create makes the issue on p with its title and body, and links it.
func (r *run) create(p *project, is *issue.Issue) error {
	fw, direct := r.writer(p, is.Author)
	title, body := r.sent(p, is, direct)
	fi, err := fw.CreateIssue(r.ctx, title, body)
	if errors.Is(err, forge.ErrRefused) {
		return r.refuse(p, is, "issue", "", err)
	}
	if err != nil {
		return err
	}
	ops := append([]oplog.Op{linkOp(is.Author, time.Now().Unix(), is.ID, fi.ID, fi.Number, fi.URL)},
		createdSeen(is.Author, time.Now().Unix(), is.ID, title, body)...)
	if err := p.st.write(r.ctx, r.s, p.key, ops); err != nil {
		return err
	}
	fi.Title, fi.Body, fi.Status, fi.Labels, fi.Assignees, fi.Comments = title, body, "open", nil, nil, nil
	p.listed[fi.ID] = fi
	r.report.Posted++
	r.did(p.key, "posted", is.ID, 0)
	return nil
}

// refuse records a refused value, and warns once.
func (r *run) refuse(p *project, is *issue.Issue, field, value string, why error) error {
	if p.st.refused[[3]string{is.ID, field, value}] {
		return nil
	}
	reason := why.Error()
	if err := p.st.write(r.ctx, r.s, p.key, []oplog.Op{refusedOp(is.Author, time.Now().Unix(), is.ID, field, value, reason)}); err != nil {
		return err
	}
	shown := value
	if len(shown) > 60 {
		shown = shown[:60] + "…"
	}
	r.warn(fmt.Sprintf("%s refused %s %q of issue %s: %s", p.key, field, shown, is.ID, reason))
	return nil
}
