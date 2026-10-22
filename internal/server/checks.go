package server

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/fields"
	"github.com/guilbep/git-board/internal/proof"
)

// Step 4 (docs/prd/cycle-protocol.md, M4): the suites and checks the base
// branch's .git-board.toml declares, run in a clean checkout, each result
// a signed line in this clone's notes (internal/proof, ADR 0001). Only
// argv from the base branch's toml runs: the page names suites, nothing
// it sends reaches a command line.

// proofJob is the run in progress, or the last one: one at a time per
// server, since every run checks out the same repository.
type proofJob struct {
	ID      string   // the issue
	Running []string // the names left to run
	Error   string   // a check that couldn't start: no verdict, the reason
}

// suiteRow is a suite or a check as the page lists it: its entry of the
// base's toml, the changed files it reads, its verdict at the head and,
// for a bug, at the test's commit.
type suiteRow struct {
	Name string `json:"name"`
	config.Suite
	Touched []string           `json:"touched"`
	Head    *proof.CheckStatus `json:"head,omitempty"`
	AtTest  *proof.CheckStatus `json:"at_test,omitempty"`
}

type stepTests struct {
	Rule       string           `json:"rule"` // [cycle.kinds]: failing-test, per-criterion, none
	Base       string           `json:"base"`
	Head       string           `json:"head,omitempty"`
	TestCommit string           `json:"test_commit,omitempty"` // the bug rule's: the commit adding the test
	Suites     []suiteRow       `json:"suites"`
	Checks     []suiteRow       `json:"checks"`
	Gate       proof.GateResult `json:"gate"`
	Running    []string         `json:"running"`
	Error      string           `json:"error,omitempty"`
}

// GET /api/issues/{id}/tests: step 4's suites and checks at the branch's
// last commit, and the gate on Mark ready for review.
func (s *apiServer) handleStepTests(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	th, err := s.b.Thread(id)
	if err != nil {
		writeIssueError(w, err)
		return
	}
	cur, err := s.cycleOf(r.Context(), th)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	ev := cur.Evidence
	res := stepTests{Rule: cur.Stages.KindRule(ev.Kind), Base: ev.Base, Head: ev.Head, Suites: []suiteRow{}, Checks: []suiteRow{}, Running: []string{}, Gate: proof.GateResult{Checks: []proof.CheckStatus{}}}
	if res.Rule == config.FailingTest && len(ev.Tests) > 0 {
		res.TestCommit = ev.Tests[0].Commit
	}
	if ev.Base == "" {
		writeJSON(w, http.StatusOK, res)
		return
	}
	cfg, signers, err := proof.AtBase(r.Context(), s.root, ev.Base)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	rows := func(suites map[string]config.Suite) ([]suiteRow, error) {
		out := []suiteRow{}
		for _, name := range slices.Sorted(maps.Keys(suites)) {
			row := suiteRow{Name: name, Suite: suites[name], Touched: []string{}}
			if ev.Head != "" {
				if row.Touched, err = proof.Touched(r.Context(), s.root, ev.Base, ev.Head, row.Files); err != nil {
					return nil, err
				}
				st, err := proof.StatusAt(r.Context(), s.root, ev.Head, name, row.Suite, signers)
				if err != nil {
					return nil, err
				}
				row.Head = &st
			}
			if res.TestCommit != "" {
				st, err := proof.StatusAt(r.Context(), s.root, res.TestCommit, name, row.Suite, signers)
				if err != nil {
					return nil, err
				}
				row.AtTest = &st
			}
			out = append(out, row)
		}
		return out, nil
	}
	if res.Suites, err = rows(cfg.Cycle.Tests); err == nil {
		res.Checks, err = rows(cfg.Cycle.Checks)
	}
	if err == nil && ev.Head != "" {
		res.Gate, err = proof.Gate(r.Context(), s.root, ev.Base, ev.Head)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.proofMu.Lock()
	if j := s.proofJob; j != nil && j.ID == th.ID {
		res.Running, res.Error = slices.Clone(j.Running), j.Error
	}
	s.proofMu.Unlock()
	writeJSON(w, http.StatusOK, res)
}

// POST /api/issues/{id}/tests/run, body {"names": [...], "target":
// "head" | "test"}: runs the named suites and checks of the base branch's
// toml at the branch's last commit, or (target test, the bug rule) at the
// commit that added the test. 202 at once; a cycle event after each
// result.
func (s *apiServer) handleRunTests(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Names  []string `json:"names"`
		Target string   `json:"target"`
	}
	if !decode(w, r, &req) {
		return
	}
	th, err := s.b.Thread(r.PathValue("id"))
	if err != nil {
		writeIssueError(w, err)
		return
	}
	cur, err := s.cycleOf(r.Context(), th)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	ev := cur.Evidence
	commit := ev.Head
	switch {
	case cur.Stage != "in-progress":
		writeError(w, http.StatusBadRequest, fmt.Errorf("runs at steps 4 and 5, the status %s", cur.Stages.InProgress[0]))
		return
	case ev.Head == "":
		writeError(w, http.StatusBadRequest, errors.New("no branch names this issue yet"))
		return
	case req.Target == "test":
		if len(ev.Tests) == 0 || ev.Tests[0].Commit == "" {
			writeError(w, http.StatusBadRequest, errors.New("no commit adds a test yet"))
			return
		}
		commit = ev.Tests[0].Commit
	case req.Target != "" && req.Target != "head":
		writeError(w, http.StatusBadRequest, fmt.Errorf("target: head or test, not %q", req.Target))
		return
	}
	if len(req.Names) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("names: the suites to run"))
		return
	}
	cfg, _, err := proof.AtBase(r.Context(), s.root, ev.Base)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	suites := map[string]config.Suite{}
	for _, name := range req.Names {
		suite, ok := cfg.Cycle.Tests[name]
		if c, isCheck := cfg.Cycle.Checks[name]; isCheck {
			suite, ok = c, true
		}
		switch {
		case !ok:
			writeError(w, http.StatusNotFound, fmt.Errorf("no suite or check %q in .git-board.toml on %s", name, ev.Base))
			return
		case suite.Where == "ci":
			writeError(w, http.StatusBadRequest, fmt.Errorf("%s runs on the forge (where = \"ci\")", name))
			return
		}
		suites[name] = suite
	}
	keyFile, err := proof.SigningKey(r.Context(), s.root)
	if err != nil {
		writeCoded(w, http.StatusBadRequest, "no-signing-key", err)
		return
	}
	writer, err := proof.WriterName(r.Context(), s.root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.proofMu.Lock()
	if s.proofJob != nil && len(s.proofJob.Running) > 0 {
		s.proofMu.Unlock()
		writeCoded(w, http.StatusConflict, "busy", errors.New("a run is in progress: try again when it's done"))
		return
	}
	job := &proofJob{ID: th.ID, Running: slices.Clone(req.Names)}
	s.proofJob = job
	s.proofMu.Unlock()

	go func() {
		// ponytail: a run outlives the request, not the server; a shutdown
		// mid-run records nothing
		ctx := context.Background()
		for _, name := range req.Names {
			_, err := proof.Run(ctx, s.root, commit, name, suites[name], keyFile, writer)
			s.proofMu.Lock()
			job.Running = job.Running[1:]
			if err != nil {
				job.Error = err.Error()
			}
			s.proofMu.Unlock()
			s.events.publish(event{Kind: "cycle", ID: th.ID})
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"running": req.Names, "commit": commit})
}

var logHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

// GET /api/issues/{id}/tests/log/{hash}: a run's output, as this clone
// kept it; 404 for another writer's run.
func (s *apiServer) handleRunLog(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	if !logHash.MatchString(hash) {
		writeError(w, http.StatusNotFound, errors.New("no such log"))
		return
	}
	dir, err := proof.LogDir(r.Context(), s.root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	data, err := os.ReadFile(filepath.Join(dir, hash))
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("no log here: another clone ran it"))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(data)
}

// readyForReview moves the status to the first in-review status once
// every [cycle.checks.*] of the base branch is proven at the branch's last
// commit: 409 unproven otherwise, with the reason.
// ponytail: a PATCH of the status still moves it ungated; the step and
// this button are the path the gate holds.
func (s *apiServer) readyForReview(w http.ResponseWriter, r *http.Request) {
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	id := r.PathValue("id")
	th, err := s.b.Thread(id)
	if err != nil {
		writeIssueError(w, err)
		return
	}
	cur, err := s.cycleOf(r.Context(), th)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	ev := cur.Evidence
	if !slices.Contains(cur.Actions, "ready-for-review") || len(cur.Stages.InReview) == 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("not yours to mark ready for review: %s", cur.Waits.What))
		return
	}
	// ponytail: no branch is no code to prove (a doc issue with nothing
	// committed): the gate is open
	if ev.Head != "" {
		g, err := proof.Gate(r.Context(), s.root, ev.Base, ev.Head)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if !g.Proven {
			writeCoded(w, http.StatusConflict, "unproven", errors.New(g.Reason))
			return
		}
	}
	if _, err := fields.Apply(s.b, s.config(), id, "status", cur.Stages.InReview[0], s.me, time.Now()); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if th, err = s.b.Thread(id); err == nil {
		cur, err = s.cycleOf(r.Context(), th)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, cur)
}
