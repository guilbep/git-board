package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/git-bug/git-bug/bridge/core"

	"github.com/guilbep/git-board/internal/gitbugio"
)

// dryRunFresh is how long a dry run stands for the real run it allows.
const dryRunFresh = 15 * time.Minute

// maxLines caps the lines kept per bridge.
const maxLines = 1000

var errDryRunFirst = errors.New("check first: a run writes, the check (a dry run) says what it would do")

// forgeStatus is the sync with the forges: which bridges take part, and
// the last run.
type forgeStatus struct {
	Bridges []gitbugio.SyncBridge `json:"bridges"`
	Job     *forgeJob             `json:"job"`
}

// the modes of a run: both ways, or one, git bug bridge pull (get) or push
// (send). A dry run looks both ways.
var forgeModes = []string{"sync", "get", "send"}

// forgeJob is one run of git-bug's forge sync, in the background: a co2
// dry run took over three minutes.
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
}

// bridgeTally counts what a run did, or would do (a dry run), on one
// bridge, both ways.
type bridgeTally struct {
	To   direction `json:"to"`   // git-board → forge
	From direction `json:"from"` // forge → git-board
	// Linked are issues a dry run found on the forge by their git-bug
	// marker: created by a run whose links weren't kept, synced, not
	// created again.
	Linked int `json:"linked"`
	// StaysLocal are texts no token can post: their authors have no token
	// on this forge.
	StaysLocal int      `json:"stays_local"`
	Warnings   int      `json:"warnings"`
	Errors     int      `json:"errors"`
	Lines      []string `json:"lines"`
}

// direction is one way of a sync. A dry run of a pull only knows "up to N
// changes" per issue: those go in Changes.
type direction struct {
	Issues   int `json:"issues"`  // created on the forge, or imported from it
	Changed  int `json:"changed"` // issues already linked with changes
	Comments int `json:"comments"`
	Edits    int `json:"edits"`
	Changes  int `json:"changes"` // title, state, labels
}

var (
	createRe = regexp.MustCompile(`^would create issue: .*, with (\d+) comments$`)
	pullRe   = regexp.MustCompile(`^would pull up to (\d+) changes`)
	countRe  = regexp.MustCompile(`^(\d+) (comments|edits)$`)
)

func (t *bridgeTally) add(r core.SyncResult, dryRun bool) {
	var nothing bool
	var reason string
	if r.Import != nil {
		nothing, reason = r.Import.Event == core.ImportEventNothing, r.Import.Reason
		switch r.Import.Event {
		case core.ImportEventNothing:
			if strings.HasPrefix(reason, "would import") {
				t.From.Issues++
			} else if m := pullRe.FindStringSubmatch(reason); m != nil {
				t.From.Changed++
				t.From.Changes += atoi(m[1])
			}
		case core.ImportEventBug:
			t.From.Issues++
		case core.ImportEventComment:
			t.From.Comments++
		case core.ImportEventCommentEdition:
			t.From.Edits++
		case core.ImportEventWarning, core.ImportEventRateLimiting:
			t.Warnings++
		case core.ImportEventError:
			t.Errors++
		default:
			t.From.Changes++
		}
	} else if r.Export != nil {
		nothing, reason = r.Export.Event == core.ExportEventNothing, r.Export.Reason
		switch r.Export.Event {
		case core.ExportEventNothing:
			t.planned(reason)
		case core.ExportEventBug:
			t.To.Issues++
		case core.ExportEventComment:
			t.To.Comments++
		case core.ExportEventCommentEdition:
			t.To.Edits++
		case core.ExportEventWarning, core.ExportEventRateLimiting:
			t.Warnings++
		case core.ExportEventError:
			t.Errors++
		default:
			t.To.Changes++
		}
	}
	// what git bug bridge sync prints
	if (!nothing || (dryRun && strings.HasPrefix(reason, "would "))) && len(t.Lines) < maxLines {
		t.Lines = append(t.Lines, r.String())
	}
}

