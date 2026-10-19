package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/guilbep/git-board/internal/release"
)

// Releases (internal/release): what runs where and the releases, an
// issue's environments, the quick writes on a release's record, and the
// jobs that move releases (deploy, cut, promote, rollback), which run in
// the background like the forge sync, one at a time.
//
// runnerMu serializes the Runner's fetches and writes: a job holds it for
// its whole run, a quick write for its own, and a read takes it when free
// (it fetches) or reads offline, what was last fetched, beside a job. An
// issue's environments are always read offline: an issue page fetches
// nothing; the Releases page does.

var (
	errNoRelease = errors.New("no [release] in .git-board.toml")
	errReleasing = errors.New("a release job is running: wait for it to end")
)

type releasesResponse struct {
	Enabled      bool                 `json:"enabled"`
	Remote       string               `json:"remote"` // the authority, [release] remote
	Trunk        string               `json:"trunk"`  // the trunk branch, e.g. dev
	Environments []release.EnvState   `json:"environments"`
	Releases     []release.ReleaseRow `json:"releases"`
	Job          *releaseJob          `json:"job,omitempty"`
}

// releaseDetail is a release's page.
type releaseDetail struct {
	Row        release.ReleaseRow        `json:"row"`
	Issues     []release.DraftIssue      `json:"issues"`
	Candidates []release.ReleaseRow      `json:"candidates"` // the rc rows of its final, but itself
	History    map[string][]historyEntry `json:"history"`    // env → its overlays' changes naming this version
}

// historyEntry is an overlay change and the deploy target it is of.
type historyEntry struct {
	release.Change
	Target string `json:"target"` // release.TargetState.Name
}

// releaseJob is the job running, or the last one.
type releaseJob struct {
	Kind    string         `json:"kind"`    // deploy, cut, promote, rollback
	Subject string         `json:"subject"` // dev, v1.0.1-rc.1 to stage, prod to v1.0.0, the rc promoted; a cut's version, then its tag
	State   string         `json:"state"`   // running, done, failed
	Error   string         `json:"error"`
	Code    string         `json:"code"`  // a failed job's: taken, older, not-adopted; "" otherwise
	Tag     string         `json:"tag"`   // the tag a cut or promote pushed or tried, once known
	Steps   []release.Step `json:"steps"` // each step once, as last reported
	Started time.Time      `json:"started"`
	Ended   *time.Time     `json:"ended"`
}

// issueEnvironment tells whether an environment runs an issue's merged
// branch.
type issueEnvironment struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	In      bool   `json:"in"`
}

// runner is the Runner of the repository served, the working directory's.
func (s *apiServer) runner() (*release.Runner, error) {
	cfg := s.config()
	if !cfg.Release.Enabled() {
		return nil, errNoRelease
	}
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	r, err := release.New(cfg, dir, s.b)
	if err != nil {
		return nil, err
	}
	if s.releaseTools != nil {
		r.Tools = s.releaseTools
	}
	if s.releaseCache != "" {
		r.CacheDir = s.releaseCache
	}
	return r, nil
}

func writeRunnerError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, errNoRelease) {
		status = http.StatusNotFound
	}
	writeError(w, status, err)
}

// reader is a Runner for a read and the func that ends it, which the
// caller defers and may call earlier: online when the runner is free, else
// Offline, beside the job or write holding it.
func (s *apiServer) reader(w http.ResponseWriter) (*release.Runner, func(), bool) {
	r, err := s.runner()
	if err != nil {
		writeRunnerError(w, err)
		return nil, nil, false
	}
	if !s.runnerMu.TryLock() {
		r.Offline = true
		return r, func() {}, true
	}
	return r, sync.OnceFunc(s.runnerMu.Unlock), true
}

func (s *apiServer) releasing() bool {
	s.releaseMu.Lock()
	defer s.releaseMu.Unlock()
	return s.releaseRun != nil && s.releaseRun.State == "running"
}

