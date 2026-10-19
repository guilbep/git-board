package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghpr"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

var co2Statuses = []string{"backlog", "comite", "ready", "in-progress", "in-review", "ready-for-validation", "qa", "tests-metier", "validated"}

// cycleConfig is co2's statuses, the risk field, and roles.
func cycleConfig(roles map[string]string, c config.Cycle) *config.Config {
	return &config.Config{
		Fields: map[string]config.Field{
			"status": {Single: true, Options: co2Statuses},
			"risk":   {Single: true, Options: []string{"low", "normal", "structural"}},
		},
		Roles: roles,
		Cycle: c,
	}
}

const twoCriteria = "## Acceptance criteria\n- Exports a CSV\n- Keeps the filters\n"

type cycleAnswer struct {
	Step    int      `json:"step"`
	Done    bool     `json:"done"`
	Note    string   `json:"note"`
	Mine    bool     `json:"mine"`
	Actions []string `json:"actions"`
	Skipped []int    `json:"skipped"`
	Waits   struct {
		Login string `json:"login"`
		What  string `json:"what"`
	} `json:"waits"`
	Stages struct {
		Ready   string `json:"ready"`
		Guessed bool   `json:"guessed"`
	} `json:"stages"`
	Evidence struct {
		Branch string `json:"branch"`
		PR     *struct {
			Number int `json:"number"`
		} `json:"pr"`
	} `json:"evidence"`
	PRCheckedAt string `json:"pr_checked_at"`
}

func TestCycleOfAnIssue(t *testing.T) {
	b := gitbugiotest.New(t)
	id, err := b.NewIssue("Export CSV", twoCriteria, []string{"status:ready"})
	if err != nil {
		t.Fatal(err)
	}
	ts := newTestServer(t, b, cycleConfig(map[string]string{"pm": "guilbep"}, config.Cycle{Risk: "pm"}))
	var c cycleAnswer
	if resp := getJSON(t, ts.URL+"/api/issues/"+id[:8]+"/cycle", &c); resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if c.Step != 2 || c.Waits.Login != "guilbep" || !c.Mine || !slices.Equal(c.Actions, []string{"set-risk"}) || c.Stages.Ready != "ready" || !c.Stages.Guessed {
		t.Errorf("at ready, no risk: %+v", c)
	}
	if code := call(t, "PATCH", ts.URL+"/api/issues/"+id[:8], map[string]string{"risk": "low"}, nil); code != http.StatusOK {
		t.Fatalf("PATCH risk: %d", code)
	}
	c = cycleAnswer{}
	getJSON(t, ts.URL+"/api/issues/"+id[:8]+"/cycle", &c)
	if c.Step != 3 || !slices.Equal(c.Skipped, []int{3}) {
		t.Errorf("low risk: %+v", c)
	}
	if resp := getJSON(t, ts.URL+"/api/issues/nope/cycle", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown issue: %d", resp.StatusCode)
	}
}

// prRepo is gateRepo with fix/2518-x a commit ahead (the evidence branch,
// on origin too) and the issue at status.
func prRepo(t *testing.T, status string) (*apiServer, *fakeGitHub, string) {
	t.Helper()
	s, gh, id := gateRepo(t, "https://github.com/o/r.git", true)
	dir := filepath.Dir(s.gitDir)
	t.Setenv("GIT_AUTHOR_DATE", "2030-01-01T10:00:00Z")
	t.Setenv("GIT_COMMITTER_DATE", "2030-01-01T10:00:00Z")
	git(t, dir, "switch", "-q", "fix/2518-x")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "Fix")
	git(t, dir, "switch", "-q", "dev")
	git(t, dir, "update-ref", "refs/remotes/origin/fix/2518-x", "fix/2518-x")
	s.cfg = cycleConfig(nil, config.Cycle{})
	if _, err := s.b.ApplyLabelChange(id, []string{"status:" + status, "risk:normal"}, nil); err != nil {
		t.Fatal(err)
	}
	return s, gh, id
}

