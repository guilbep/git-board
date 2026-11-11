package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/guilbep/git-board/app"
	"github.com/guilbep/git-board/forge"
)

// Sync: the forges' runs, in the background, and the git remotes, at
// once (docs/rewrite/api.md, Sync).

// Run is one sync run.
type Run struct {
	ID        string    `json:"id"`
	Scope     string    `json:"scope"` // forges or all
	Way       string    `json:"way"`   // sync, pull or push
	DryRun    bool      `json:"dry_run"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitzero"`
	State     string    `json:"state"` // running, done, failed
	Steps     []Step    `json:"steps"`
	Tallies   []Tally   `json:"tallies"`
	Report    *report   `json:"report,omitempty"`
	Error     string    `json:"error,omitempty"`
	told      time.Time // the last count told
}

// Step is one thing a run did: a whole sync's step (pull, forges,
// push), or a forge's event.
type Step struct {
	Name  string `json:"name,omitempty"`  // a whole sync's step
	State string `json:"state,omitempty"` // running, done, skipped, failed
	Forge string `json:"forge,omitempty"`
	Way   string `json:"way,omitempty"`
	What  string `json:"what,omitempty"`
	Issue string `json:"issue,omitempty"`
	Line  string `json:"line,omitempty"`
	Done  int    `json:"done,omitempty"`
	Total int    `json:"total,omitempty"`
}

// Tally is how many of one thing a run did on one forge, one way: a
// write (create-comment, set-status…), an issue pulled or pushed, or a
// warning of one reason. A check of 1,000 issues is a few tallies, not
// 5,000 steps.
type Tally struct {
	Forge    string `json:"forge,omitempty"`
	Way      string `json:"way,omitempty"`
	What     string `json:"what"`
	N        int    `json:"n"`
	Examples []Step `json:"examples"` // the first few, as they came
}

// maxExamples is how many steps a tally keeps.
const maxExamples = 5

// tallied says whether a run's step goes into a tally, not the steps: a
// write, an issue's change, a warning. The steps keep what a person reads
// whole: notes, holds, a forge left out, waits, a whole sync's steps.
func tallied(st Step) bool {
	switch st.What {
	case "", "progress", "note", "held", "left", "waiting":
		return false
	}
	return st.Name == ""
}

var (
	quoted = regexp.MustCompile(`"[^"]*"`)
	hexID  = regexp.MustCompile(`\b[0-9a-f]{7,40}\b`)
)

// tallyOf is the tally a step counts in: a write by its operation, a
// warning by its reason (its ids and quoted values masked).
func tallyOf(st Step) (way, what string) {
	switch {
	case st.What == "warning":
		return st.Way, "warning: " + hexID.ReplaceAllString(quoted.ReplaceAllString(st.Line, `"…"`), "…")
	case st.Line != "" && st.What != "release":
		return st.Way, st.Line // a write's operation
	}
	return st.Way, st.What
}

// tally counts st in run's tallies.
func (run *Run) tally(st Step) {
	way, what := tallyOf(st)
	for i := range run.Tallies {
		t := &run.Tallies[i]
		if t.Forge == st.Forge && t.Way == way && t.What == what {
			t.N++
			if len(t.Examples) < maxExamples {
				t.Examples = append(t.Examples, st)
			}
			return
		}
	}
	run.Tallies = append(run.Tallies, Tally{Forge: st.Forge, Way: way, What: what, N: 1, Examples: []Step{st}})
}

type report struct {
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	Posted    int `json:"posted"`
	Pushed    int `json:"pushed"`
	Restored  int `json:"restored"` // a backup's own edits, put back
}

// maxSteps is how many steps a run keeps; the writes and the issues'
// changes are tallied instead (Tally).
const maxSteps = 500

type syncRes struct {
	Forges  []forgeRes `json:"forges"`
	Remotes []string   `json:"remotes"`
	Run     *Run       `json:"run"`
}

type forgeRes struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Project    string `json:"project"`
	Host       string `json:"host"`
	Login      string `json:"login,omitempty"`
	Sync       bool   `json:"sync"`
	Way        string `json:"way"` // push (a backup), pull (read-only), or both
	Discovered bool   `json:"discovered,omitempty"`
}