// quickWrite runs write holding the runner and answers {}; while a job runs
// it answers 409 releasing. A write isn't cut short by its page closing.
// ponytail: a job starting between the check and the lock makes the write
// wait for it; check and lock under releaseMu if that window ever matters
func (s *apiServer) quickWrite(w http.ResponseWriter, write func(ctx context.Context) error) {
	if s.releasing() {
		writeCoded(w, http.StatusConflict, "releasing", errReleasing)
		return
	}
	err := func() error {
		s.runnerMu.Lock()
		defer s.runnerMu.Unlock()
		return write(context.Background())
	}()
	switch {
	case errors.Is(err, release.ErrNoTag) || errors.Is(err, release.ErrNoRecord):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, release.ErrBadProposal):
		writeError(w, http.StatusBadRequest, err)
	case err != nil: // a fetch or a push
		writeError(w, http.StatusInternalServerError, err)
	default:
		writeJSON(w, http.StatusOK, struct{}{})
	}
}

// startJob runs f in the background as the job, answering 202 and the job,
// or 409 releasing while one runs. A tag f returns becomes the subject.
func (s *apiServer) startJob(w http.ResponseWriter, r *release.Runner, kind, subject string, f func(ctx context.Context, r *release.Runner) (string, error)) {
	s.releaseMu.Lock()
	if s.releaseRun != nil && s.releaseRun.State == "running" {
		s.releaseMu.Unlock()
		writeCoded(w, http.StatusConflict, "releasing", errReleasing)
		return
	}
	job := &releaseJob{Kind: kind, Subject: subject, State: "running", Steps: []release.Step{}, Started: time.Now()}
	s.releaseRun = job
	started := *job
	s.releaseMu.Unlock()
	go s.runJob(job, r, f)
	writeJSON(w, http.StatusAccepted, started)
}

func (s *apiServer) runJob(job *releaseJob, r *release.Runner, f func(ctx context.Context, r *release.Runner) (string, error)) {
	s.runnerMu.Lock()
	defer s.runnerMu.Unlock()
	r.Step = func(st release.Step) {
		s.releaseMu.Lock()
		if n := len(job.Steps); n > 0 && job.Steps[n-1].Name == st.Name && job.Steps[n-1].State == "running" {
			job.Steps[n-1] = st
		} else {
			job.Steps = append(job.Steps, st)
		}
		s.releaseMu.Unlock()
		s.events.publish(event{Kind: "releases"})
	}
	r.OnTag = func(tag string) {
		s.releaseMu.Lock()
		job.Tag = tag
		s.releaseMu.Unlock()
	}
	tag, err := f(context.Background(), r)
	s.releaseMu.Lock()
	now := time.Now()
	job.State, job.Ended = "done", &now
	if tag != "" {
		job.Subject = tag
	}
	if err != nil {
		job.State, job.Error, job.Code = "failed", err.Error(), jobCode(err)
	}
	s.releaseMu.Unlock()
	s.seeRefs() // the records it wrote: ours, not an outside change
	s.events.publish(event{Kind: "releases"})
}

// jobCode is what the page acts on in a job's error.
func jobCode(err error) string {
	switch {
	case errors.Is(err, release.ErrTaken):
		return "taken"
	case errors.Is(err, release.ErrOlder):
		return "older"
	case errors.Is(err, release.ErrNotAdopted):
		return "not-adopted"
	}
	return ""
}

// jobSnapshot is a copy of the job, nil when none ran.
func (s *apiServer) jobSnapshot() *releaseJob {
	s.releaseMu.Lock()
	defer s.releaseMu.Unlock()
	if s.releaseRun == nil {
		return nil
	}
	j := *s.releaseRun
	j.Steps = slices.Clone(j.Steps)
	return &j
}