// planned counts a dry run's push plan (bridge/core planPush).
func (t *bridgeTally) planned(reason string) {
	switch {
	case reason == "missing author token":
		t.StaysLocal++
	case strings.HasPrefix(reason, "would sync with"):
		t.Linked++
	case createRe.MatchString(reason):
		t.To.Issues++
		t.To.Comments += atoi(createRe.FindStringSubmatch(reason)[1])
	case strings.HasPrefix(reason, "would send: "):
		t.To.Changed++
		for _, part := range strings.Split(strings.TrimPrefix(reason, "would send: "), ", ") {
			if m := countRe.FindStringSubmatch(part); m != nil && m[2] == "comments" {
				t.To.Comments += atoi(m[1])
			} else if m != nil {
				t.To.Edits += atoi(m[1])
			} else {
				t.To.Changes++ // title, status, labels
			}
		}
	}
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func (s *apiServer) forgeSnapshot() forgeStatus {
	bridges, _ := s.b.SyncBridges()
	if bridges == nil {
		bridges = []gitbugio.SyncBridge{}
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
			if _, inDryRun := last.Bridges[name]; !inDryRun && !slices.Contains(last.Names, name) {
				writeError(w, http.StatusBadRequest, errors.New("bridge "+name+" wasn't in the check"))
				return
			}
		}
		if err := sendsAgain(req.Mode, last, req.Bridges); err != nil {
			writeError(w, http.StatusConflict, err)
			return
		}
	} else {
		req.Bridges, req.Mode = nil, "sync" // a dry run looks at every forge, both ways
	}
	run := s.runSync
	if run == nil {
		run = s.forgeRun
	}
	// edits wait from here: a write already under way finishes first
	s.syncing.Store(true)
	names := req.Bridges
	if len(names) == 0 {
		for _, b := range s.forgeSnapshot().Bridges {
			names = append(names, b.Name)
		}
	}
	job := &forgeJob{DryRun: req.DryRun, Mode: req.Mode, State: "running", Started: time.Now(), Names: names, Bridges: map[string]*bridgeTally{}}
	s.job = job
	go s.runForgeSync(job, run, req.Bridges)
	writeJSON(w, http.StatusAccepted, s.forgeSnapshot())
}

// sendsAgain refuses a send to a forge where the dry run found issues by
// their git-bug marker: created there by a run whose links weren't kept,
// only a pull links them. A send alone would create them again.
func sendsAgain(mode string, dry *forgeJob, names []string) error {
	if mode != "send" {
		return nil
	}
	for name, t := range dry.Bridges {
		if (len(names) == 0 || slices.Contains(names, name)) && t.Linked > 0 {
			return fmt.Errorf("%s has %d issues of this repository already, found by their git-bug marker: a send alone would create them again. Get or sync first", name, t.Linked)
		}
	}
	return nil
}

// forgeRun is git-bug's sync, or one way of it.
func (s *apiServer) forgeRun(ctx context.Context, mode string, dryRun bool, names []string) (<-chan core.SyncResult, error) {
	switch mode {
	case "get":
		return s.b.GetForges(ctx, names)
	case "send":
		return s.b.SendForges(ctx, names)
	}
	return s.b.SyncForges(ctx, dryRun, names)
}

func (s *apiServer) runForgeSync(job *forgeJob, run func(context.Context, string, bool, []string) (<-chan core.SyncResult, error), names []string) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	defer s.syncing.Store(false)

	finish := func(state, errText string) {
		s.forgeMu.Lock()
		now := time.Now()
		job.State, job.Error, job.Ended = state, errText, &now
		s.forgeMu.Unlock()
		// the pages update: the header's sync link, and the issues a real
		// run may have changed
		s.seeRefs()
		s.events.publish(event{Kind: "sync"})
		if !job.DryRun {
			s.events.publish(event{Kind: "issues"})
		}
	}
	results, err := run(context.Background(), job.Mode, job.DryRun, names)
	if err != nil {
		finish("failed", err.Error())
		return
	}
	for r := range results {
		s.forgeMu.Lock()
		t := job.Bridges[r.Bridge]
		if t == nil {
			t = &bridgeTally{Lines: []string{}}
			job.Bridges[r.Bridge] = t
		}
		t.add(r, job.DryRun)
		if r.IsError() {
			job.Errors++
		}
		s.forgeMu.Unlock()
	}
	finish("done", "")
}
