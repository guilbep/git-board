package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/guilbep/git-board/internal/gitbugio"
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

// One issue on one forge (git-bug's Link, Unlink, Take): each names a
// bridge and writes nothing to the forge. The engine's refusals answer 409
// with its words; an unknown issue or bridge, 404.

// forgeEdit resolves the issue and holds the write lock, as any edit. A
// dry run of this issue is stale after it: what a sync sends changes.
func (s *apiServer) forgeEdit(w http.ResponseWriter, r *http.Request) (string, func(), bool) {
	issue, err := s.b.GetIssue(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return "", nil, false
	}
	done, ok := s.beginWrite(w)
	if !ok {
		return "", nil, false
	}
	s.forgeMu.Lock()
	delete(s.issueDry, issue.ID)
	s.forgeMu.Unlock()
	return issue.ID, done, true
}

func writeForgeError(w http.ResponseWriter, err error) {
	if errors.Is(err, gitbugio.ErrNoBridge) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeError(w, http.StatusConflict, err)
}

// handleIssueLink: the issue is one already on the bridge's forge, by its
// number or URL. It answers the forge issue.
func (s *apiServer) handleIssueLink(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Bridge string `json:"bridge"`
		Issue  string `json:"issue"`
	}
	if !decode(w, r, &req) {
		return
	}
	id, done, ok := s.forgeEdit(w, r)
	if !ok {
		return
	}
	defer done()
	ref, err := s.b.LinkIssue(r.Context(), id, req.Bridge, strings.TrimSpace(req.Issue))
	if err != nil {
		writeForgeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": ref.ID, "url": ref.URL, "number": ref.Number})
}

// handleIssueUnlink: the issue never syncs with the bridge's forge again.
func (s *apiServer) handleIssueUnlink(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Bridge string `json:"bridge"`
	}
	if !decode(w, r, &req) {
		return
	}
	id, done, ok := s.forgeEdit(w, r)
	if !ok {
		return
	}
	defer done()
	if err := s.b.UnlinkIssue(r.Context(), id, req.Bridge); err != nil {
		writeForgeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{})
}

// handleIssueTake: what the issue hasn't sent to the bridge's forge is
// replaced by the forge's version, pulled now. It answers what came in, as
// one issue's sync counts it.
func (s *apiServer) handleIssueTake(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Bridge string `json:"bridge"`
	}
	if !decode(w, r, &req) {
		return
	}
	id, done, ok := s.forgeEdit(w, r)
	if !ok {
		return
	}
	defer done()
	results, err := s.b.TakeIssue(r.Context(), id, req.Bridge)
	if err != nil {
		writeForgeError(w, err)
		return
	}
	t := &bridgeTally{Lines: []string{}}
	res := issueSyncResult{Bridges: map[string]*bridgeTally{req.Bridge: t}}
	for sr := range results {
		t.add(sr, false)
		if sr.IsError() {
			res.Errors++
		}
	}
	writeJSON(w, http.StatusOK, res)
}
