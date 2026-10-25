package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// dryRunFresh is how long a dry run stands for the real run it allows.
const dryRunFresh = 15 * time.Minute

// maxLines caps the lines kept per bridge.
const maxLines = 1000

var errDryRunFirst = errors.New("check first: a run writes, the check (a dry run) says what it would do")

// A bridge is a forge of .git-board.toml ([forges.<name>]), by its name.

// syncBridge is a forge taking part in the sync.
type syncBridge struct {
	Name   string `json:"name"`
	Target string `json:"target"` // github, gitlab
}

// forgeStatus is the sync with the forges: which bridges take part, and
// the last run.
type forgeStatus struct {
	Bridges []syncBridge `json:"bridges"`
	Job     *forgeJob    `json:"job"`
}

// the modes of a run: both ways, or one, pull (get) or push (send). A dry
// run looks both ways.
var forgeModes = []string{"sync", "get", "send"}

// forgeJob is one run of the forge sync, in the background: a co2 dry run
// took over three minutes.
type forgeJob struct {
	DryRun  bool                    `json:"dry_run"`
	Mode    string                  `json:"mode"`  // sync, get, send
	Names   []string                `json:"names"` // the bridges of the run
	State   string                  `json:"state"` // running, done, failed
	Started time.Time               `json:"started"`
	Ended   *time.Time              `json:"ended,omitempty"`
	Bridges map[string]*bridgeTally `json:"bridges"`
	Errors  int                     `json:"errors"`
	Error   string                  `json:"error,omitempty"` // the run couldn't start or failed
	Step    *syncStep               `json:"step,omitempty"`  // where it is, for a page loading mid-run
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

// syncStep is where a run is: the bridge, the way, how many issues that
// way has come to so far, and the issue it is on.
type syncStep struct {
	Bridge string `json:"bridge"`
	Way    string `json:"way"` // pull, push
	Done   int    `json:"done"`
	// ponytail: Total is never set, the run doesn't say how many it will
	// write; the page shows the count alone
	Total int    `json:"total,omitempty"`
	Issue string `json:"issue,omitempty"` // "#43 import: users lose their work"
}

// stepper turns a run's events into its steps: one each time a way of a
// bridge comes to another issue.
type stepper struct {
	counts map[string]int // issues come to, per bridge and way
	last   string         // bridge, way and issue of the last step
}

func (st *stepper) see(e gitbugio.SyncEvent, name string) (syncStep, bool) {
	key := e.Forge + " " + e.Way
	if !names(e) || st.last == key+" "+e.Issue {
		return syncStep{}, false // a write, a warning, or the same issue: the step stays
	}
	st.last = key + " " + e.Issue
	if st.counts == nil {
		st.counts = map[string]int{}
	}
	st.counts[key]++
	return syncStep{Bridge: e.Forge, Way: e.Way, Done: st.counts[key], Issue: name}, true
}

// names says an event names an issue of git-board's.
func names(e gitbugio.SyncEvent) bool {
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
	Assignable int      `json:"assignable"`
	Warnings   int      `json:"warnings"`
	Errors     int      `json:"errors"`
	Lines      []string `json:"lines"`
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
func (t *bridgeTally) add(e gitbugio.SyncEvent, line string) {
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
	}
	if line != "" && len(t.Lines) < maxLines {
		t.Lines = append(t.Lines, line)
	}
}

// line says an event in words, its issue by name; "" for a write, which
// the counts say.
func line(e gitbugio.SyncEvent, dry bool, name string) string {
	var l string
	switch e.What {
	case "created":
		l = fmt.Sprintf("pull a new issue, %s (%d changes)", name, e.Ops)
	case "updated":
		l = fmt.Sprintf("pull %d changes into %s", e.Ops, name)
	case "linked":
		l = "link " + name + ", found by its marker"
	case "posted":
		l = "create " + name
	case "pushed":
		l = "send the changes of " + name
	case "warning":
		return e.Line
	default:
		return ""
	}
	if dry {
		return "would " + l
	}
	return l
}

// tell is a run's events counted into tally, and its steps published:
// tally is called with forgeMu held. id names the issue a step is on, for
// its drawer; "" for a full run.
func (s *apiServer) tell(dry bool, id string, tally func(name string) *bridgeTally, step func(*syncStep)) func(gitbugio.SyncEvent) {
	var st stepper
	return func(e gitbugio.SyncEvent) {
		name := ""
		if names(e) {
			name = s.issueName(e.Issue)
		}
		text := line(e, dry, name)
		next, moved := st.see(e, name)
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

// syncForges are the forges taking part in the sync.
func (s *apiServer) syncForges() []config.Forge {
	return slices.DeleteFunc(slices.Clone(s.config().Forges), func(f config.Forge) bool { return !f.Sync })
}

func (s *apiServer) forgeSnapshot() forgeStatus {
	bridges := []syncBridge{}
	for _, f := range s.syncForges() {
		bridges = append(bridges, syncBridge{Name: f.Name, Target: f.Kind})
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
// the plan of. A real run goes both ways, or one: get, send.
func (s *apiServer) handleForgeSync(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DryRun  bool     `json:"dry_run"`
		Mode    string   `json:"mode"`    // sync (empty), get, send
		Bridges []string `json:"bridges"` // a real run: some of the dry run's; empty: all
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Mode == "" {
		req.Mode = "sync"
	}
	if !slices.Contains(forgeModes, req.Mode) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("mode %q: want sync, get or send", req.Mode))
		return
	}
	s.forgeMu.Lock()
	defer s.forgeMu.Unlock()
	if s.syncing.Load() {
		writeCoded(w, http.StatusConflict, "syncing", errSyncing)
		return
	}
	if !req.DryRun {
		last := s.job
		if last == nil || !last.DryRun || last.State != "done" || last.Ended == nil || time.Since(*last.Ended) > dryRunFresh {
			writeError(w, http.StatusConflict, errDryRunFirst)
			return
		}
		if last.Errors > 0 {
			writeError(w, http.StatusConflict, errors.New("the check had errors: fix them and check again"))
			return
		}
		for _, name := range req.Bridges {
			if !slices.Contains(last.Names, name) {
				writeError(w, http.StatusBadRequest, errors.New("bridge "+name+" wasn't in the check"))
				return
			}
		}
	} else {
		req.Bridges, req.Mode = nil, "sync" // a dry run looks at every forge, both ways
	}
	forges := s.syncForges()
	if len(req.Bridges) > 0 {
		forges = slices.DeleteFunc(forges, func(f config.Forge) bool { return !slices.Contains(req.Bridges, f.Name) })
	}
	job := &forgeJob{DryRun: req.DryRun, Mode: req.Mode, State: "running", Started: time.Now(), Names: []string{}, Bridges: map[string]*bridgeTally{}}
	for _, f := range forges {
		job.Names = append(job.Names, f.Name)
		job.tally(f.Name) // every bridge of the run, those with nothing to say too
	}
	// edits wait from here: a write already under way finishes first
	s.syncing.Store(true)
	s.job = job
	go s.runForgeSync(job, forges)
	writeJSON(w, http.StatusAccepted, s.forgeSnapshot())
}

// forgeRun is the core's sync, or one way of it, with forges opened for
// the run.
func (s *apiServer) forgeRun(ctx context.Context, mode string, forges []config.Forge, dryRun bool, see func(gitbugio.SyncEvent)) error {
	run, err := s.openRun(ctx, forges, dryRun, see)
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
// the tokens may have changed since the last.
func (s *apiServer) openRun(ctx context.Context, forges []config.Forge, dryRun bool, see func(gitbugio.SyncEvent)) (gitbugio.ForgeRun, error) {
	open := s.openForges
	if open == nil {
		open = gitbugio.OpenForges
	}
	fs, err := open(ctx, forges)
	if err != nil {
		return gitbugio.ForgeRun{}, err
	}
	return gitbugio.ForgeRun{Forges: fs, PublishPrivate: s.config().PublishPrivate, DryRun: dryRun, See: see}, nil
}

func (s *apiServer) runForgeSync(job *forgeJob, forges []config.Forge) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	finish := func(state, errText string) {
		s.forgeMu.Lock()
		now := time.Now()
		job.State, job.Error, job.Ended = state, errText, &now
		if errText != "" {
			job.Errors++
		}
		s.syncing.Store(false) // with the state: who sees it ended may start the next at once
		s.forgeMu.Unlock()
		// the pages update: the header's sync link, and the issues a real
		// run may have changed
		s.seeRefs()
		s.events.publish(event{Kind: "sync"})
		if !job.DryRun {
			s.events.publish(event{Kind: "issues"})
		}
	}
	run := s.runSync
	if run == nil {
		run = s.forgeRun
	}
	see := s.tell(job.DryRun, "", job.tally, func(st *syncStep) { job.Step = st })
	if err := run(context.Background(), job.Mode, forges, job.DryRun, see); err != nil {
		finish("failed", err.Error())
		return
	}
	finish("done", "")
	go s.fetchAssignable(job) // a forge that hangs keeps neither the run nor the edits
}