// orEmpty is s, or [] rather than null.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func (s *apiServer) handleReleases(w http.ResponseWriter, r *http.Request) {
	if !s.config().Release.Enabled() {
		writeJSON(w, http.StatusOK, map[string]bool{"enabled": false})
		return
	}
	run, done, ok := s.reader(w)
	if !ok {
		return
	}
	defer done()
	st, err := run.Status(r.Context())
	done()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, releasesResponse{Enabled: true, Remote: run.Cfg.RemoteName(), Trunk: run.BaseBranch(), Environments: orEmpty(st.Envs), Releases: orEmpty(st.Releases), Job: s.jobSnapshot()})
}

func (s *apiServer) handleDraft(w http.ResponseWriter, r *http.Request) {
	run, done, ok := s.reader(w)
	if !ok {
		return
	}
	defer done()
	d, err := run.Draft(r.Context(), r.URL.Query().Get("final") == "1")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *apiServer) handleReleaseJob(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.jobSnapshot())
}

func (s *apiServer) handleProposals(w http.ResponseWriter, r *http.Request) {
	run, done, ok := s.reader(w)
	if !ok {
		return
	}
	defer done()
	ps, err := run.Proposals(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, orEmpty(ps))
}

func (s *apiServer) handleRelease(w http.ResponseWriter, r *http.Request) {
	run, done, ok := s.reader(w)
	if !ok {
		return
	}
	defer done()
	st, err := run.Status(r.Context())
	done() // the rest reads what Status fetched
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	d, found, err := detail(r.Context(), run, st, r.PathValue("tag"))
	switch {
	case err != nil:
		writeError(w, http.StatusInternalServerError, err)
	case !found:
		writeError(w, http.StatusNotFound, fmt.Errorf("no release tag %s", r.PathValue("tag")))
	default:
		writeJSON(w, http.StatusOK, d)
	}
}

// detail is the page of release tag name in st, read without fetching;
// found is false when no release tag has the name.
// ponytail: reopens the config repos Status opened (a stat each); Status
// returning them is the upgrade if it ever shows
func detail(ctx context.Context, run *release.Runner, st release.Status, name string) (d releaseDetail, found bool, err error) {
	i := slices.IndexFunc(st.Releases, func(row release.ReleaseRow) bool { return row.Tag.Name == name })
	if i < 0 {
		return d, false, nil
	}
	d.Row = st.Releases[i]
	final := d.Row.Tag.Version.Final()
	d.Candidates = []release.ReleaseRow{}
	for _, row := range st.Releases {
		if v := row.Tag.Version; v.RC != 0 && v.Final() == final && row.Tag.Name != name {
			d.Candidates = append(d.Candidates, row)
		}
	}
	if d.Issues, err = run.Issues(ctx, d.Row.Tag); err != nil {
		return d, true, err
	}
	d.Issues = orEmpty(d.Issues)
	d.History = map[string][]historyEntry{}
	for _, e := range st.Envs {
		for _, t := range e.Targets {
			c, err := release.OpenConfigRepo(ctx, run.CacheDir, t.Repo)
			if err != nil {
				return d, true, err
			}
			// ponytail: the overlay's last 1000 changes, as the cut's candidate check reads
			h, err := c.History(ctx, t.Path, 1000)
			if err != nil {
				return d, true, err
			}
			for _, ch := range h {
				if ch.Version == name {
					d.History[e.Name] = append(d.History[e.Name], historyEntry{Change: ch, Target: t.Name})
				}
			}
		}
	}
	return d, true, nil
}

