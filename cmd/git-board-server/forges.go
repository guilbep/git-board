package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/git-bug/git-bug/bridge/core"

	"github.com/guilbep/git-board/internal/gitbugio"
)

// dryRunFresh is how long a dry run stands for the real run it allows.
const dryRunFresh = 15 * time.Minute

// maxLines caps the lines kept per bridge.
const maxLines = 1000

var errDryRunFirst = errors.New("run a dry run first: a real sync writes to GitHub/GitLab, the dry run says what it would do")

// forgeStatus is the sync with GitHub/GitLab: which bridges take part, and
// the last run.
type forgeStatus struct {
	Bridges []gitbugio.SyncBridge `json:"bridges"`
	Job     *forgeJob             `json:"job"`
}

// forgeJob is one run of git-bug's forge sync, in the background: a co2
// dry run took over three minutes.
type forgeJob struct {
	DryRun  bool                    `json:"dry_run"`
	Push    bool                    `json:"push"`
	State   string                  `json:"state"` // running, done, failed
	Started time.Time               `json:"started"`
	Ended   *time.Time              `json:"ended,omitempty"`
	Bridges map[string]*bridgeTally `json:"bridges"`
	Errors  int                     `json:"errors"`
	Error   string                  `json:"error,omitempty"`  // the run couldn't start or failed
	Pushed  string                  `json:"pushed,omitempty"` // pushing git-bug data to origin, after a real run
}

// bridgeTally counts what a run did, or would do, on one bridge.
type bridgeTally struct {
	Imported    int      `json:"imported"` // new issues pulled
	Pulled      int      `json:"pulled"`   // other changes pulled
	Created     int      `json:"created"`  // issues created on the forge
	Pushed      int      `json:"pushed"`   // other changes pushed
	WouldImport int      `json:"would_import"`
	WouldPull   int      `json:"would_pull"`
	WouldCreate int      `json:"would_create"`
	WouldPush   int      `json:"would_push"`
	Warnings    int      `json:"warnings"`
	Errors      int      `json:"errors"`
	Lines       []string `json:"lines"`
}

func (t *bridgeTally) add(r core.SyncResult, dryRun bool) {
	var nothing bool
	var reason string
	if r.Import != nil {
		nothing, reason = r.Import.Event == core.ImportEventNothing, r.Import.Reason
		switch r.Import.Event {
		case core.ImportEventNothing:
			switch {
			case strings.HasPrefix(reason, "would import"):
				t.WouldImport++
			case strings.HasPrefix(reason, "would "):
				t.WouldPull++
			}
		case core.ImportEventBug:
			t.Imported++
		case core.ImportEventWarning, core.ImportEventRateLimiting:
			t.Warnings++
		case core.ImportEventError:
			t.Errors++
		default:
			t.Pulled++
		}
	} else if r.Export != nil {
		nothing, reason = r.Export.Event == core.ExportEventNothing, r.Export.Reason
		switch r.Export.Event {
		case core.ExportEventNothing:
			switch {
			case strings.HasPrefix(reason, "would create"):
				t.WouldCreate++
			case strings.HasPrefix(reason, "would "):
				t.WouldPush++
			}
		case core.ExportEventBug:
			t.Created++
		case core.ExportEventWarning, core.ExportEventRateLimiting:
			t.Warnings++
		case core.ExportEventError:
			t.Errors++
		default:
			t.Pushed++
		}
	}
	// what git bug bridge sync prints
	if (!nothing || (dryRun && strings.HasPrefix(reason, "would "))) && len(t.Lines) < maxLines {
		t.Lines = append(t.Lines, r.String())
	}
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
// the plan of. A real run can then push the git-bug data to origin, for
// other clones to know the links it made.
func (s *apiServer) handleForgeSync(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DryRun bool `json:"dry_run"`
		Push   bool `json:"push"`
	}
	if !decode(w, r, &req) {
		return
	}
	s.forgeMu.Lock()
	defer s.forgeMu.Unlock()
	if s.syncing.Load() {
		writeError(w, http.StatusConflict, errSyncing)
		return
	}
	if !req.DryRun {
		last := s.job
		if last == nil || !last.DryRun || last.State != "done" || last.Ended == nil || time.Since(*last.Ended) > dryRunFresh {
			writeError(w, http.StatusConflict, errDryRunFirst)
			return
		}
	}
	run := s.runSync
	if run == nil {
		run = s.b.SyncForges
	}
	// edits wait from here: a write already under way finishes first
	s.syncing.Store(true)
	job := &forgeJob{DryRun: req.DryRun, Push: req.Push && !req.DryRun, State: "running", Started: time.Now(), Bridges: map[string]*bridgeTally{}}
	s.job = job
	go s.runForgeSync(job, run)
	writeJSON(w, http.StatusAccepted, s.forgeSnapshot())
}

func (s *apiServer) runForgeSync(job *forgeJob, run func(context.Context, bool) (<-chan core.SyncResult, error)) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	defer s.syncing.Store(false)

	finish := func(state, errText string) {
		s.forgeMu.Lock()
		defer s.forgeMu.Unlock()
		now := time.Now()
		job.State, job.Error, job.Ended = state, errText, &now
	}
	results, err := run(context.Background(), job.DryRun)
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
	if job.Push && job.Errors == 0 {
		out, err := s.b.Push("origin")
		s.forgeMu.Lock()
		if err != nil {
			job.Pushed = "not pushed to origin: " + err.Error()
		} else {
			job.Pushed = strings.TrimSpace(out)
		}
		s.forgeMu.Unlock()
	}
	finish("done", "")
}
