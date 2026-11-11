package issuesync

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/kernel"
	bsync "github.com/guilbep/git-board/sync"
)

// Options are git-board's settings for a run.
type Options struct {
	// Readers is how many issues a pull reads from a forge at once, ahead
	// of the pull, which takes them in order; 0 is 4 (DefaultReaders).
	Readers int
	// ForgeOnly is a board kept on the forges alone, in a copy pull fills
	// (ADR 0002, section 4): a marker from sync's accounts naming an id that
	// isn't here is pulled as new, as anyone's text is, since that id has
	// nowhere to arrive from.
	ForgeOnly bool
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
	Quote func(issueID string, c Comment) string
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
	// Assignees says whether the assignees live natively on the project
	// named by key: false plans no assignee write or read there — the
	// field's mapping gives them another home on that forge, and a field
	// is read from its home alone. Nil is true everywhere.
	Assignees func(key string) bool
	// Did hears each issue the run wrote for, as Report counts them, on the
	// project named by its key: "created" and "updated" with the number of
	// operations pulled, "linked" for a forge issue found by its marker or
	// its origin, "joined" for a mirror the fork posted, "posted" and
	// "pushed". Nil hears nothing.
	Did func(forge, what, issue string, ops int)
	// Ways is each forge's own way, by its key ([forges.<name>] way, ADR
	// 0022): a pull-only forge is written to never, and a push-only one, a
	// backup, is read from only to put the board's values back there. A
	// forge it names not is both.
	Ways map[string]bsync.Way
	// Mirrors are forge issues the fork made one issue, from its own data,
	// each group's places as "<key>#<number>", keys in any case (sync.md's
	// "Switching from the fork"). Nil leaves the fork's texts to say.
	Mirrors [][]string
	// Create, when set, is asked before the push creates issues on a
	// project: its key, how many it would create, and whether the run
	// started with no sync entity for it (first). False creates none of
	// them there, and the rest of the push goes on. Nil creates them.
	Create func(key string, n int, first bool) bool
	// Progress hears each forge issue the pull reaches, and each issue the
	// push walks: done of total, the total known before the first, and the
	// issue's name ("#43 Export CSV"). Nil hears nothing.
	Progress func(key, way string, done, total int, name string)
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
// Restored counts the issues of a backup (a forge whose way is push)
// that the run put back to the board's values: changed there, never taken.
type Report struct{ Created, Updated, Unchanged, Posted, Pushed, Restored int }

// Sync pulls from every forge, then pushes to every forge, as sync.md's "A
// run" says. A failed write stops it with the error; what was written stays
// written, and recorded.
func Sync(ctx context.Context, s *kernel.Board, opt Options, forges ...forge.Forge) (Report, error) {
	return start(ctx, s, opt, forges, true, true)
}

// Pull is a run's pull alone.
func Pull(ctx context.Context, s *kernel.Board, opt Options, forges ...forge.Forge) (Report, error) {
	return start(ctx, s, opt, forges, true, false)
}

// Push is a run's push alone. It reads each forge issue it writes to, and
// takes nothing from it.
func Push(ctx context.Context, s *kernel.Board, opt Options, forges ...forge.Forge) (Report, error) {
	return start(ctx, s, opt, forges, false, true)
}

// run is one sync run's working state.
type run struct {
	ctx        context.Context
	s          *kernel.Board
	opt        Options
	now        int64
	au         *authors
	projects   []*project
	states     map[string]*state // by forge key, the run's projects' and others read for a header or a reference
	issues     map[string]*Issue
	pull, push bool
	j          *bsync.Journal // what a cut run read, and what this one does
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
	f   forge.Forge
	key string
	way bsync.Way // the forge's own way (Options.Ways)
	// pull and push are this run's, for this forge: the run's way and the
	// forge's, both. Every step reads these, never the run's alone.
	pull, push bool
	putBack    map[string]bool // issue id -> a backup's own edit the push puts back
	vis        forge.Visibility
	me         forge.Account
	st         *state
	first      bool                     // no sync entity when the run started
	list       []forge.Issue            // the pull's listing, read before any write
	listed     map[string]forge.Issue   // forge id -> as read this run
	tokens     map[string]forge.Forge   // account id -> its view, nil when it has no token
	acc        map[string]forge.Account // person id -> their account on this server
}

func start(ctx context.Context, s *kernel.Board, opt Options, forges []forge.Forge, pull, push bool) (_ Report, err error) {
	now := time.Now().Unix()
	j, err := bsync.Lock(s.Store(), now)
	if err != nil {
		return Report{}, err
	}
	defer j.Unlock()
	wayOf := func(f forge.Forge) bsync.Way { return cmp.Or(opt.Ways[f.Key()], bsync.Both) }
	if opt.Issue == "" { // every forge whose sync is on takes part
		var keys []string
		for _, f := range forges {
			if wayOf(f).Pulls() { // a push-only forge's pull reads the journal never: its lines go
				keys = append(keys, f.Key())
			}
		}
		j.Keep(keys)
	}
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
	r := &run{ctx: ctx, s: s, opt: opt, now: now, j: j, states: map[string]*state{}, pull: pull, push: push,
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
		way := wayOf(f)
		p := &project{f: f, key: f.Key(), way: way, pull: pull && way.Pulls(), push: push && way.Pushes(), putBack: map[string]bool{}, vis: vis, listed: map[string]forge.Issue{}, tokens: map[string]forge.Forge{}, acc: map[string]forge.Account{}}
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
		p.first = !p.st.exists
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
			if !p.pull {
				continue // push-only: writes go there, and nothing is read from it as truth
			}
			if err := r.pullFrom(p); err != nil {
				return r.report, err
			}
		}
		for _, x := range r.later {
			reached, err := r.pullOne(x.p, x.fi, true)
			if err != nil {
				return r.report, err
			}
			r.reached(x.p, x.fi, reached)
		}
	}
	if push {
		for _, p := range r.projects {
			if !p.pull {
				// a push alone links by marker first, and so does a forge
				// the run never pulls: its listing is read to be written to
				if err := r.relink(p); err != nil {
					return r.report, err
				}
			}
			if !p.push {
				continue // pull-only: the run reads it, and writes nothing there
			}
			if err := r.pushTo(p); err != nil {
				return r.report, err
			}
		}
	}
	if !pull || !push || opt.Issue != "" {
		return r.report, nil
	}
	var recorded []string
	defer func() { j.Drop(recorded) }() // listed from this run's start next time: what the journal says of them is older
	for _, p := range r.projects {
		if !p.st.wrote {
			continue // a quiet run records nothing, and the entity doesn't grow
		}
		who, err := r.au.person(ctx, p.f.Server(), p.me)
		if err != nil {
			return r.report, err
		}
		if err := p.st.write(ctx, s, p.key, []kernel.Op{runOp(who.ID, time.Now().Unix(), r.now)}); err != nil {
			return r.report, err
		}
		recorded = append(recorded, p.key)
	}
	return r.report, nil
}

