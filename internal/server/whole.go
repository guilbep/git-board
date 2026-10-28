package server

import (
	"cmp"
	"context"
	"errors"
	"log"
	"net/http"
	"slices"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
)

// The whole sync, Settings › Sync's and git-board sync's: store.SyncWhole
// run as the forge sync's job (forges.go), its forge step counted in the
// job's tallies, its Project step the Project's check or sync (sync.go),
// its git steps the remote's last check (remotes.go).

// wholeRemote is the git remote of the whole sync, as git-board sync's
// default: origin, which numbers the issues.
const wholeRemote = store.NumbersRemote

// wholeStatus is Settings › Sync: the git remote, every forge, the
// remotes' URLs that name none, the GitHub Project, and the last run.
type wholeStatus struct {
	Remote  *remoteStatus `json:"remote"` // nil: the repository has no origin
	Forges  []bridge      `json:"forges"`
	Skipped []string      `json:"skipped"` // "skipped <url>: why"
	Project syncStatus    `json:"project"`
	Job     *forgeJob     `json:"job"`
}

// jobStep is a step of the whole sync: pull, forges, project, push.
type jobStep struct {
	Name  string `json:"name"`
	State string `json:"state"`          // waiting, running, done, failed, skipped
	Note  string `json:"note,omitempty"` // why it failed or was skipped, what it did
}

// bridgeRows are every forge, sync on or off, and the URLs naming none.
func (s *apiServer) bridgeRows() ([]bridge, []string) {
	rows, skipped := []bridge{}, []string{}
	fs, err := s.b.Forges(s.config(), func(l string) { skipped = append(skipped, l) })
	if err != nil {
		log.Printf("the forges of the git remotes: %v", err)
	}
	for _, f := range fs {
		rows = append(rows, bridge{Name: f.Name, Target: f.Kind, Project: cmp.Or(f.Path, f.Project), Where: f.Where(), Sync: f.Sync, Discovered: f.Discovered})
	}
	return rows, skipped
}

func (s *apiServer) handleWhole(w http.ResponseWriter, r *http.Request) {
	rs, err := s.remotes()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var st wholeStatus
	if i := slices.IndexFunc(rs.Remotes, func(r remoteStatus) bool { return r.Name == wholeRemote }); i >= 0 {
		st.Remote = &rs.Remotes[i]
	}
	st.Forges, st.Skipped = s.bridgeRows()
	// ponytail: waits for a Project step running, as GET /api/sync does
	s.syncMu.Lock()
	st.Project = s.status()
	s.syncMu.Unlock()
	s.forgeMu.Lock()
	defer s.forgeMu.Unlock()
	st.Job = s.job
	writeJSON(w, http.StatusOK, st)
}

// handleWholeSync starts the whole sync: its check, or a real run right
// after a clean check of the whole sync (within dryRunFresh). It runs as
// the forge sync does: one at a time, edits waiting.
func (s *apiServer) handleWholeSync(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DryRun      bool `json:"dry_run"`
		AllowCreate bool `json:"allow_create"` // consent to the creations the check held
	}
	if !decode(w, r, &req) {
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
		if err := last.fresh(true); err != nil {
			writeError(w, http.StatusConflict, err)
			return
		}
	} else {
		req.AllowCreate = false
	}
	forges := s.syncForges()
	job := newJob(req.DryRun, "all", req.AllowCreate, forges, last)
	if !req.DryRun {
		forges = slices.DeleteFunc(forges, func(f config.Forge) bool { return !slices.Contains(last.Read, f.Name) })
	}
	for _, name := range store.Steps {
		job.Steps = append(job.Steps, jobStep{Name: name, State: "waiting"})
	}
	s.syncing.Store(true)
	s.job = job
	go s.runWhole(job, forges)
	writeJSON(w, http.StatusAccepted, s.forgeSnapshot())
}

func (s *apiServer) runWhole(job *forgeJob, forges []config.Forge) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	run := store.Whole{Remote: wholeRemote, DryRun: job.DryRun, Step: func(name, state, note string) {
		s.forgeMu.Lock()
		defer s.forgeMu.Unlock()
		if i := slices.IndexFunc(job.Steps, func(st jobStep) bool { return st.Name == name }); i >= 0 {
			job.Steps[i].State, job.Steps[i].Note = state, note
		}
		if state == "done" && (name == store.StepPull || name == store.StepPush) {
			if s.checked == nil {
				s.checked = map[string]time.Time{}
			}
			s.checked[wholeRemote] = time.Now()
		}
	}}
	if len(forges) > 0 {
		o := runOpts{DryRun: job.DryRun, AllowCreate: job.AllowCreate, Merge: wholeRemote}
		run.Forges = func(ctx context.Context) error { return s.run()(ctx, "sync", forges, o, s.see(job)) }
	}
	var projectErr error
	if gp := s.config().GithubProject; gp.Owner != "" && gp.Number != 0 {
		run.Project = func(context.Context) error {
			s.syncMu.Lock()
			defer s.syncMu.Unlock()
			if job.DryRun {
				projectErr = s.check()
			} else {
				projectErr = s.apply("") // the Project written to with write = true alone
			}
			return projectErr
		}
	}
	err := s.b.SyncWhole(context.Background(), run)
	if projectErr != nil && errors.Is(err, projectErr) && onlyProjectFailed(job) {
		// the Project's own failure, its step told: it doesn't make the
		// check unclean, nor the run failed, for git and the forges
		err = nil
	}
	s.endJob(job, err)
}

// onlyProjectFailed says the Project's is the job's one failed step.
func onlyProjectFailed(job *forgeJob) bool {
	for _, st := range job.Steps {
		if st.State == "failed" && st.Name != store.StepProject {
			return false
		}
	}
	return true
}