func TestCycleReadsThePR(t *testing.T) {
	s, gh, id := prRepo(t, "in-review")
	ts := serve(t, s)
	var c cycleAnswer
	getJSON(t, ts.URL+"/api/issues/"+id[:8]+"/cycle", &c)
	if c.Step != 6 || c.Evidence.Branch != "fix/2518-x" || c.Evidence.PR == nil || c.Evidence.PR.Number != 3082 || c.PRCheckedAt == "" {
		t.Fatalf("in-review: %+v", c)
	}
	asked := len(gh.sent())
	if !slices.Contains(gh.sent(), "POST /graphql") {
		t.Errorf("GitHub was asked %v", gh.sent())
	}
	getJSON(t, ts.URL+"/api/issues/"+id[:8]+"/cycle", &cycleAnswer{})
	if len(gh.sent()) != asked {
		t.Errorf("a second read asked GitHub again: %v", gh.sent())
	}

	s, gh, id = prRepo(t, "ready")
	getJSON(t, serve(t, s).URL+"/api/issues/"+id[:8]+"/cycle", &cycleAnswer{})
	if len(gh.sent()) != 0 {
		t.Errorf("at ready, GitHub was asked %v", gh.sent())
	}
}

// Offline the step still computes, and a failure keeps the last answer.
func TestCycleOffline(t *testing.T) {
	s, gh, id := prRepo(t, "in-review")
	setDown := func(down bool) {
		gh.mu.Lock()
		gh.down = down
		gh.mu.Unlock()
		s.pulls.age("fix/2518-x", time.Hour)
	}
	ts := serve(t, s)
	setDown(true)
	var c cycleAnswer
	if resp := getJSON(t, ts.URL+"/api/issues/"+id[:8]+"/cycle", &c); resp.StatusCode != http.StatusOK || c.Step != 6 || c.Evidence.PR != nil {
		t.Fatalf("GitHub down: %d %+v", resp.StatusCode, c)
	}
	setDown(false)
	getJSON(t, ts.URL+"/api/issues/"+id[:8]+"/cycle", &cycleAnswer{})
	setDown(true)
	c = cycleAnswer{}
	getJSON(t, ts.URL+"/api/issues/"+id[:8]+"/cycle", &c)
	if c.Evidence.PR == nil || c.Evidence.PR.Number != 3082 {
		t.Errorf("down again: the last answer is gone: %+v", c)
	}
}

// age makes the branch's entry older, as if read d ago.
func (c *pullCache) age(branch string, d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.m[branch]; ok {
		e.tried = e.tried.Add(-d)
		c.m[branch] = e
	}
}

// post is a JSON POST, and the answer's status and body.
func post(t *testing.T, url string, body any) (int, string) {
	t.Helper()
	data, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(out)
}

