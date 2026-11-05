package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/guilbep/git-board/app"
	"github.com/guilbep/git-board/domain/releases"
	"github.com/guilbep/git-board/tools/ship"
)

// Releases (docs/rewrite/api.md, routes 43 to 48): what runs where and
// the releases, a release's page, the quick writes on its record, and
// the jobs that move releases (cut, promote, deploy, rollback), which run
// in the background, one at a time, each step a `release` event.
//
// relMu serializes the runner's fetches and writes: a job holds it for
// its whole run, a quick write for its own; a read takes it when free (it
// fetches) or reads offline, what was last fetched, beside a job.

// Job is the release job running, or the last one.
type Job struct {
	Kind    string      `json:"kind"`    // cut, promote, deploy, rollback
	Subject string      `json:"subject"` // dev, v1.0.1-rc.1 to stage, prod to v1.0.0; a cut's version, then its tag
	State   string      `json:"state"`   // running, done, failed
	Error   string      `json:"error,omitempty"`
	Code    string      `json:"code,omitempty"` // a failed job's: taken, older, not-adopted
	Tag     string      `json:"tag,omitempty"`  // the tag a cut or promote pushed or tried, once known
	Steps   []ship.Step `json:"steps"`          // each step once, as last reported
	Started time.Time   `json:"started_at"`
	Ended   time.Time   `json:"ended_at,omitzero"`
}

type releasesRes struct {
	Enabled      bool              `json:"enabled"`
	Remote       string            `json:"remote"` // the authority, [release] remote
	Trunk        string            `json:"trunk"`  // the trunk branch, dev
	Environments []ship.EnvState   `json:"environments"`
	Binaries     []string          `json:"binaries"` // [release] binaries: a cut builds and publishes their archives
	Releases     []ship.ReleaseRow `json:"releases"`
	Proposals    []ship.Proposal   `json:"proposals"` // the commits the overlays not adopted run, proposed
	Job          *Job              `json:"job"`
}

// releaseRes is a release's page.
type releaseRes struct {
	Row        ship.ReleaseRow           `json:"row"`
	Issues     []ship.DraftIssue         `json:"issues"`     // what shipped
	Candidates []ship.ReleaseRow         `json:"candidates"` // the rc rows of its final, but itself
	History    map[string][]historyEntry `json:"history"`    // env → its overlays' changes naming this version
}

type historyEntry struct {
	ship.Change
	Target string `json:"target"`
}

var errNoRelease = &app.Error{Class: app.NotFound, Err: app.ErrNoRelease}

// runner is the board's release runner.
func runner(ctx context.Context) (*ship.Runner, error) {
	r, err := board(ctx).Runner(ctx)
	if errors.Is(err, app.ErrNoRelease) {
		return nil, errNoRelease
	}
	return r, err
}