// handleIssueEnvironments answers, for each environment, whether it runs
// a branch of the issue merged into the base: its head is an ancestor of
// the environment's commit. Split or not adopted, an environment has none.
// It reads offline (R25), so it never waits for the runner.
func (s *apiServer) handleIssueEnvironments(w http.ResponseWriter, r *http.Request) {
	run, err := s.runner()
	if err != nil {
		writeRunnerError(w, err)
		return
	}
	run.Offline = true
	is, err := s.b.GetIssue(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	st, err := run.Status(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	_, linked, err := s.branchesOf(is)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	res := []issueEnvironment{}
	for _, e := range st.Envs {
		env := issueEnvironment{Name: e.Name, Version: e.Version}
		for _, l := range linked {
			if !l.Merged || e.Commit == "" || env.In {
				continue
			}
			if env.In, err = release.IsAncestor(r.Context(), run.Dir, l.LastCommit.SHA, e.Commit); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}
		res = append(res, env)
	}
	writeJSON(w, http.StatusOK, res)
}

// The writes: each answers 404 without [release], then 400 for what the
// request got wrong, 404 for a tag or environment that isn't there, then
// 409 releasing while a job runs.

func (s *apiServer) handleReleaseNotes(w http.ResponseWriter, r *http.Request) {
	run, err := s.runner()
	if err != nil {
		writeRunnerError(w, err)
		return
	}
	var req struct {
		Notes string `json:"notes"`
	}
	if !decode(w, r, &req) {
		return
	}
	s.quickWrite(w, func(ctx context.Context) error { return run.SetNotes(ctx, r.PathValue("tag"), req.Notes) })
}

func (s *apiServer) handleReleaseComment(w http.ResponseWriter, r *http.Request) {
	run, err := s.runner()
	if err != nil {
		writeRunnerError(w, err)
		return
	}
	var req struct {
		Message string `json:"message"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Message == "" {
		writeError(w, http.StatusBadRequest, errors.New("message is required"))
		return
	}
	s.quickWrite(w, func(ctx context.Context) error { return run.Comment(ctx, r.PathValue("tag"), req.Message) })
}

func (s *apiServer) handleYank(w http.ResponseWriter, r *http.Request) {
	run, err := s.runner()
	if err != nil {
		writeRunnerError(w, err)
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Reason == "" {
		writeError(w, http.StatusBadRequest, errors.New("a reason is required"))
		return
	}
	s.quickWrite(w, func(ctx context.Context) error { return run.Yank(ctx, r.PathValue("tag"), req.Reason) })
}

func (s *apiServer) handleAdopt(w http.ResponseWriter, r *http.Request) {
	run, err := s.runner()
	if err != nil {
		writeRunnerError(w, err)
		return
	}
	var req struct {
		Proposals []release.Proposal `json:"proposals"`
	}
	if !decode(w, r, &req) {
		return
	}
	s.quickWrite(w, func(ctx context.Context) error { return run.Adopt(ctx, req.Proposals) })
}

func (s *apiServer) handleCut(w http.ResponseWriter, r *http.Request) {
	run, err := s.runner()
	if err != nil {
		writeRunnerError(w, err)
		return
	}
	var req struct {
		Version string `json:"version"` // final form, e.g. 1.4.20; "": the draft's
		Final   bool   `json:"final"`
		Hotfix  bool   `json:"hotfix"`
		Commit  string `json:"commit"`
		Notes   string `json:"notes"`
		Deploy  bool   `json:"deploy"`
	}
	if !decode(w, r, &req) {
		return
	}
	c := release.Cut{Final: req.Final, Hotfix: req.Hotfix, Deploy: req.Deploy, Commit: req.Commit, Notes: req.Notes}
	subject := ""
	if req.Version != "" {
		v, ok := release.ParseTag("v" + strings.TrimPrefix(req.Version, "v"))
		if !ok || v.RC != 0 {
			writeError(w, http.StatusBadRequest, fmt.Errorf("version %q: want a final version like 1.4.20; the rc number is chosen for you", req.Version))
			return
		}
		c.Version, subject = v, v.Tag()
	}
	// a cut would push the one made offline instead, without these notes
	// or the deploy
	if p, err := release.Pending(r.Context(), run.Dir); err != nil || len(p) > 0 {
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeCoded(w, http.StatusConflict, "pending", fmt.Errorf("%s, cut offline, isn't pushed yet: a deploy, or git-board release cut, pushes it first", p[0].Name))
		return
	}
	s.startJob(w, run, "cut", subject, func(ctx context.Context, run *release.Runner) (string, error) {
		t, err := run.Cut(ctx, c)
		return t.Name, err // pushed, even when a later step failed
	})
}

func (s *apiServer) handlePromote(w http.ResponseWriter, r *http.Request) {
	run, err := s.runner()
	if err != nil {
		writeRunnerError(w, err)
		return
	}
	tag := r.PathValue("tag")
	s.startJob(w, run, "promote", tag, func(ctx context.Context, run *release.Runner) (string, error) {
		_, err := run.Promote(ctx, tag)
		return "", err
	})
}

// environment is the environment a request names, false (answered 404)
// when [release] has no such.
func environment(w http.ResponseWriter, r *http.Request, run *release.Runner) (string, bool) {
	env := r.PathValue("env")
	if !slices.Contains(run.Cfg.Environments, env) {
		writeError(w, http.StatusNotFound, fmt.Errorf("no environment %s in [release]", env))
		return "", false
	}
	return env, true
}

// releaseTag is the release tag a request names, false when it names none:
// answered 400 when the name isn't a release tag's, 404 when no such tag
// has been fetched.
func releaseTag(w http.ResponseWriter, r *http.Request, run *release.Runner, name string) (release.Tag, bool) {
	if _, ok := release.ParseTag(name); !ok {
		writeError(w, http.StatusBadRequest, fmt.Errorf("%q isn't a release tag", name))
		return release.Tag{}, false
	}
	tags, err := release.ReleaseTags(r.Context(), run.Dir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return release.Tag{}, false
	}
	if i := slices.IndexFunc(tags, func(t release.Tag) bool { return t.Name == name }); i >= 0 {
		return tags[i], true
	}
	writeError(w, http.StatusNotFound, fmt.Errorf("no release tag %q", name))
	return release.Tag{}, false
}

func (s *apiServer) handleDeploy(w http.ResponseWriter, r *http.Request) {
	run, err := s.runner()
	if err != nil {
		writeRunnerError(w, err)
		return
	}
	var req struct {
		Tag string `json:"tag"` // "": the trunk's head, to the followed environment
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) { // {tag?}: a body is optional
		writeError(w, http.StatusBadRequest, err)
		return
	}
	env, ok := environment(w, r, run)
	if !ok {
		return
	}
	if req.Tag == "" {
		if env != run.Cfg.Followed() {
			writeError(w, http.StatusBadRequest, fmt.Errorf("%s doesn't follow the trunk: deploy it a release tag", env))
			return
		}
		s.startJob(w, run, "deploy", env, func(ctx context.Context, run *release.Runner) (string, error) {
			return "", run.DeployFollowed(ctx)
		})
		return
	}
	t, ok := releaseTag(w, r, run, req.Tag)
	if !ok {
		return
	}
	s.startJob(w, run, "deploy", t.Name+" to "+env, func(ctx context.Context, run *release.Runner) (string, error) {
		return "", run.Deploy(ctx, env, release.Target{Commit: t.Commit, Version: t.Name})
	})
}

func (s *apiServer) handleRollback(w http.ResponseWriter, r *http.Request) {
	run, err := s.runner()
	if err != nil {
		writeRunnerError(w, err)
		return
	}
	var req struct {
		Tag    string `json:"tag"`
		Reason string `json:"reason"` // kept on the record of what env ran
	}
	if !decode(w, r, &req) {
		return
	}
	env, ok := environment(w, r, run)
	if !ok {
		return
	}
	if req.Tag == "" || req.Reason == "" {
		writeError(w, http.StatusBadRequest, errors.New("a rollback takes a tag and a reason"))
		return
	}
	if _, ok := releaseTag(w, r, run, req.Tag); !ok {
		return
	}
	s.startJob(w, run, "rollback", env+" to "+req.Tag, func(ctx context.Context, run *release.Runner) (string, error) {
		return "", run.Rollback(ctx, env, req.Tag, req.Reason)
	})
}