// loadIssues reads every issue.
// ponytail: every run loads every issue; the index when repositories get big.
func (r *run) loadIssues() error {
	ids, err := ListIssues(r.s)
	if err != nil {
		return err
	}
	r.issues = map[string]*Issue{}
	for _, id := range ids {
		is, err := LoadIssue(r.s, id)
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
func (r *run) blocked(p *project, origins ...*kernel.Origin) bool {
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
	return bsync.Translate(text, title, func(n string) string {
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
func (r *run) header(author string, written int64, origin *kernel.Origin, issueID string) string {
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
	return bsync.HeaderLine(name, written, where)
}

func origin(c Change) string {
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

// pullFrom reads p's issues and pulls each, as sync.md's pull says. Each
// is completed first, the one place a listed issue is: what follows, and
// the push's plan, read its comments. One a cut run read, unchanged
// since, is skipped (goesOn).
func (r *run) pullFrom(p *project) error {
	if r.opt.Issue == "" {
		r.j.Listed(p.key, len(p.list))
	}
	// the reads ahead: an issue goes on or is read (goesOn's answer for
	// one doesn't move while the others are pulled), Options.Readers at a time
	// ahead of the pull, which takes them in order
	skip := make([]bool, len(p.list))
	for i, fi := range p.list {
		skip[i] = r.goesOn(p, fi)
	}
	read := r.readAhead(p, skip)
	defer read.stop()
	for i, fi := range p.list {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		if skip[i] {
			// counted done by the next issue read, so the rate is the reads';
			// out of p.listed too: a push that writes to it reads it
			if w := bsync.Unwrap(fi.Body); w.ForkID != "" && r.isSync(p, fi.Author) {
				r.forkIDs[w.ForkID] = p.st.links[fi.ID] // the issue its mirrors join, as pullOne would say
			}
			r.report.Unchanged++
			continue
		}
		r.progress(p.key, "pull", i+1, len(p.list), fi.Number, fi.Title)
		whole, err := read.take(i)
		if errors.Is(err, forge.ErrNotFound) || errors.Is(err, forge.ErrConfidential) {
			r.report.Unchanged++
			continue // gone or made confidential since the listing: as if not listed
		}
		if err != nil {
			return fmt.Errorf("syncer: forge issue #%s: %w", fi.Number, err)
		}
		fi = whole
		if fi.Status != "open" && fi.Status != "closed" {
			return fmt.Errorf("syncer: forge issue %s: bad status %q", fi.ID, fi.Status)
		}
		p.listed[fi.ID] = fi
		reached, err := r.pullOne(p, fi, r.opt.Issue != "")
		if err != nil {
			return err
		}
		r.reached(p, fi, reached)
	}
	if r.opt.Issue == "" {
		r.j.Finished(p.key) // walked to its end: not cut, though some issues weren't reached
	}
	return nil
}

// goesOn says a listed forge issue was read by a run that didn't record
// its run, and is as that run read it: linked, its issue here, and not
// updated since a margin before that run started. A run of one issue
// reads all it lists.
func (r *run) goesOn(p *project, fi forge.Issue) bool {
	read, ok := r.j.Since(p.key, fi.ID)
	if !ok || r.opt.Issue != "" {
		return false
	}
	id, linked := p.st.links[fi.ID]
	_, here := r.issues[id]
	return linked && here && fi.Updated < read-bsync.GoOnMargin
}

// DefaultReaders is how many issues a pull reads at once when Options
// say none: a forge answers each in a round trip, and a board of a
// thousand issues waited a thousand in a row.
// ponytail: 4, reads alone (writes stay one at a time, paced); fewer if a
// forge's secondary limits answer 403s to it.
const DefaultReaders = 4

// readAhead reads the issues of p.list not skipped, Options.Readers at a time:
// a read waits for its slot, which the pull frees as it takes an answer,
// so no more than Readers answers wait.
type readAhead struct {
	got    []chan completed
	slots  chan struct{}
	cancel context.CancelFunc
	// one reader is none ahead: the pull reads each issue as it takes it,
	// and nothing past one that fails
	direct func(i int) (forge.Issue, error)
}

type completed struct {
	is  forge.Issue
	err error
}

func (r *run) readAhead(p *project, skip []bool) *readAhead {
	n := r.opt.Readers
	if n <= 0 {
		n = DefaultReaders
	}
	if n == 1 {
		return &readAhead{cancel: func() {}, direct: func(i int) (forge.Issue, error) { return p.f.Complete(r.ctx, p.list[i]) }}
	}
	ctx, cancel := context.WithCancel(r.ctx)
	ra := &readAhead{got: make([]chan completed, len(p.list)), slots: make(chan struct{}, n), cancel: cancel}
	for i := range p.list {
		if !skip[i] {
			ra.got[i] = make(chan completed, 1)
		}
	}
	go func() {
		for i, fi := range p.list {
			if skip[i] {
				continue
			}
			select {
			case ra.slots <- struct{}{}:
			case <-ctx.Done():
				return
			}
			go func() {
				whole, err := p.f.Complete(ctx, fi)
				ra.got[i] <- completed{whole, err}
			}()
		}
	}()
	return ra
}

// take is issue i as read, and frees its slot.
func (ra *readAhead) take(i int) (forge.Issue, error) {
	if ra.direct != nil {
		return ra.direct(i)
	}
	c := <-ra.got[i]
	<-ra.slots
	return c.is, c.err
}

// stop drops the reads not taken: the pull ended, or failed.
func (ra *readAhead) stop() { ra.cancel() }

// reached journals a forge issue the pull finished, for a run that goes
// on after this one is cut.
func (r *run) reached(p *project, fi forge.Issue, reached bool) {
	if reached && r.opt.Issue == "" {
		r.j.Reached(p.key, fi.ID, r.now)
	}
}

// pullOne pulls a listed forge issue. A mirror the fork posted waits for
// every original to be pulled, unless final. reached says it is pulled: not
// waiting, nor skipped.
func (r *run) pullOne(p *project, fi forge.Issue, final bool) (reached bool, _ error) {
	id, err := r.match(p, fi, final)
	if err != nil {
		return false, err
	}
	if r.opt.Issue != "" && id != r.opt.Issue {
		r.report.Unchanged++
		return false, nil
	}
	switch id {
	case "":
		r.report.Unchanged++
		return false, nil
	case "later":
		r.later = append(r.later, listed{p, fi})
		return false, nil
	case "new":
		id, err := r.pullNew(p, fi)
		if err != nil {
			return false, err
		}
		if w := bsync.Unwrap(fi.Body); w.ForkID != "" && r.isSync(p, fi.Author) {
			r.forkIDs[w.ForkID] = id
		}
		r.report.Created++
		return true, nil
	}
	if r.issues[id].Deleted {
		// deleted here: its forge issue stays linked to it, so that no pull
		// brings it back, and nothing goes either way (docs/adr/0019)
		r.report.Unchanged++
		return true, nil
	}
	wrote, _, pulled, err := r.reconcile(p, id, fi, true, false)
	if err != nil {
		return false, err
	}
	if wrote {
		r.report.Updated++
		r.did(p.key, "updated", id, pulled)
	} else {
		r.report.Unchanged++
	}
	return true, nil
}

// progress tells Progress where a way is, the issue named by its number
// on the forge, when it has one, and its title.
func (r *run) progress(key, way string, done, total int, number, title string) {
	if r.opt.Progress == nil {
		return
	}
	name := title
	if number != "" {
		name = "#" + number + " " + title
	}
	r.opt.Progress(key, way, done, total, name)
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
		w := bsync.Unwrap(fi.Body)
		mine := w.Kind == "issue" && w.ID == id && r.isSync(p, fi.Author)
		from := is != nil && is.Origin != nil && *is.Origin == kernel.Origin{Forge: p.key, Event: "issue:" + fi.ID}
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
	from := kernel.Origin{Forge: p.key, Event: "issue:" + fi.ID}
	for _, id := range slices.Sorted(maps.Keys(r.issues)) { // ponytail: a scan per unlinked forge issue
		is := r.issues[id]
		if is.Origin != nil && *is.Origin == from && !p.st.anyLink[id] {
			r.did(p.key, "linked", id, 0)
			return id, p.st.write(r.ctx, r.s, p.key, []kernel.Op{linkOp(is.Author, r.now, id, fi.ID, fi.Number, fi.URL)})
		}
	}
	return r.mirror(p, fi, final)
}

// byMarker links an unlinked forge issue whose marker, from sync's
// accounts, names an issue: a push stopped before its link. Last seen is
// what the push sent. ok says it has such a marker; id is "" when its
// issue isn't here, or is linked already.
func (r *run) byMarker(p *project, fi forge.Issue) (id string, ok bool, err error) {
	w := bsync.Unwrap(fi.Body)
	if w.Kind != "issue" || !r.isSync(p, fi.Author) {
		return "", false, nil
	}
	is, here := r.issues[w.ID]
	if !here && r.opt.ForgeOnly {
		return "", false, nil // its id never arrives: pulled as new (ADR 0002)
	}
	if !here || p.st.anyLink[w.ID] {
		return "", true, nil
	}
	title, body, err := r.sentBefore(p, is, fi)
	if err != nil {
		return "", true, err
	}
	ops := append([]kernel.Op{linkOp(is.Author, r.now, is.ID, fi.ID, fi.Number, fi.URL)}, createdSeen(is.Author, r.now, is.ID, title, body)...)
	r.did(p.key, "linked", is.ID, 0)
	return is.ID, true, p.st.write(r.ctx, r.s, p.key, ops)
}

// sentBefore is the title and body the create of fi sent, for a push
// stopped before its link. The issue may have changed here since, so each
// is the forge's when some earlier state of the issue sends it (the forge
// still shows what the create sent), else what the issue sends now (the
// forge changed it since).
func (r *run) sentBefore(p *project, is *Issue, fi forge.Issue) (title, body string, err error) {
	_, direct := r.writer(p, is.Author)
	title, body = r.sent(p, is, direct)
	if title == fi.Title && body == fi.Body {
		return title, body, nil
	}
	log, _, err := issues.Kind.Log(r.ctx, r.s, kernel.ID(is.ID))
	if err != nil {
		return "", "", err
	}
	var sentTitle, sentBody bool
	for k := len(log) - 1; k > 0 && !(sentTitle && sentBody); k-- {
		was, err := issueFold(is.ID, log[:k])
		if err != nil {
			return "", "", err
		}
		t, b := r.sent(p, was, direct)
		sentTitle = sentTitle || t == fi.Title
		sentBody = sentBody || b == fi.Body
	}
	if sentTitle {
		title = fi.Title
	}
	if sentBody {
		body = fi.Body
	}
	return title, body, nil
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
	w := bsync.Unwrap(fi.Body)
	trusted := w.Forked() && r.isSync(p, fi.Author)
	places := slices.Clone(r.mirrors[strings.ToLower(p.key+"#"+fi.Number)])
	switch {
	case !trusted && len(places) == 0:
		return "new", nil
	case w.Forked() && !final:
		return "later", nil
	}
	var found []string
	if trusted {
		found = append(found, r.linkedAt(p, w.Link), r.forkIDs[w.ForkID])
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
			if trusted && w.ForkID != "" {
				r.forkIDs[w.ForkID] = id
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
	return p.st.write(r.ctx, r.s, p.key, []kernel.Op{linkOp(a, r.now, id, fi.ID, fi.Number, fi.URL),
		seenOp(a, r.now, id, "title", fi.Title), seenOp(a, r.now, id, "body", fi.Body), seenOp(a, r.now, id, "status", fi.Status)})
}

// createdSeen are the seen values of a forge issue sync just created.
func createdSeen(author string, now int64, id, title, body string) []kernel.Op {
	return []kernel.Op{
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
	if p.way == bsync.Push {
		// a backup: what changed there since its last run is read whole, so
		// the plan sees it and puts the board's values back
		for _, fi := range p.list {
			if _, linked := p.st.links[fi.ID]; !linked {
				continue
			}
			whole, err := p.f.Complete(r.ctx, fi)
			if errors.Is(err, forge.ErrNotFound) || errors.Is(err, forge.ErrConfidential) {
				continue
			}
			if err != nil {
				return fmt.Errorf("syncer: forge issue #%s: %w", fi.Number, err)
			}
			p.listed[whole.ID] = whole
		}
	}
	var todo []*Issue
	for _, id := range ids {
		if is, ok := r.issues[id]; ok && r.mayCreate(p, is) {
			todo = append(todo, is)
		}
	}
	done, total := 0, len(ids)
	if len(todo) > 0 && (r.opt.Create == nil || r.opt.Create(p.key, len(todo), p.first)) {
		total += len(todo)
		for _, is := range todo {
			done++
			r.progress(p.key, "push", done, total, "", is.Title)
			if err := r.create(p, is); err != nil {
				return err
			}
		}
	}
	for _, id := range ids {
		done++
		if is, ok := r.issues[id]; ok {
			r.progress(p.key, "push", done, total, p.st.link[id].Number, is.Title)
		}
		l, ok := p.st.link[id]
		if is, here := r.issues[id]; !ok || !here || is.Deleted {
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
		_, pushed, _, err := r.reconcile(p, id, fi, p.pull, true)
		switch {
		case pushed && p.putBack[id]:
			r.report.Restored++
			r.did(p.key, "restored", id, 0)
		case pushed:
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
func (r *run) mayCreate(p *project, is *Issue) bool {
	if is.Deleted || p.st.anyLink[is.ID] || p.st.refused[[3]string{is.ID, "issue", ""}] {
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
func (r *run) sent(p *project, is *Issue, direct bool) (title, body string) {
	w := bsync.Wrapped{Body: r.form(p, is.Body, origin(is.Changes["body"]), false), Marker: bsync.MarkerOf("issue", is.ID, false, "")}
	if !direct {
		w.Header = r.header(is.Author, is.Created, is.Origin, is.ID)
	}
	return r.form(p, is.Title, origin(is.Changes["title"]), true), w.String()
}

// create makes the issue on p with its title and body, and links it.
func (r *run) create(p *project, is *Issue) error {
	fw, direct := r.writer(p, is.Author)
	title, body := r.sent(p, is, direct)
	fi, err := fw.CreateIssue(r.ctx, title, body)
	if errors.Is(err, forge.ErrRefused) {
		return r.refuse(p, is, "issue", "", err)
	}
	if err != nil {
		return err
	}
	ops := append([]kernel.Op{linkOp(is.Author, time.Now().Unix(), is.ID, fi.ID, fi.Number, fi.URL)},
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
func (r *run) refuse(p *project, is *Issue, field, value string, why error) error {
	if p.st.refused[[3]string{is.ID, field, value}] {
		return nil
	}
	reason := why.Error()
	if err := p.st.write(r.ctx, r.s, p.key, []kernel.Op{refusedOp(is.Author, time.Now().Unix(), is.ID, field, value, reason)}); err != nil {
		return err
	}
	shown := value
	if len(shown) > 60 {
		shown = shown[:60] + "…"
	}
	r.warn(fmt.Sprintf("%s refused %s %q of issue %s: %s", p.key, field, shown, is.ID, reason))
	return nil
}