// reader is a runner for a read and the func that ends it: online when
// the runner is free, else offline beside the job or write holding it.
func (s *Server) reader(ctx context.Context) (*ship.Runner, func(), error) {
	r, err := runner(ctx)
	if err != nil {
		return nil, nil, err
	}
	if !s.relMu.TryLock() {
		r.Offline = true
		return r, func() {}, nil
	}
	return r, sync.OnceFunc(s.relMu.Unlock), nil
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

type releasesReq struct {
	Local bool `query:"local"`
}

// releases is route 43; local: this clone's, at once, without a fetch.
func (s *Server) releases(ctx context.Context, q releasesReq) (releasesRes, error) {
	res := releasesRes{Environments: []ship.EnvState{}, Binaries: []string{}, Releases: []ship.ReleaseRow{}, Proposals: []ship.Proposal{}, Job: s.lastJob()}
	if !board(ctx).Config.Release.Enabled() {
		return res, nil // the page's shape still, its lists empty
	}
	r, done, err := s.reader(ctx)
	if err != nil {
		return res, err
	}
	defer done()
	r.Offline = r.Offline || q.Local
	st, err := r.Status(ctx)
	if err != nil {
		return res, err
	}
	res.Enabled, res.Remote, res.Trunk = true, r.Cfg.RemoteName(), r.BaseBranch()
	res.Environments, res.Binaries, res.Releases = orEmpty(st.Envs), orEmpty(r.Cfg.Binaries), orEmpty(st.Releases)
	if slices.ContainsFunc(st.Envs, func(e ship.EnvState) bool { return !e.Adopted && len(e.Targets) > 0 }) {
		r.Offline = true // fetched just now
		ps, err := r.Proposals(ctx)
		if err != nil {
			return res, err
		}
		res.Proposals = orEmpty(ps)
	}
	return res, nil
}

type releaseReq struct {
	Tag   string `path:"tag"`
	Final bool   `query:"final"`
}

// release is route 45: a release's page; the tag next is the draft of
// the next cut (final: of the final).
func (s *Server) release(ctx context.Context, q releaseReq) (any, error) {
	r, done, err := s.reader(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	if q.Tag == "next" {
		return r.Draft(ctx, q.Final)
	}
	st, err := r.Status(ctx)
	done() // the rest reads what Status fetched
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(st.Releases, func(row ship.ReleaseRow) bool { return row.Tag.Name == q.Tag })
	if i < 0 {
		return nil, &app.Error{Class: app.NotFound, Err: fmt.Errorf("no release tag %s", q.Tag)}
	}
	d := releaseRes{Row: st.Releases[i], Candidates: []ship.ReleaseRow{}, History: map[string][]historyEntry{}}
	final := d.Row.Tag.Version.Final()
	for _, row := range st.Releases {
		if v := row.Tag.Version; v.RC != 0 && v.Final() == final && row.Tag.Name != q.Tag {
			d.Candidates = append(d.Candidates, row)
		}
	}
	if d.Issues, err = r.Issues(ctx, d.Row.Tag); err != nil {
		return nil, err
	}
	d.Issues = orEmpty(d.Issues)
	for _, e := range st.Envs {
		for _, t := range e.Targets {
			c, err := ship.OpenConfigRepo(ctx, r.CacheDir, t.Repo)
			if err != nil {
				return nil, err
			}
			h, err := c.History(ctx, t.Path, 1000) // the last 1000 changes, as a cut's check reads
			if err != nil {
				return nil, err
			}
			for _, ch := range h {
				if ch.Version == q.Tag {
					d.History[e.Name] = append(d.History[e.Name], historyEntry{Change: ch, Target: t.Name})
				}
			}
		}
	}
	return d, nil
}

// quickWrite runs write holding the runner; busy while a job runs.
func (s *Server) quickWrite(ctx context.Context, tag string, write func(r *ship.Runner) error) (struct{}, error) {
	r, err := runner(ctx)
	if err != nil {
		return struct{}{}, err
	}
	if s.releasing() {
		return struct{}{}, errReleasing
	}
	err = func() error {
		s.relMu.Lock()
		defer s.relMu.Unlock()
		return write(r)
	}()
	switch {
	case errors.Is(err, ship.ErrNoTag), errors.Is(err, ship.ErrNoRecord):
		err = &app.Error{Class: app.NotFound, Err: err}
	case errors.Is(err, ship.ErrBadProposal):
		err = &app.Error{Class: app.Usage, Err: err}
	}
	if err == nil {
		s.publish(ctx, Event{Kind: "release", ID: tag, By: clientOf(ctx)})
	}
	return struct{}{}, err
}

var errReleasing = &app.Error{Class: app.Busy, Err: errors.New("a release job is running: wait for it to end")}

type patchReleaseReq struct {
	Tag    string          `path:"tag"`
	Notes  *string         `json:"notes"`
	State  string          `json:"state"`  // promoted, yanked
	Reason string          `json:"reason"` // a yank's
	Adopt  []ship.Proposal `json:"adopt"`  // the commits its overlays run
}

// patchRelease is route 46: its notes, yanked with a reason, adopted by
// the overlays running it, or promoted (a job, 202).
func (s *Server) patchRelease(ctx context.Context, q patchReleaseReq) (any, error) {
	switch q.State {
	case "", "yanked":
	case "promoted":
		if err := runsCode(ctx, "releases are promoted"); err != nil {
			return nil, err
		}
		if _, err := runner(ctx); err != nil {
			return nil, err
		}
		return s.startJob(ctx, "promote", q.Tag, func(ctx context.Context, r *ship.Runner) (string, error) {
			_, err := r.Promote(ctx, q.Tag)
			return "", err
		})
	default:
		return nil, invalid("state", fmt.Errorf("state %q: promoted or yanked", q.State))
	}
	if q.State == "yanked" && strings.TrimSpace(q.Reason) == "" {
		return nil, invalid("reason", errors.New("a yank takes a reason"))
	}
	if q.Notes == nil && q.State == "" && q.Adopt == nil {
		return nil, invalid("", errors.New("nothing to change: notes, state or adopt"))
	}
	if q.Adopt != nil {
		// adopting writes the overlays' config repos, which say what runs
		if err := runsCode(ctx, "overlays are adopted"); err != nil {
			return nil, err
		}
	}
	return s.quickWrite(ctx, q.Tag, func(r *ship.Runner) error {
		if q.Adopt != nil {
			if err := r.Adopt(ctx, q.Adopt); err != nil {
				return err
			}
		}
		if q.Notes != nil {
			if err := r.SetNotes(ctx, q.Tag, *q.Notes); err != nil {
				return err
			}
		}
		if q.State == "yanked" {
			return r.Yank(ctx, q.Tag, q.Reason)
		}
		return nil
	})
}

type releaseCommentReq struct {
	Tag  string `path:"tag"`
	Body string `json:"body"`
}

// releaseComment is route 47.
func (s *Server) releaseComment(ctx context.Context, q releaseCommentReq) (struct{}, error) {
	if strings.TrimSpace(q.Body) == "" {
		return struct{}{}, invalid("body", errors.New("a comment takes a body"))
	}
	return s.quickWrite(ctx, q.Tag, func(r *ship.Runner) error { return r.Comment(ctx, q.Tag, q.Body) })
}

type cutReq struct {
	Version string `json:"version"` // final form, 1.4.20; "": the draft's
	Final   bool   `json:"final"`
	Hotfix  bool   `json:"hotfix"`
	Commit  string `json:"commit"`
	Notes   string `json:"notes"`
	Deploy  bool   `json:"deploy"`
}

// cut is route 44: a job, 202.
func (s *Server) cut(ctx context.Context, q cutReq) (Reply, error) {
	if err := runsCode(ctx, "releases are cut"); err != nil {
		return Reply{}, err
	}
	r, err := runner(ctx)
	if err != nil {
		return Reply{}, err
	}
	c := ship.Cut{Final: q.Final, Hotfix: q.Hotfix, Deploy: q.Deploy, Commit: q.Commit, Notes: q.Notes}
	subject := ""
	if q.Version != "" {
		v, ok := releases.ParseTag("v" + strings.TrimPrefix(q.Version, "v"))
		if !ok || v.RC != 0 {
			return Reply{}, invalid("version", fmt.Errorf("version %q: want a final version like 1.4.20; the rc number is chosen for you", q.Version))
		}
		c.Version, subject = v, v.Tag()
	}
	// a cut would push the one made offline instead, without these notes
	// or the deploy
	if p, err := ship.Pending(ctx, r.Dir); err != nil {
		return Reply{}, err
	} else if len(p) > 0 {
		return Reply{}, &app.Error{Class: app.Busy, Err: fmt.Errorf("%s, cut offline, isn't pushed yet: a deploy, or git-board release cut, pushes it first", p[0].Name)}
	}
	return s.startJob(ctx, "cut", subject, func(ctx context.Context, r *ship.Runner) (string, error) {
		t, err := r.Cut(ctx, c)
		return t.Name, err // pushed, even when a later step failed
	})
}

type deployReq struct {
	Env    string `path:"env"`
	Tag    string `json:"tag"`    // "": the trunk's head, to the followed environment
	Reason string `json:"reason"` // a rollback's: an older tag, kept on the record
}

// deploy is route 48: a deploy, or with reason a rollback; a job, 202.
func (s *Server) deploy(ctx context.Context, q deployReq) (Reply, error) {
	if err := runsCode(ctx, "environments are deployed"); err != nil {
		return Reply{}, err
	}
	r, err := runner(ctx)
	if err != nil {
		return Reply{}, err
	}
	if !slices.Contains(r.Cfg.Environments, q.Env) {
		return Reply{}, &app.Error{Class: app.NotFound, Err: fmt.Errorf("no environment %s in [release]", q.Env)}
	}
	if q.Tag == "" {
		if q.Reason != "" {
			return Reply{}, invalid("tag", errors.New("a rollback takes the tag to go back to"))
		}
		if q.Env != r.Cfg.Followed() {
			return Reply{}, invalid("tag", fmt.Errorf("%s doesn't follow the trunk: deploy it a release tag", q.Env))
		}
		return s.startJob(ctx, "deploy", q.Env, func(ctx context.Context, r *ship.Runner) (string, error) {
			return "", r.DeployFollowed(ctx)
		})
	}
	if _, ok := releases.ParseTag(q.Tag); !ok {
		return Reply{}, invalid("tag", fmt.Errorf("%q isn't a release tag", q.Tag))
	}
	tags, err := ship.ReleaseTags(ctx, r.Dir)
	if err != nil {
		return Reply{}, err
	}
	i := slices.IndexFunc(tags, func(t releases.Tag) bool { return t.Name == q.Tag })
	if i < 0 {
		return Reply{}, &app.Error{Class: app.NotFound, Err: fmt.Errorf("no release tag %q", q.Tag)}
	}
	t := tags[i]
	if q.Reason != "" {
		return s.startJob(ctx, "rollback", q.Env+" to "+q.Tag, func(ctx context.Context, r *ship.Runner) (string, error) {
			return "", r.Rollback(ctx, q.Env, q.Tag, q.Reason)
		})
	}
	return s.startJob(ctx, "deploy", t.Name+" to "+q.Env, func(ctx context.Context, r *ship.Runner) (string, error) {
		return "", r.Deploy(ctx, q.Env, ship.Target{Commit: t.Commit, Version: t.Name})
	})
}

func (s *Server) releasing() bool {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	return s.job != nil && s.job.State == "running"
}

// lastJob is a copy of the job running, or the last one; nil for none.
func (s *Server) lastJob() *Job {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	if s.job == nil {
		return nil
	}
	j := *s.job
	j.Steps = slices.Clone(j.Steps)
	return &j
}

// startJob runs f in the background on a board of its own, answering 202
// and the job, or busy while one runs. A tag f returns becomes the
// subject.
func (s *Server) startJob(ctx context.Context, kind, subject string, f func(ctx context.Context, r *ship.Runner) (string, error)) (Reply, error) {
	s.jobMu.Lock()
	if s.job != nil && s.job.State == "running" {
		s.jobMu.Unlock()
		return Reply{}, errReleasing
	}
	job := &Job{Kind: kind, Subject: subject, State: "running", Steps: []ship.Step{}, Started: time.Now()}
	s.job = job
	started := *job
	s.jobMu.Unlock()
	h := s.boards.get()
	by := clientOf(ctx)
	// the request's runner, on the job's board: the request's ends with it
	r, err := h.b.Runner(ctx)
	if err != nil {
		s.boards.release(h)
		s.jobMu.Lock()
		job.State, job.Error, job.Ended = "failed", err.Error(), time.Now()
		s.jobMu.Unlock()
		return Reply{}, err
	}
	go func() {
		defer s.boards.release(h)
		ctx := context.WithValue(s.base, logKey{}, s.log)
		ctx = context.WithValue(ctx, clientKey{}, by)
		s.runJob(ctx, job, r, f)
	}()
	s.publish(ctx, Event{Kind: "release", By: by})
	return Reply{Status: http.StatusAccepted, Body: started}, nil
}

func (s *Server) runJob(ctx context.Context, job *Job, r *ship.Runner, f func(ctx context.Context, r *ship.Runner) (string, error)) {
	s.relMu.Lock()
	defer s.relMu.Unlock()
	by := clientOf(ctx)
	s.log.Info("release job started", "kind", job.Kind, "subject", job.Subject)
	r.Step = func(st ship.Step) {
		s.jobMu.Lock()
		if n := len(job.Steps); n > 0 && job.Steps[n-1].Name == st.Name && job.Steps[n-1].State == "running" {
			job.Steps[n-1] = st
		} else {
			job.Steps = append(job.Steps, st)
		}
		s.jobMu.Unlock()
		s.publish(ctx, Event{Kind: "release", ID: job.Tag, By: by})
	}
	r.OnTag = func(tag string) {
		s.jobMu.Lock()
		job.Tag = tag
		s.jobMu.Unlock()
	}
	tag, err := func() (tag string, err error) {
		defer func() { // a job runs on its own goroutine: a panic would take the server down
			if p := recover(); p != nil {
				err = fmt.Errorf("panic: %v", p)
			}
		}()
		return f(ctx, r)
	}()
	s.jobMu.Lock()
	job.State, job.Ended = "done", time.Now()
	if tag != "" {
		job.Subject = tag
	}
	if err != nil {
		job.State, job.Error, job.Code = "failed", app.NoCredentials(err.Error()), jobCode(err)
	}
	s.jobMu.Unlock()
	if err != nil {
		s.log.Error("release job failed", "kind", job.Kind, "subject", job.Subject, "error", job.Error)
	} else {
		s.log.Info("release job ended", "kind", job.Kind, "subject", job.Subject)
	}
	s.publish(ctx, Event{Kind: "release", ID: job.Tag, By: by})
}

// jobCode is what the page acts on in a job's error.
func jobCode(err error) string {
	switch {
	case errors.Is(err, ship.ErrTaken):
		return "taken"
	case errors.Is(err, ship.ErrOlder):
		return "older"
	case errors.Is(err, ship.ErrNotAdopted):
		return "not-adopted"
	}
	return ""
}
