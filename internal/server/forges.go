package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
)

// dryRunFresh is how long a dry run stands for the real run it allows.
const dryRunFresh = 15 * time.Minute

// maxLines caps the lines kept per bridge.
const maxLines = 1000

var errDryRunFirst = errors.New("check first: a run writes, the check (a dry run) says what it would do")

// A bridge is a forge of .git-board.toml ([forges.<name>]), by its name.

// syncBridge is a forge taking part in the sync.
type syncBridge struct {
	Name       string `json:"name"`
	Target     string `json:"target"`               // github, gitlab
	Discovered bool   `json:"discovered,omitempty"` // found in the git remotes, with no table
}

// forgeStatus is the sync with the forges: which bridges take part, and
// the last run.
type forgeStatus struct {
	Bridges []syncBridge `json:"bridges"`
	Job     *forgeJob    `json:"job"`
}

// the modes of a forge run: both ways, push alone (send, the Work page's
// Send all), or pull alone (get). A dry run looks both ways. The whole
// sync's job is mode all (whole.go).
var forgeModes = []string{"sync", "send", "get"}

// forgeJob is one run of the forge sync, in the background: a co2 dry run
// took over three minutes.
type forgeJob struct {
	DryRun  bool                    `json:"dry_run"`
	Mode    string                  `json:"mode"`  // sync, send, get; all: the whole sync
	Names   []string                `json:"names"` // the bridges of the run
	State   string                  `json:"state"` // running, done, failed
	Started time.Time               `json:"started"`
	Ended   *time.Time              `json:"ended,omitempty"`
	Bridges map[string]*bridgeTally `json:"bridges"`
	Errors  int                     `json:"errors"`
	Error   string                  `json:"error,omitempty"` // the run couldn't start or failed
	// AllowCreate is the run's consent to the creations a check held
	// (store.ForgeRun.AllowCreate): the page's "I understand".
	AllowCreate bool `json:"allow_create"`
	// Read are the forges a check read, its names without those left
	// out: the only ones a real run after it syncs.
	Read  []string  `json:"read,omitempty"`
	Step  *syncStep `json:"step,omitempty"`  // where it is, for a page loading mid-run
	Steps []jobStep `json:"steps,omitempty"` // the whole sync's, in order
}

// fresh says job is a clean check of the last dryRunFresh, which a real
// run needs: one of the whole sync, for a whole run. A check of the whole
// sync is a forge check too.
func (j *forgeJob) fresh(whole bool) error {
	if j == nil || !j.DryRun || j.State != "done" || j.Ended == nil || time.Since(*j.Ended) > dryRunFresh || (whole && j.Mode != "all") {
		return errDryRunFirst
	}
	if j.Errors > 0 {
		return errors.New("the check had errors: fix them and check again")
	}
	return nil
}

// tally is a bridge's, made on first use; forgeMu held.
func (j *forgeJob) tally(name string) *bridgeTally {
	t := j.Bridges[name]
	if t == nil {
		t = &bridgeTally{Lines: []string{}}
		j.Bridges[name] = t
	}
	return t
}

// syncStep is where a run is: the bridge, the way, how many issues of how
// many it has come to, and the issue it is on.
type syncStep struct {
	Bridge string `json:"bridge"`
	Way    string `json:"way"` // pull, push
	Done   int    `json:"done"`
	Total  int    `json:"total,omitempty"`
	Issue  string `json:"issue,omitempty"` // "#43 import: users lose their work"
}

// names says an event names an issue of git-board's.
func names(e store.SyncEvent) bool {
	switch e.What {
	case "created", "updated", "linked", "posted", "pushed":
		return true
	}
	return false
}

// issueName labels an issue in a step: its project number, else its forge
// number, else its short id, and its title. One a dry run would pull is
// only in the dry run's copy: its short id.
func (s *apiServer) issueName(id string) string {
	short := id[:min(7, len(id))]
	i, err := s.b.GetIssue(id)
	if err != nil {
		return short
	}
	n := i.Number
	if n == "" {
		n = short
		if f := i.BareNumber(); f != "" {
			n = "#" + f
		}
	}
	title := i.Shown
	if title == "" {
		title = i.Title
	}
	return n + " " + title
}

// bridgeTally counts what a run did, or would do (a dry run), on one
// bridge, both ways.
type bridgeTally struct {
	To   direction `json:"to"`   // git-board → forge
	From direction `json:"from"` // forge → git-board
	// Linked are issues the run found on the forge by their marker, or
	// their origin: created by a run that stopped before its link, now
	// linked rather than created again. Only a pull links them.
	Linked int `json:"linked"`
	// Assignable is how many people the forge says can be assigned,
	// asked after the run (assignable.go)
	Assignable int `json:"assignable"`
	Warnings   int `json:"warnings"`
	Errors     int `json:"errors"`
	// Held are the creations waiting for consent: on a forge never synced
	// with, or MaxCreate or more in one run (store.ForgeRun.AllowCreate).
	Held int `json:"held"`
	// Left is why the forge was left out of the run: it couldn't be read.
	Left  string   `json:"left,omitempty"`
	Lines []string `json:"lines"`
}