// issueAt is an issue with two criteria at status, risk low unless labels
// say otherwise.
func issueAt(t *testing.T, b *gitbugio.Backend, status string, labels ...string) string {
	t.Helper()
	id, err := b.NewIssue("Export CSV", twoCriteria, append([]string{"status:" + status}, labels...))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func lastEvent(t *testing.T, b *gitbugio.Backend, id string) (gitbugio.Thread, gitbugio.Event) {
	t.Helper()
	th, err := b.Thread(id)
	if err != nil {
		t.Fatal(err)
	}
	return th, th.Events[len(th.Events)-1]
}

func TestCycleApprovePlan(t *testing.T) {
	b := gitbugiotest.New(t)
	id := issueAt(t, b, "ready", "risk:normal")
	cfg := cycleConfig(map[string]string{"lead-dev": "guilbep"}, config.Cycle{Plan: "lead-dev"})
	url := newTestServer(t, b, cfg).URL + "/api/issues/" + id[:8] + "/cycle/"

	var bob cycleAnswer
	getJSON(t, serve(t, &apiServer{cfg: cfg, b: b, me: "bob"}).URL+"/api/issues/"+id[:8]+"/cycle", &bob)
	if bob.Step != 3 || bob.Mine || bob.Waits.Login != "guilbep" {
		t.Errorf("bob sees %+v", bob)
	}
	before, _ := lastEvent(t, b, id)
	if code, body := post(t, serve(t, &apiServer{cfg: cfg, b: b, me: "bob"}).URL+"/api/issues/"+id[:8]+"/cycle/approve-plan", struct{}{}); code != http.StatusForbidden || !strings.Contains(body, "waits for the plan role: guilbep") {
		t.Errorf("bob approves: %d %s", code, body)
	}
	if th, _ := lastEvent(t, b, id); len(th.Events) != len(before.Events) {
		t.Errorf("a refused approval wrote %d events", len(th.Events)-len(before.Events))
	}

	var c cycleAnswer
	if code := call(t, "POST", url+"approve-plan", struct{}{}, &c); code != http.StatusOK || c.Step != 3 || c.Note != "plan approved, waits for the status in-progress" {
		t.Errorf("approve: %d %+v", code, c)
	}
	if _, e := lastEvent(t, b, id); e.Meta["plan:approve"] != "1" {
		t.Errorf("last event %+v", e)
	}

	structural := issueAt(t, b, "ready", "risk:structural")
	if code, body := post(t, newTestServer(t, b, cfg).URL+"/api/issues/"+structural[:8]+"/cycle/approve-plan", struct{}{}); code != http.StatusBadRequest || !strings.Contains(body, "the plan PR is approved on GitHub") {
		t.Errorf("structural: %d %s", code, body)
	}
}

func TestCycleSignoffRefused(t *testing.T) {
	b := gitbugiotest.New(t)
	id := issueAt(t, b, "qa", "risk:low")
	cfg := cycleConfig(map[string]string{"pm": "alice"}, config.Cycle{Signoffs: map[string]string{"qa": "pm"}})
	before, _ := lastEvent(t, b, id)
	code, body := post(t, newTestServer(t, b, cfg).URL+"/api/issues/"+id[:8]+"/cycle/signoff", map[string]string{"why": "by hand"})
	if code != http.StatusForbidden || !strings.Contains(body, "waits for the pm role: alice") {
		t.Errorf("guilbep signs off for alice: %d %s", code, body)
	}
	if th, _ := lastEvent(t, b, id); !slices.Contains(th.Labels, "status:qa") || len(th.Events) != len(before.Events) {
		t.Errorf("labels %v, %d new events", th.Labels, len(th.Events)-len(before.Events))
	}
}

func TestCycleSignoffNeedsARun(t *testing.T) {
	b := gitbugiotest.New(t)
	id := issueAt(t, b, "qa", "risk:low")
	cfg := cycleConfig(map[string]string{"pm": "guilbep"}, config.Cycle{Signoffs: map[string]string{"qa": "pm"}})
	url := newTestServer(t, b, cfg).URL + "/api/issues/" + id[:8] + "/cycle/signoff"
	if code, body := post(t, url, struct{}{}); code != http.StatusBadRequest || !strings.Contains(body, "no run; sign off with a reason") {
		t.Errorf("no run: %d %s", code, body)
	}
	if code, body := post(t, url, map[string]string{"why": "ran it by hand"}); code != http.StatusOK {
		t.Fatalf("with a reason: %d %s", code, body)
	}
	th, e := lastEvent(t, b, id)
	signed := th.Events[len(th.Events)-2] // then the status moved
	if signed.Meta["signoff:why"] != "ran it by hand" || signed.Meta["signoff:status"] != "qa" || !slices.Contains(th.Labels, "status:tests-metier") {
		t.Errorf("sign-off %+v, then %+v, labels %v", signed, e, th.Labels)
	}
}

// Once the PR is merged the run happens at the merge commit, not at the
// branch's head: the sign-off's check accepts that commit too.
func TestCycleSignoffAtTheMergedHead(t *testing.T) {
	s, _, id := prRepo(t, "qa")
	s.pulls.put("fix/2518-x", &ghpr.PR{Number: 3082, State: "MERGED", Head: "0000000", MergeCommit: "deadbee0"}, nil)
	url := serve(t, s).URL + "/api/issues/" + id[:8] + "/cycle/"
	if code, body := post(t, url+"verify", map[string]string{"output": "ok", "commit": "deadbee0"}); code != http.StatusOK {
		t.Fatalf("verify: %d %s", code, body)
	}
	if code, body := post(t, url+"signoff", struct{}{}); code != http.StatusOK {
		t.Errorf("sign off after a run at the merged head: %d %s", code, body)
	}
	if th, _ := lastEvent(t, s.b, id); !slices.Contains(th.Labels, "status:tests-metier") {
		t.Errorf("labels %v", th.Labels)
	}
}

func TestCycleVerifyThenSignoff(t *testing.T) {
	b := gitbugiotest.New(t)
	id := issueAt(t, b, "qa", "risk:low")
	url := newTestServer(t, b, cycleConfig(nil, config.Cycle{})).URL + "/api/issues/" + id[:8] + "/cycle/"
	if code, _ := post(t, url+"verify", map[string]string{"output": ""}); code != http.StatusBadRequest {
		t.Errorf("an empty run: %d", code)
	}
	if code, body := post(t, url+"verify", map[string]string{"output": "ok", "commit": ""}); code != http.StatusOK {
		t.Fatalf("verify: %d %s", code, body)
	}
	if _, e := lastEvent(t, b, id); e.Meta["verify:run"] != "1" || e.Body != "Run\n\n```\nok\n```" {
		t.Errorf("the run %+v", e)
	}
	if code, body := post(t, url+"check", map[string]int{"index": 0}); code != http.StatusOK {
		t.Fatalf("check: %d %s", code, body)
	}
	if _, e := lastEvent(t, b, id); e.Meta["verify:criterion"] != "0" || e.Body != "Checked: Exports a CSV" {
		t.Errorf("the check %+v", e)
	}
	if code, _ := post(t, url+"check", map[string]int{"index": 9}); code != http.StatusBadRequest {
		t.Errorf("criterion 9: %d", code)
	}
	if code, body := post(t, url+"signoff", struct{}{}); code != http.StatusOK {
		t.Errorf("signoff: %d %s", code, body)
	}
	if th, _ := lastEvent(t, b, id); th.Events[len(th.Events)-2].Body != "Signed off qa" {
		t.Errorf("the sign-off %q", th.Events[len(th.Events)-2].Body)
	}

	ready := issueAt(t, b, "ready")
	if code, body := post(t, newTestServer(t, b, cycleConfig(nil, config.Cycle{})).URL+"/api/issues/"+ready[:8]+"/cycle/verify", map[string]string{"output": "ok"}); code != http.StatusBadRequest || !strings.Contains(body, "not at Verify") {
		t.Errorf("verify at ready: %d %s", code, body)
	}
}

func TestCycleSignoffCloses(t *testing.T) {
	b := gitbugiotest.New(t)
	id := issueAt(t, b, "tests-metier", "risk:low")
	url := newTestServer(t, b, cycleConfig(nil, config.Cycle{})).URL + "/api/issues/" + id[:8] + "/cycle/"
	if code, body := post(t, url+"verify", map[string]string{"output": "ok"}); code != http.StatusOK {
		t.Fatalf("verify: %d %s", code, body)
	}
	var c cycleAnswer
	if code := call(t, "POST", url+"signoff", struct{}{}, &c); code != http.StatusOK || !c.Done {
		t.Errorf("signoff: %d %+v", code, c)
	}
	if th, _ := lastEvent(t, b, id); th.Status != "closed" || !slices.Contains(th.Labels, "status:validated") {
		t.Errorf("status %s, labels %v", th.Status, th.Labels)
	}
}

func TestCycleBadAction(t *testing.T) {
	b := gitbugiotest.New(t)
	id := issueAt(t, b, "qa")
	url := newTestServer(t, b, cycleConfig(nil, config.Cycle{})).URL + "/api/issues/" + id[:8] + "/cycle/"
	if code, _ := post(t, url+"dance", struct{}{}); code != http.StatusNotFound {
		t.Errorf("dance: %d", code)
	}
	resp, err := http.Post(url+"verify", "text/plain", strings.NewReader(`{"output":"ok"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("not JSON: %d", resp.StatusCode)
	}
}

// With a branch, a sign-off needs a run at its head.
func TestCycleSignoffAtTheHead(t *testing.T) {
	s, _, id := prRepo(t, "qa")
	head := git(t, filepath.Dir(s.gitDir), "rev-parse", "fix/2518-x")
	url := serve(t, s).URL + "/api/issues/" + id[:8] + "/cycle/"
	if code, body := post(t, url+"verify", map[string]string{"output": "ok", "commit": "0123456789"}); code != http.StatusOK {
		t.Fatalf("verify elsewhere: %d %s", code, body)
	}
	if code, body := post(t, url+"signoff", struct{}{}); code != http.StatusBadRequest || !strings.Contains(body, "no run at "+head[:7]) {
		t.Errorf("a run at another commit: %d %s", code, body)
	}
	if code, body := post(t, url+"verify", map[string]string{"output": "ok"}); code != http.StatusOK {
		t.Fatalf("verify at the head: %d %s", code, body)
	}
	if code, body := post(t, url+"signoff", struct{}{}); code != http.StatusOK {
		t.Errorf("signoff: %d %s", code, body)
	}
	th, _ := lastEvent(t, s.b, id)
	if e := th.Events[len(th.Events)-2]; e.Meta["signoff:commit"] != head || e.Body != "Signed off qa at "+head[:7] {
		t.Errorf("the sign-off %+v", e)
	}
}