func (s *Server) syncStatus(ctx context.Context, _ none) (syncRes, error) {
	b := board(ctx)
	res := syncRes{Forges: []forgeRes{}, Run: s.lastRun()}
	tables, err := b.Forges(ctx, func(string) {})
	if err != nil {
		return res, err
	}
	for _, t := range tables {
		res.Forges = append(res.Forges, forgeRes{Name: t.Name, Kind: t.Kind, Project: t.Project, Host: t.Host, Login: t.Login, Sync: t.Sync, Way: t.Direction(), Discovered: t.Discovered})
	}
	if res.Remotes, err = b.Remotes(ctx); res.Remotes == nil {
		res.Remotes = []string{}
	}
	return res, err
}

// state is route 60: the board's state, named (app.State), with this
// server's own run, which it knows first.
func (s *Server) state(ctx context.Context, _ none) (app.State, error) {
	st, err := board(ctx).State(ctx)
	if r := s.lastRun(); r != nil && r.State == "running" {
		st.Run = app.RunState{State: "running"}
	}
	return st, err
}

// lastRun is a copy of the run running, or the last one.
func (s *Server) lastRun() *Run {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.run == nil {
		return nil
	}
	r := *s.run
	r.Steps = slices.Clone(r.Steps)
	r.Tallies = cloneTallies(r.Tallies)
	return &r
}

func cloneTallies(ts []Tally) []Tally {
	out := slices.Clone(ts)
	for i := range out {
		out[i].Examples = slices.Clone(out[i].Examples)
	}
	return out
}

type syncReq struct {
	Scope       string   `json:"scope"`
	Way         string   `json:"way"`
	Forges      []string `json:"forges"`
	DryRun      bool     `json:"dry_run"`
	AllowCreate bool     `json:"allow_create"`
	Remote      string   `json:"remote"`
}

// startSync starts a run, which goes on after the request: it holds a
// board of its own until it ends.
func (s *Server) startSync(ctx context.Context, r syncReq) (Reply, error) {
	if r.Scope == "" {
		r.Scope = "forges"
	}
	if r.Way == "" || r.Way == "both" { // the forge table's word for it
		r.Way = string(app.WaySync)
	}
	switch {
	case r.Scope != "forges" && r.Scope != "all":
		return Reply{}, invalid("scope", fmt.Errorf("scope %q: forges or all", r.Scope))
	case r.Way != "sync" && r.Way != "pull" && r.Way != "push":
		return Reply{}, invalid("way", fmt.Errorf("way %q: both (or sync), pull or push", r.Way))
	case r.Scope == "all" && (r.Way != "sync" || r.DryRun):
		return Reply{}, invalid("way", errors.New("a whole sync goes both ways, and has no dry run: scope forges has"))
	}
	if board(ctx).Syncing() { // this server's run, the CLI's, or another server's
		return Reply{}, &app.Error{Class: app.Busy, Err: errors.New("a sync runs: try again once it ends")}
	}
	s.runMu.Lock()
	if s.run != nil && s.run.State == "running" {
		s.runMu.Unlock()
		return Reply{}, &app.Error{Class: app.Busy, Err: errors.New("a sync runs: try again once it ends")}
	}

	s.runs++
	run := &Run{ID: strconv.Itoa(s.runs), Scope: r.Scope, Way: r.Way, DryRun: r.DryRun, StartedAt: time.Now(), State: "running", Steps: []Step{}, Tallies: []Tally{}}
	s.run = run
	started := *run // the reply's: the run's own goes on changing as it runs
	s.runMu.Unlock()

	h := s.boards.get()
	by := clientOf(ctx)
	go func() {
		defer s.boards.release(h)
		ctx := context.WithValue(s.base, logKey{}, s.log)
		ctx = context.WithValue(ctx, clientKey{}, by)
		s.log.Info("sync started", "run", run.ID, "scope", run.Scope, "way", run.Way, "dry_run", run.DryRun)
		rep, err := s.runSync(ctx, h.b, run, r)
		s.runMu.Lock()
		run.EndedAt, run.State = time.Now(), "done"
		if rep != nil {
			run.Report = rep
		}
		if err != nil {
			run.State, run.Error = "failed", app.NoCredentials(err.Error())
		}
		s.runMu.Unlock()
		s.pulls.Stale() // the branches and the forge moved: each PR is read again
		if err != nil {
			s.log.Error("sync failed", "run", run.ID, "code", app.Code(err), "error", run.Error)
		} else {
			s.log.Info("sync ended", "run", run.ID, "took", run.EndedAt.Sub(run.StartedAt).Round(time.Millisecond))
		}
		s.publish(ctx, Event{Kind: "sync", ID: run.ID, By: by})
	}()
	s.publish(ctx, Event{Kind: "sync", ID: run.ID, By: by})
	return Reply{Status: http.StatusAccepted, Body: &started}, nil
}

