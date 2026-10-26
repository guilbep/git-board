package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
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

// handleIssueSync syncs one issue with every sync bridge (SyncIssue): its
// changes pulled from each forge it is on, then pushed.
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

	// every sync bridge, those with nothing to say too: the page tells
	// what a refresh pulling from one forge sends to the others
	forges := s.syncForges()
	res := issueSyncResult{DryRun: req.DryRun, Bridges: map[string]*bridgeTally{}}
	tally := func(name string) *bridgeTally {
		if res.Bridges[name] == nil {
			res.Bridges[name] = &bridgeTally{Lines: []string{}}
		}
		return res.Bridges[name]
	}
	for _, f := range forges {
		tally(f.Name)
	}
	run := s.runIssueSync
	if run == nil {
		run = s.issueRun
	}
	if err := run(r.Context(), forges, req.DryRun, issue.ID, s.tell(req.DryRun, issue.ID, tally, nil)); err != nil {
		s.forgeMu.Lock()
		delete(s.issueDry, issue.ID)
		s.forgeMu.Unlock()
		writeError(w, http.StatusBadGateway, err)
		return
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

// issueRun is the core's sync of one issue, with forges opened for it:
// the person sending this one issue consents to its creation.
func (s *apiServer) issueRun(ctx context.Context, forges []config.Forge, dryRun bool, id string, see func(store.SyncEvent)) error {
	run, err := s.openRun(ctx, forges, runOpts{DryRun: dryRun, AllowCreate: true}, see)
	if err != nil {
		return err
	}
	_, err = s.b.SyncIssue(ctx, run, id)
	return err
}
