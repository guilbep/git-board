package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/guilbep/git-board/next/app"
	"github.com/guilbep/git-board/next/forge"
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
	Report    *report   `json:"report,omitempty"`
	Error     string    `json:"error,omitempty"`
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

type report struct {
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	Posted    int `json:"posted"`
	Pushed    int `json:"pushed"`
}

// maxSteps is how many steps a run keeps: the progress events carry
// them all.
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
		res.Forges = append(res.Forges, forgeRes{Name: t.Name, Kind: t.Kind, Project: t.Project, Host: t.Host, Login: t.Login, Sync: t.Sync, Discovered: t.Discovered})
	}
	if res.Remotes, err = b.Remotes(ctx); res.Remotes == nil {
		res.Remotes = []string{}
	}
	return res, err
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
	return &r
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
	if r.Way == "" {
		r.Way = string(app.WaySync)
	}
	switch {
	case r.Scope != "forges" && r.Scope != "all":
		return Reply{}, invalid("scope", fmt.Errorf("scope %q: forges or all", r.Scope))
	case r.Way != "sync" && r.Way != "pull" && r.Way != "push":
		return Reply{}, invalid("way", fmt.Errorf("way %q: sync, pull or push", r.Way))
	case r.Scope == "all" && (r.Way != "sync" || r.DryRun):
		return Reply{}, invalid("way", errors.New("a whole sync goes both ways, and has no dry run: scope forges has"))
	}
	s.runMu.Lock()
	if s.run != nil && s.run.State == "running" {
		s.runMu.Unlock()
		return Reply{}, &app.Error{Class: app.Busy, Err: errors.New("a sync runs: try again once it ends")}
	}
	s.runs++
	run := &Run{ID: strconv.Itoa(s.runs), Scope: r.Scope, Way: r.Way, DryRun: r.DryRun, StartedAt: time.Now(), State: "running", Steps: []Step{}}
	s.run = run
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
		if err != nil {
			s.log.Error("sync failed", "run", run.ID, "code", app.Code(err), "error", run.Error)
		} else {
			s.log.Info("sync ended", "run", run.ID, "took", run.EndedAt.Sub(run.StartedAt).Round(time.Millisecond))
		}
		s.publish(ctx, Event{Kind: "sync", ID: run.ID, By: by})
	}()
	s.publish(ctx, Event{Kind: "sync", ID: run.ID, By: by})
	return Reply{Status: http.StatusAccepted, Body: run}, nil
}

// step records a run's step, and tells the pages.
func (s *Server) step(ctx context.Context, run *Run, st Step) {
	s.runMu.Lock()
	if len(run.Steps) < maxSteps && st.What != "progress" { // a count: the events carry it
		run.Steps = append(run.Steps, st)
	}
	s.runMu.Unlock()
	switch st.What {
	case "warning", "left", "held":
		s.log.LogAttrs(ctx, slog.LevelWarn, "sync", slog.String("run", run.ID), slog.String("forge", st.Forge), slog.String("what", st.What), slog.String("line", app.NoCredentials(st.Line)))
	}
	if st.State == "failed" {
		s.log.LogAttrs(ctx, slog.LevelWarn, "sync step failed", slog.String("run", run.ID), slog.String("step", st.Name), slog.String("line", app.NoCredentials(st.Line)))
	}
	s.publish(ctx, Event{Kind: "progress", ID: run.ID, Issue: st.Issue, Step: &st, By: clientOf(ctx)})
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
		rep = &report{Created: got.Created, Updated: got.Updated, Unchanged: got.Unchanged, Posted: got.Posted, Pushed: got.Pushed}
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
	return res, err
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
