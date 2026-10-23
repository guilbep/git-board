package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/git-bug/git-bug/cache"
	"github.com/git-bug/git-bug/repository/repotest"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

func getWork(t *testing.T, ts *httptest.Server, who string) workResponse {
	t.Helper()
	var res workResponse
	if code := call(t, "GET", ts.URL+"/api/work?who="+who, nil, &res); code != http.StatusOK {
		t.Fatalf("GET /api/work: %d", code)
	}
	return res
}

func needs(res workResponse) []string {
	var out []string
	for _, r := range res.Needs {
		out = append(out, r.Issue.ID)
	}
	return out
}

func waiting(res workResponse) []string {
	var out []string
	for _, r := range res.Waiting {
		out = append(out, r.Issue.ID)
	}
	return out
}

// issueBy opens an issue as who, with labels, since NewIssue writes as the
// test identity.
func issueBy(t *testing.T, b *gitbugio.Backend, who *cache.IdentityCache, title string, labels ...string) string {
	t.Helper()
	bug, _, err := b.TestCache().Bugs().NewRaw(who, time.Now().Unix(), title, twoCriteria, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := bug.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	id := bug.Id().String()
	if _, err := b.ApplyLabelChange(id, labels, nil); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestWorkGroups(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	cfg := cycleConfig(map[string]string{"pm": "guilbep", "lead-dev": "ann"}, config.Cycle{Risk: "pm", Plan: "lead-dev"})
	ts := newTestServer(t, b, cfg)
	ann := gitbugiotest.NewPerson(t, b, "Ann", "ann")
	risk, _ := b.NewIssue("Needs a risk", twoCriteria, []string{"status:ready"})                // step 2, waits for the pm: me
	plan, _ := b.NewIssue("Needs a plan", twoCriteria, []string{"status:ready", "risk:normal"}) // step 3, waits for ann; mine as author
	anns := issueBy(t, b, ann, "Ann's", "status:ready", "risk:normal", "pm:ann")                // waits for ann; the inherited pm would make it mine
	closed, _ := b.NewIssue("Closed", twoCriteria, []string{"status:ready"})
	gitbugiotest.CloseIssue(t, b, closed)

	res := getWork(t, ts, "me")
	if !slices.Equal(needs(res), []string{risk}) || !slices.Equal(waiting(res), []string{plan}) {
		t.Errorf("me: needs %v, waiting %v", needs(res), waiting(res))
	}
	if r := res.Needs[0]; r.Step != 2 || !r.Mine || r.Waits.What != "set the risk class" || !slices.Equal(r.Actions, []string{"set-risk"}) || !r.Involved || r.Since.IsZero() {
		t.Errorf("the risk row: %+v", r)
	}
	if res.Me.Login != "guilbep" {
		t.Errorf("me: %+v", res.Me)
	}
	res = getWork(t, ts, "team")
	if !slices.Equal(needs(res), []string{risk}) || !slices.Equal(slices.Sorted(slices.Values(waiting(res))), slices.Sorted(slices.Values([]string{plan, anns}))) { // same second: the order is the ids'

		t.Errorf("team: needs %v, waiting %v", needs(res), waiting(res))
	}
}

func TestWorkAnyonesGate(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	ts := newTestServer(t, b, cycleConfig(nil, config.Cycle{}))
	id, _ := b.NewIssue("Unassigned", twoCriteria, []string{"status:in-progress", "risk:low"}) // step 4, no assignee: anyone
	if res := getWork(t, ts, "me"); len(res.Needs) != 0 || !slices.Equal(waiting(res), []string{id}) {
		t.Errorf("me: needs %v, waiting %v", needs(res), waiting(res))
	}
	if res := getWork(t, ts, "team"); !slices.Equal(needs(res), []string{id}) || !res.Needs[0].Waits.Anyone {
		t.Errorf("team: needs %v, waiting %v", needs(res), waiting(res))
	}
}

func TestWorkQuestion(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	ts := newTestServer(t, b, cycleConfig(map[string]string{"lead-dev": "ann"}, config.Cycle{Plan: "lead-dev"}))
	ann := gitbugiotest.NewPerson(t, b, "Ann", "ann")
	id, _ := b.NewIssue("Asked", twoCriteria, []string{"status:ready", "risk:normal"}) // waits for ann
	cid := gitbugiotest.CommentAs(t, b, id, ann, time.Now().Unix(), "Which tolerance?")
	res := getWork(t, ts, "me")
	if !slices.Equal(needs(res), []string{id}) || res.Needs[0].Question == nil || res.Needs[0].Question.ID != cid || res.Needs[0].Question.By.Login != "ann" {
		t.Fatalf("asked: needs %v %+v", needs(res), res.Needs)
	}
	if _, err := b.AddComment(id, "2mm."); err != nil {
		t.Fatal(err)
	}
	if res := getWork(t, ts, "me"); len(res.Needs) != 0 || !slices.Equal(waiting(res), []string{id}) {
		t.Errorf("answered: needs %v, waiting %v", needs(res), waiting(res))
	}
}

func TestWorkNeedsWhoYouAre(t *testing.T) {
	t.Parallel()
	noLogin := &apiServer{cfg: &config.Config{}, b: gitbugiotest.New(t)}
	ts := httptest.NewServer(withCORS(newMux(noLogin, http.NotFoundHandler())))
	t.Cleanup(ts.Close)
	if code, c := codeOf(t, ts.URL+"/api/work"); code != http.StatusConflict || c != "no-login" {
		t.Errorf("no login: %d %q", code, c)
	}
	b, err := gitbugio.NewForRepo(repotest.NewGoGitRepo(t, false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	ts2 := httptest.NewServer(withCORS(newMux(&apiServer{cfg: &config.Config{}, b: b, me: "guilbep"}, http.NotFoundHandler())))
	t.Cleanup(ts2.Close)
	if code, c := codeOf(t, ts2.URL+"/api/work"); code != http.StatusConflict || c != "no-identity" {
		t.Errorf("no identity: %d %q", code, c)
	}
}

// Review Focus 2: a worktree names its issue by branch; detached or
// unmatched, it is a row with no issue.
func TestWorkAgents(t *testing.T) {
	s, _, id := gateRepo(t, "https://github.com/o/r.git", false)
	s.cfg = cycleConfig(nil, config.Cycle{})
	if _, err := s.b.ApplyLabelChange(id, []string{"status:in-progress", "risk:low"}, nil); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(s.gitDir)
	ts := serve(t, s)
	if res := getWork(t, ts, "me"); len(res.Agents) != 0 {
		t.Fatalf("no worktree: %+v", res.Agents)
	}
	wt, d := filepath.Join(t.TempDir(), "wt"), filepath.Join(t.TempDir(), "d")
	git(t, dir, "worktree", "add", "-q", wt, "fix/2518-x")
	git(t, dir, "worktree", "add", "-q", "--detach", d)
	res := getWork(t, ts, "me")
	if len(res.Agents) != 2 {
		t.Fatalf("agents: %+v", res.Agents)
	}
	if a := res.Agents[0]; a.Branch != "fix/2518-x" || a.Issue == nil || a.Issue.ID != id || a.Step != 4 || a.State != "Waits for anyone to write the tests" || a.At.IsZero() {
		t.Errorf("on the branch: %+v", a)
	}
	if a := res.Agents[1]; len(a.Branch) != 7 || a.Issue != nil || a.State != "No issue" {
		t.Errorf("detached: %+v", a)
	}
}

func TestWorkToSend(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	ts := newTestServer(t, b, cycleConfig(nil, config.Cycle{}))
	author, err := b.TestCache().GetUserIdentity(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	bug, _, err := b.TestCache().Bugs().NewRaw(author, time.Now().Unix(), "synced", "", nil, map[string]string{
		"sync:github.com/o/r:id": "I_1", "sync:github.com/o/r:url": "https://github.com/o/r/issues/7", "sync:github.com/o/r:number": "7",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := bug.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	id := bug.Id().String()
	if res := getWork(t, ts, "me"); res.Pending != 0 || len(res.ToSend) != 0 {
		t.Fatalf("nothing to send yet: %d %+v", res.Pending, res.ToSend)
	}
	if _, err := b.AddComment(id, "not posted"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second) // git-bug's operation times are whole seconds; newest first needs the order
	if _, err := b.ApplyLabelChange(id, []string{"risk:normal"}, nil); err != nil {
		t.Fatal(err)
	}
	res := getWork(t, ts, "me")
	if res.Pending != 2 || len(res.ToSend) != 2 || res.ToSend[0].What != "the risk" || res.ToSend[1].What != "a comment" || res.ToSend[0].Issue.ID != id {
		t.Errorf("to send: %d %+v", res.Pending, res.ToSend)
	}
}

// Review Focus 5: no [sprints], no retro clock: the summary says so.
func TestWorkWithoutSprints(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	cfg := cycleConfig(nil, config.Cycle{})
	ts := newTestServer(t, b, cfg)
	id, _ := b.NewIssue("Corrected", twoCriteria, []string{"status:ready"})
	if _, err := b.AddCommentMeta(id, "The agent dropped the filter", map[string]string{"correction": "1"}); err != nil {
		t.Fatal(err)
	}
	if res := getWork(t, ts, "me"); res.Retro.Sprint != "" || res.Retro.Corrections != 0 {
		t.Errorf("without sprints: %+v", res.Retro)
	}
	cfg.Sprints = config.SprintsConfig{LengthDays: 14, List: []config.Sprint{{Slug: "s1", Title: "Sprint 1", Start: time.Now().AddDate(0, 0, -3)}}}
	if res := getWork(t, ts, "me"); res.Retro.Sprint != "s1" || res.Retro.Corrections != 1 || res.Retro.Since.IsZero() {
		t.Errorf("in sprint 1: %+v", res.Retro)
	}
}

// The page refreshes the PRs once; the cycle of one of its issues reads
// the cache.
func TestWorkAsksGitHubOnce(t *testing.T) {
	s, gh, id := prRepo(t, "in-review")
	ts := serve(t, s)
	res := getWork(t, ts, "team")
	if !slices.Equal(needs(res), []string{id}) || res.Needs[0].Step != 6 || res.Needs[0].Branch != "fix/2518-x" {
		t.Fatalf("in review: needs %v %+v", needs(res), res.Needs)
	}
	asked := len(gh.sent())
	if asked == 0 {
		t.Fatal("GitHub was not asked")
	}
	getWork(t, ts, "team")
	getJSON(t, ts.URL+"/api/issues/"+id[:8]+"/cycle", &cycleAnswer{})
	if len(gh.sent()) != asked {
		t.Errorf("asked again: %v", gh.sent())
	}
}