// progressEvery is how often a run's counts and tallies are told: a page
// falls behind on one event an issue.
var progressEvery = 250 * time.Millisecond

// step records a run's step, and tells the pages: a step a person reads
// at once; a count (progress) or a tallied step at most every
// progressEvery, with the tallies, and the last count.
func (s *Server) step(ctx context.Context, run *Run, st Step) {
	s.runMu.Lock()
	var tallies []Tally
	switch {
	case st.What == "progress" || tallied(st):
		if tallied(st) {
			run.tally(st)
		}
		now := time.Now()
		last := st.What == "progress" && st.Done >= st.Total
		if now.Sub(run.told) < progressEvery && !last {
			s.runMu.Unlock()
			return
		}
		run.told = now
		tallies = cloneTallies(run.Tallies)
		if st.What != "progress" {
			st = Step{} // the tallies say it
		}
	case len(run.Steps) < maxSteps:
		run.Steps = append(run.Steps, st)
	}
	s.runMu.Unlock()
	switch st.What {
	case "left", "held":
		s.log.LogAttrs(ctx, slog.LevelWarn, "sync", slog.String("run", run.ID), slog.String("forge", st.Forge), slog.String("what", st.What), slog.String("line", app.NoCredentials(st.Line)))
	}
	if st.State == "failed" {
		s.log.LogAttrs(ctx, slog.LevelWarn, "sync step failed", slog.String("run", run.ID), slog.String("step", st.Name), slog.String("line", app.NoCredentials(st.Line)))
	}
	e := Event{Kind: "progress", ID: run.ID, Issue: st.Issue, Tallies: tallies, By: clientOf(ctx)}
	if st != (Step{}) {
		e.Step = &st
	}
	s.publish(ctx, e)
}

func (s *Server) runSync(ctx context.Context, b *app.Board, run *Run, r syncReq) (*report, error) {
	var mu sync.Mutex
	var rep *report
	see := func(e app.SyncEvent) {
		s.step(ctx, run, Step{Forge: e.Forge, Way: e.Way, What: e.What, Issue: e.Issue, Line: e.Line, Done: e.Done, Total: e.Total})
	}
	forges := func(ctx context.Context, tables []forge.Table) error {
		fs, err := b.OpenForges(ctx, tables)
		if err != nil {
			return err
		}
		got, err := b.SyncForges(ctx, app.Way(r.Way), app.ForgeRun{Forges: fs, AllowCreate: r.AllowCreate, DryRun: r.DryRun, See: see})
		mu.Lock()
		rep = &report{Created: got.Created, Updated: got.Updated, Unchanged: got.Unchanged, Posted: got.Posted, Pushed: got.Pushed, Restored: got.Restored}
		mu.Unlock()
		return err
	}
	say := func(line string) { s.step(ctx, run, Step{What: "note", Line: line}) }
	tables, err := b.Forges(ctx, say)
	if err != nil {
		return nil, err
	}
	if len(r.Forges) > 0 {
		tables = slices.DeleteFunc(tables, func(t forge.Table) bool { return !slices.Contains(r.Forges, t.Name) })
		if len(tables) == 0 {
			return nil, errUsage(fmt.Errorf("no forge named %v", r.Forges))
		}
	}
	if r.Scope == "forges" {
		if len(tables) == 0 {
			return nil, errUsage(errors.New("no forge: name one in .git-board.toml [forges], or add a remote on one"))
		}
		return rep, forges(ctx, tables)
	}
	w := app.Whole{Remote: cmpString(r.Remote, "origin"), Step: func(name, state, note string) {
		s.step(ctx, run, Step{Name: name, State: state, Line: note})
	}}
	if slices.ContainsFunc(tables, func(t forge.Table) bool { return t.Sync }) {
		w.Forges = func(ctx context.Context) error { return forges(ctx, tables) }
	}
	err = b.SyncWhole(ctx, w)
	mu.Lock()
	defer mu.Unlock()
	return rep, err
}