// direction is one way of a sync.
type direction struct {
	Issues   int `json:"issues"`  // created on the forge, or pulled from it
	Changed  int `json:"changed"` // issues already linked with changes
	Comments int `json:"comments"`
	Edits    int `json:"edits"`
	// Changes, to a forge, are titles, states, labels and assignees; from
	// one, every operation pulled
	Changes int `json:"changes"`
}

// add counts an event, and keeps its line.
func (t *bridgeTally) add(e store.SyncEvent, line string) {
	switch e.What {
	case "created":
		t.From.Issues++
		t.From.Changes += e.Ops
	case "updated":
		t.From.Changed++
		t.From.Changes += e.Ops
	case "linked":
		t.Linked++
	case "posted":
		t.To.Issues++
	case "pushed":
		t.To.Changed++
	case "comment":
		t.To.Comments++
	case "edit":
		t.To.Edits++
	case "change":
		t.To.Changes++
	case "warning":
		t.Warnings++
	case "held":
		t.Held += e.Ops
	case "left":
		t.Left = e.Line
		t.Warnings++
	}
	if line != "" && len(t.Lines) < maxLines {
		t.Lines = append(t.Lines, line)
	}
}

// tell is a run's events counted into tally, and its steps published:
// tally is called with forgeMu held. id names the issue a step is on, for
// its drawer; "" for a full run.
func (s *apiServer) tell(dry bool, id string, tally func(name string) *bridgeTally, step func(*syncStep)) func(store.SyncEvent) {
	return func(e store.SyncEvent) {
		name := ""
		if names(e) {
			name = s.issueName(e.Issue)
		}
		text := e.Text(dry, name)
		next, moved := syncStep{Bridge: e.Forge, Way: e.Way, Done: e.Done, Total: e.Total, Issue: e.Line}, e.What == "progress"
		s.forgeMu.Lock()
		if e.Forge != "" {
			tally(e.Forge).add(e, text)
		}
		if moved && step != nil {
			step(&next)
		}
		s.forgeMu.Unlock()
		if moved {
			s.events.publish(event{Kind: "progress", ID: id, Step: &next})
		}
	}
}

// forges are the settings' forges, or with no table, those found in the
// git remotes (Backend.Forges), their GitLab ids read by each run.
// ponytail: a URL that names no forge isn't told; the CLI's runs tell it
func (s *apiServer) forges() []config.Forge {
	fs, err := s.b.Forges(s.config(), func(string) {})
	if err != nil {
		log.Printf("the forges of the git remotes: %v", err)
	}
	return fs
}

// syncForges are the forges taking part in the sync.
func (s *apiServer) syncForges() []config.Forge {
	return slices.DeleteFunc(slices.Clone(s.forges()), func(f config.Forge) bool { return !f.Sync })
}

func (s *apiServer) forgeSnapshot() forgeStatus {
	bridges := []syncBridge{}
	for _, f := range s.syncForges() {
		bridges = append(bridges, syncBridge{Name: f.Name, Target: f.Kind, Discovered: f.Discovered})
	}
	return forgeStatus{Bridges: bridges, Job: s.job}
}

func (s *apiServer) handleForges(w http.ResponseWriter, r *http.Request) {
	s.forgeMu.Lock()
	defer s.forgeMu.Unlock()
	writeJSON(w, http.StatusOK, s.forgeSnapshot())
}

// handleForgeSync starts a forge sync: a dry run, or a real run right
// after one (within dryRunFresh), so a real run is always one the user saw
// the plan of. A real run goes both ways, or sends alone.
func (s *apiServer) handleForgeSync(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DryRun      bool     `json:"dry_run"`
		Mode        string   `json:"mode"`         // sync (empty), send, get
		Bridges     []string `json:"bridges"`      // a real run: some of the dry run's; empty: all
		AllowCreate bool     `json:"allow_create"` // consent to the creations the check held
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Mode == "" {
		req.Mode = "sync"
	}
	if !slices.Contains(forgeModes, req.Mode) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("mode %q: want sync, send or get", req.Mode))
		return
	}
	s.forgeMu.Lock()
	defer s.forgeMu.Unlock()
	if s.syncing.Load() {
		writeCoded(w, http.StatusConflict, "syncing", errSyncing)
		return
	}
	last := s.job
	if !req.DryRun {
		if err := last.fresh(false); err != nil {
			writeError(w, http.StatusConflict, err)
			return
		}
		for _, name := range req.Bridges {
			if !slices.Contains(last.Names, name) {
				writeError(w, http.StatusBadRequest, errors.New("bridge "+name+" wasn't in the check"))
				return
			}
		}
	} else {
		req.Bridges, req.Mode, req.AllowCreate = nil, "sync", false // a dry run looks at every forge, both ways
	}
	forges := s.syncForges()
	if len(req.Bridges) > 0 {
		forges = slices.DeleteFunc(forges, func(f config.Forge) bool { return !slices.Contains(req.Bridges, f.Name) })
	}
	job := newJob(req.DryRun, req.Mode, req.AllowCreate, forges, last)
	if !req.DryRun {
		forges = slices.DeleteFunc(forges, func(f config.Forge) bool { return !slices.Contains(last.Read, f.Name) })
	}
	// edits wait from here: a write already under way finishes first
	s.syncing.Store(true)
	s.job = job
	go s.runForgeSync(job, forges)
	writeJSON(w, http.StatusAccepted, s.forgeSnapshot())
}

