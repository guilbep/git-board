package server

import (
	"errors"
	"net/http"
	"time"
)

var errIssueDryRunFirst = errors.New("run a dry run of this issue first: it says what the sync would write to the forges")

// issueSyncResult is one issue's sync, per bridge as the full sync counts
// it. Writes are what it sends, or would send, to the forges.
type issueSyncResult struct {
	DryRun  bool                    `json:"dry_run"`
	Writes  int                     `json:"writes"`
	Errors  int                     `json:"errors"`
	Bridges map[string]*bridgeTally `json:"bridges"`
}

// handleIssueSync syncs one issue with every sync bridge (git-bug's
// SyncIssue): its changes pulled from each forge it is on, then pushed.
// It answers when done, a few calls per forge. A real run needs a clean
// dry run of that issue in the last 15 minutes, as the full sync does; a
// full sync running refuses it, and edits wait while it writes.
func (s *apiServer) handleIssueSync(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DryRun bool `json:"dry_run"`
	}
	if !decode(w, r, &req) {
		return
	}
	issue, err := s.b.GetIssue(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	s.forgeMu.Lock()
	last, dried := s.issueDry[issue.ID]
	s.forgeMu.Unlock()
	if !req.DryRun && (!dried || time.Since(last) > dryRunFresh) {
		writeError(w, http.StatusConflict, errIssueDryRunFirst)
		return
	}
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()

	run := s.runIssueSync
	if run == nil {
		run = s.b.SyncIssue
	}
	results, err := run(r.Context(), req.DryRun, issue.ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// every sync bridge, those with nothing to say too: the page tells
	// what a refresh pulling from one forge sends to the others
	res := issueSyncResult{DryRun: req.DryRun, Bridges: map[string]*bridgeTally{}}
	if bridges, err := s.b.SyncBridges(); err == nil {
		for _, b := range bridges {
			res.Bridges[b.Name] = &bridgeTally{Lines: []string{}}
		}
	}
	st := stepper{name: s.issueName, pushTotal: 1}
	for sr := range results {
		t := res.Bridges[sr.Bridge]
		if t == nil {
			t = &bridgeTally{Lines: []string{}}
			res.Bridges[sr.Bridge] = t
		}
		t.add(sr, req.DryRun)
		if sr.IsError() {
			res.Errors++
		}
		if step, moved := st.see(sr); moved {
			s.events.publish(event{Kind: "progress", ID: issue.ID, Step: &step})
		}
	}
	for _, t := range res.Bridges {
		res.Writes += t.To.Issues + t.To.Comments + t.To.Edits + t.To.Changes
	}

	s.forgeMu.Lock()
	if s.issueDry == nil {
		s.issueDry = map[string]time.Time{}
	}
	if req.DryRun && res.Errors == 0 {
		s.issueDry[issue.ID] = time.Now()
	} else {
		delete(s.issueDry, issue.ID) // a real run, or a failed dry run: dry run again
	}
	s.forgeMu.Unlock()
	if !req.DryRun {
		s.events.publish(event{Kind: "issues"})
	}
	writeJSON(w, http.StatusOK, res)
}