type remoteReq struct {
	Name   string `path:"name"`
	Action string `path:"action"`
}

type remoteRes struct {
	Remote string     `json:"remote"`
	Action string     `json:"action"`
	Note   string     `json:"note,omitempty"`
	Issues *app.Ahead `json:"issues,omitempty"`
	People *app.Ahead `json:"people,omitempty"`
}

func remote(ctx context.Context, r remoteReq) (remoteRes, error) {
	b := board(ctx)
	res := remoteRes{Remote: r.Name, Action: r.Action}
	var err error
	switch r.Action {
	case "fetch":
		var is, ps app.Ahead
		is, ps, err = b.Fetch(ctx, r.Name)
		res.Issues, res.People = &is, &ps
	case "pull":
		res.Note, err = b.Pull(ctx, r.Name)
	case "push":
		res.Note, err = b.Push(ctx, r.Name)
	default:
		return res, invalid("action", fmt.Errorf("%q: fetch, pull or push", r.Action))
	}
	if b.PRs != nil && r.Action != "push" {
		b.PRs.Stale() // the branches moved: each PR is read again
	}
	return res, err
}

// addRemoteReq adds a git remote, from the Sync page: a name in the
// path, its URL in the body.
type addRemoteReq struct {
	Name string `path:"name"`
	URL  string `json:"url"`
}

// addRemote is the git remote the sync steps want, added from the web
// app. It answers 204: the page reads GET /api/sync again.
func (s *Server) addRemote(ctx context.Context, r addRemoteReq) (Reply, error) {
	if err := board(ctx).AddRemote(ctx, r.Name, strings.TrimSpace(r.URL)); err != nil {
		return Reply{}, on("url", err)
	}
	return Reply{Status: http.StatusNoContent}, nil
}

type numbersReq struct {
	Prefix string `json:"prefix"`
	Start  int    `json:"start"`
}

type numbersRes struct {
	Where    string `json:"where,omitempty"`
	Numbered int    `json:"numbered"`
}

func numbers(ctx context.Context, r numbersReq) (numbersRes, error) {
	b := board(ctx)
	if r.Prefix != "" || r.Start != 0 {
		where, err := b.InitNumbers(ctx, r.Prefix, r.Start)
		return numbersRes{Where: where}, on("prefix", err)
	}
	n, err := b.Number(ctx)
	return numbersRes{Numbered: n}, err
}

type keptRes struct {
	Name    string `json:"name"`
	Where   string `json:"where"`
	Replica bool   `json:"replica"`
	Counter bool   `json:"counter"`
}

func datastore(ctx context.Context, _ none) (keptRes, error) {
	b := board(ctx)
	k, err := app.Datastore(ctx, b.Dir)
	return keptRes{Name: b.DatastoreName(), Where: k.Where, Replica: k.Replica, Counter: k.Counter}, err
}

type copyReq struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type copiedRes struct {
	To       string `json:"to"`
	Entities int    `json:"entities"`
	Counter  bool   `json:"counter"`
}

func copyDatastore(ctx context.Context, r copyReq) (copiedRes, error) {
	c, err := app.CopyDatastore(ctx, board(ctx).Dir, r.From, r.To)
	return copiedRes{To: c.To, Entities: c.Entities, Counter: c.Counter}, on("to", err)
}

// putDatastoreReq is the datastore this clone keeps its board in, as
// board.datastore in its own git config: "" or "git" for git, else the
// value, a postgres:// URL or markdown:<dir>. A loopback request alone:
// a URL can hold a password, and the key is the clone's own.
type putDatastoreReq struct {
	Value string `json:"value"`
}

// putDatastore is route 28: the datastore choice, made from the web app.
// The value is opened first, so a name no datastore understands is
// refused and the clone's board still opens; the board then opens again,
// and the pages hear a config event. No restart, no terminal.
func (s *Server) putDatastore(ctx context.Context, r putDatastoreReq) (keptRes, error) {
	if err := board(ctx).SetDatastore(ctx, strings.TrimSpace(r.Value)); err != nil {
		return keptRes{}, on("value", err)
	}
	if err := s.boards.reopen(ctx); err != nil {
		return keptRes{}, err
	}
	s.publish(ctx, Event{Kind: "config", By: clientOf(ctx)})
	h := s.boards.get()
	defer s.boards.release(h)
	return datastore(context.WithValue(ctx, boardKey{}, h.b), none{})
}