// newJob is a job of these forges, each tallied, those with nothing to
// say too. A real run takes only the forges its check (last) read: one
// that wasn't, left out or turned on since, is told and left out.
func newJob(dry bool, mode string, allow bool, forges []config.Forge, last *forgeJob) *forgeJob {
	job := &forgeJob{DryRun: dry, Mode: mode, AllowCreate: allow, State: "running", Started: time.Now(), Names: []string{}, Bridges: map[string]*bridgeTally{}}
	for _, f := range forges {
		job.Names = append(job.Names, f.Name)
		t := job.tally(f.Name)
		if !dry && !slices.Contains(last.Read, f.Name) {
			t.add(store.SyncEvent{Forge: f.Name, What: "warning"}, "not in the check: run Check again to sync it")
		}
	}
	return job
}

// runOpts are a forge run's settings: a dry run, the consent to create
// (ForgeRun.AllowCreate), and the git remote a check merges first
// (ForgeRun.Merge).
type runOpts struct {
	DryRun, AllowCreate bool
	Merge               string
}

// forgeRun is the core's sync, or its push or its pull alone, with forges
// opened for the run.
func (s *apiServer) forgeRun(ctx context.Context, mode string, forges []config.Forge, o runOpts, see func(store.SyncEvent)) error {
	run, err := s.openRun(ctx, forges, o, see)
	if err != nil {
		return err
	}
	switch mode {
	case "send":
		_, err = s.b.SendForges(ctx, run)
	case "get":
		_, err = s.b.GetForges(ctx, run)
	default:
		_, err = s.b.SyncForges(ctx, run)
	}
	return err
}

// openRun opens the forges' adapters for one run, fresh: the settings and
// the tokens may have changed since the last. A forge found in the
// remotes whose GitLab id can't be read is left out, with a warning.
func (s *apiServer) openRun(ctx context.Context, forges []config.Forge, o runOpts, see func(store.SyncEvent)) (store.ForgeRun, error) {
	open := s.openForges
	if open == nil {
		open = store.OpenForges
	}
	forges = store.ResolveForges(ctx, forges, func(f config.Forge, why error) {
		see(store.SyncEvent{Forge: f.Name, What: "left", Line: fmt.Sprintf("%s %s: %v", f.Host, f.Path, why)})
	})
	fs, err := open(ctx, forges)
	if err != nil {
		return store.ForgeRun{}, err
	}
	return store.ForgeRun{Forges: fs, PublishPrivate: s.config().PublishPrivate, DryRun: o.DryRun, AllowCreate: o.AllowCreate, Merge: o.Merge, See: see}, nil
}

func (s *apiServer) runForgeSync(job *forgeJob, forges []config.Forge) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.endJob(job, s.run()(context.Background(), job.Mode, forges, runOpts{DryRun: job.DryRun, AllowCreate: job.AllowCreate}, s.see(job)))
}

// run is the forge run: runSync, else forgeRun.
func (s *apiServer) run() func(ctx context.Context, mode string, forges []config.Forge, o runOpts, see func(store.SyncEvent)) error {
	if s.runSync != nil {
		return s.runSync
	}
	return s.forgeRun
}

// see hears a job's forge events: its tallies and its step.
func (s *apiServer) see(job *forgeJob) func(store.SyncEvent) {
	return s.tell(job.DryRun, "", job.tally, func(st *syncStep) { job.Step = st })
}

// endJob ends a job, failed with err, and tells the pages: the header's
// sync link, and the issues a real run may have changed. Done, it asks the
// forges who can be assigned.
func (s *apiServer) endJob(job *forgeJob, err error) {
	s.forgeMu.Lock()
	now := time.Now()
	job.State, job.Ended = "done", &now
	if job.DryRun {
		for _, name := range job.Names {
			if job.tally(name).Left == "" {
				job.Read = append(job.Read, name)
			}
		}
	}
	if err != nil {
		job.State, job.Error = "failed", store.NoCredentials(err.Error()) // a git URL may carry a token
		job.Errors++
	}
	s.syncing.Store(false) // with the state: who sees it ended may start the next at once
	s.forgeMu.Unlock()
	s.seeRefs()
	s.events.publish(event{Kind: "sync"})
	if !job.DryRun {
		s.events.publish(event{Kind: "issues"})
	}
	if err == nil {
		go s.fetchAssignable(job) // a forge that hangs keeps neither the run nor the edits
	}
}
