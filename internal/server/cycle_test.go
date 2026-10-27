package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghpr"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
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
	t.Parallel()
	b := storetest.New(t)
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

	// at ready, the draft PR holds the plan: asked, unless plans-in-pr = false
	s, gh, id = prRepo(t, "ready")
	getJSON(t, serve(t, s).URL+"/api/issues/"+id[:8]+"/cycle", &cycleAnswer{})
	if !slices.Contains(gh.sent(), "POST /graphql") {
		t.Errorf("at ready, plans in PRs: GitHub was asked %v", gh.sent())
	}
	s, gh, id = prRepo(t, "ready")
	off := false
	s.cfg.Cycle.PlansInPR = &off
	getJSON(t, serve(t, s).URL+"/api/issues/"+id[:8]+"/cycle", &cycleAnswer{})
	if len(gh.sent()) != 0 {
		t.Errorf("at ready, plans-in-pr = false: GitHub was asked %v", gh.sent())
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
func issueAt(t *testing.T, b *store.Backend, status string, labels ...string) string {
	t.Helper()
	id, err := b.NewIssue("Export CSV", twoCriteria, append([]string{"status:" + status}, labels...))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func lastEvent(t *testing.T, b *store.Backend, id string) (store.Thread, store.Event) {
	t.Helper()
	th, err := b.Thread(id)
	if err != nil {
		t.Fatal(err)
	}
	return th, th.Events[len(th.Events)-1]
}

func TestCycleApprovePlan(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
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

	// the approval is step 3's comment, then the status moves to in-progress
	var c cycleAnswer
	if code := call(t, "POST", url+"approve-plan", struct{}{}, &c); code != http.StatusOK || c.Step != 4 {
		t.Errorf("approve: %d %+v", code, c)
	}
	th, _ := lastEvent(t, b, id)
	if e := th.Events[len(th.Events)-2]; e.Meta["plan:approve"] != "1" || e.Meta["cycle:step"] != "3" || !slices.Contains(th.Labels, "status:in-progress") {
		t.Errorf("approval %+v, labels %v", e, th.Labels)
	}

	structural := issueAt(t, b, "ready", "risk:structural")
	if code, body := post(t, newTestServer(t, b, cfg).URL+"/api/issues/"+structural[:8]+"/cycle/approve-plan", struct{}{}); code != http.StatusBadRequest || !strings.Contains(body, "the plan PR is approved on GitHub") {
		t.Errorf("structural: %d %s", code, body)
	}
}

func TestCycleSignoffRefused(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
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
	t.Parallel()
	b := storetest.New(t)
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
	t.Parallel()
	b := storetest.New(t)
	id := issueAt(t, b, "qa", "risk:low")
	url := newTestServer(t, b, cycleConfig(nil, config.Cycle{})).URL + "/api/issues/" + id[:8] + "/cycle/"
	if code, _ := post(t, url+"verify", map[string]string{"output": ""}); code != http.StatusBadRequest {
		t.Errorf("an empty run: %d", code)
	}
	if code, body := post(t, url+"verify", map[string]string{"output": "ok", "commit": ""}); code != http.StatusOK {
		t.Fatalf("verify: %d %s", code, body)
	}
	if _, e := lastEvent(t, b, id); e.Meta["verify:run"] != "1" || e.Meta["cycle:step"] != "7" || e.Body != "Run\n\n```\nok\n```" {
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
	t.Parallel()
	b := storetest.New(t)
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
	t.Parallel()
	b := storetest.New(t)
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

// With no validation status, Verify happens in in-review once the PR is
// merged, and the sign-off closes the issue.
func TestCycleSignoffClosesFromReview(t *testing.T) {
	s, _, id := prRepo(t, "in-review")
	s.cfg = &config.Config{Fields: map[string]config.Field{
		"status": {Single: true, Options: []string{"backlog", "ready", "in-progress", "in-review", "done"}},
		"risk":   {Single: true, Options: []string{"low", "normal", "structural"}},
	}}
	s.pulls.put("fix/2518-x", &ghpr.PR{Number: 3082, State: "MERGED", MergeCommit: "deadbee0"}, nil)
	url := serve(t, s).URL + "/api/issues/" + id[:8] + "/cycle"
	var c cycleAnswer
	if getJSON(t, url, &c); c.Step != 7 || !slices.Contains(c.Actions, "signoff") {
		t.Fatalf("merged in in-review: %+v", c)
	}
	if code, body := post(t, url+"/signoff", map[string]string{"why": "checked by hand"}); code != http.StatusOK {
		t.Fatalf("signoff: %d %s", code, body)
	}
	if th, _ := lastEvent(t, s.b, id); th.Status != "closed" || !slices.Contains(th.Labels, "status:done") {
		t.Errorf("status %s, labels %v", th.Status, th.Labels)
	}
}

// fakeLLM is an OpenAI-compatible chat API that answers answer, set as
// the env's (AI on). Serial: it sets the environment.
func fakeLLM(t *testing.T, answer string) *[]string {
	t.Helper()
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		asked = append(asked, in.Messages[len(in.Messages)-1].Content)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}}}})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LLM_BASE_URL", srv.URL)
	t.Setenv("LLM_MODEL", "test-model")
	t.Setenv("LLM_API_KEY", "")
	return &asked
}

func TestCycleSuggestRisk(t *testing.T) {
	asked := fakeLLM(t, "Structural\nThe export changes a schema.")
	b := storetest.New(t)
	id, err := b.NewIssue("Export CSV", twoCriteria, []string{"status:ready"})
	if err != nil {
		t.Fatal(err)
	}
	url := newTestServer(t, b, cycleConfig(nil, config.Cycle{})).URL + "/api/issues/" + id[:8]
	var c struct {
		cycleAnswer
		AI bool `json:"ai"`
	}
	getJSON(t, url+"/cycle", &c)
	if c.Step != 2 || !c.AI {
		t.Fatalf("at ready, AI on: %+v", c)
	}
	if code, out := post(t, url+"/cycle/suggest-risk", map[string]any{}); code != http.StatusOK {
		t.Fatalf("suggest-risk: %d %s", code, out)
	}
	if len(*asked) != 1 || !strings.Contains((*asked)[0], "Export CSV") || !strings.Contains((*asked)[0], "Keeps the filters") {
		t.Errorf("the model was sent %q, want the title and the description", *asked)
	}
	th, e := lastEvent(t, b, id)
	if e.Body != "Suggested: **structural**\n\nThe export changes a schema." || e.Meta["ai:risk"] != "structural" || e.Meta["ai:model"] != "test-model" || e.Meta["cycle:step"] != "2" {
		t.Errorf("the comment: %q %v", e.Body, e.Meta)
	}
	if slices.ContainsFunc(th.Labels, func(l string) bool { return strings.HasPrefix(l, "risk:") }) {
		t.Errorf("the AI set the risk: %v", th.Labels)
	}
	if code := call(t, "PATCH", url, map[string]string{"risk": "low"}, nil); code != http.StatusOK {
		t.Fatalf("PATCH risk: %d", code)
	}
	if code, out := post(t, url+"/cycle/suggest-risk", map[string]any{}); code != http.StatusBadRequest || !strings.Contains(out, "not at Risk") {
		t.Errorf("past step 2: %d %s", code, out)
	}
}

func TestCycleSuggestRiskRefused(t *testing.T) {
	fakeLLM(t, "I would say it depends.")
	b := storetest.New(t)
	id, err := b.NewIssue("Export CSV", twoCriteria, []string{"status:ready"})
	if err != nil {
		t.Fatal(err)
	}
	url := newTestServer(t, b, cycleConfig(nil, config.Cycle{})).URL + "/api/issues/" + id[:8] + "/cycle/suggest-risk"
	before, _ := lastEvent(t, b, id)
	if code, out := post(t, url, map[string]any{}); code != http.StatusBadGateway || !strings.Contains(out, "no class") {
		t.Errorf("an answer without a class: %d %s", code, out)
	}
	t.Setenv("LLM_BASE_URL", "")
	if code, out := post(t, url, map[string]any{}); code != http.StatusBadRequest || !strings.Contains(out, "AI is off") {
		t.Errorf("AI off: %d %s", code, out)
	}
	if th, _ := lastEvent(t, b, id); len(th.Events) != len(before.Events) {
		t.Errorf("a refused suggestion wrote %d events", len(th.Events)-len(before.Events))
	}
}

func TestRiskOf(t *testing.T) {
	t.Parallel()
	for answer, want := range map[string][2]string{
		"normal\nOne area.":       {"normal", "One area."},
		"**Low**. A copy change.": {"low", "A copy change."},
		"Structural: a schema.":   {"structural", "a schema."},
		"  low":                   {"low", ""},
		"Lowish, it seems.":       {"", ""},
		"It is normal, I think.":  {"", ""},
	} {
		if c, why := riskOf(answer); c != want[0] || why != want[1] {
			t.Errorf("riskOf(%q) = %q, %q; want %q", answer, c, why, want)
		}
	}
}

func TestSpecVerdict(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	open := cycleConfig(nil, config.Cycle{})
	pm := cycleConfig(map[string]string{"pm": "bob"}, config.Cycle{Spec: "pm"})
	as := func(cfg *config.Config, me, id, verdict, on, body string) (int, string) {
		return post(t, serve(t, &apiServer{cfg: cfg, b: b, me: me}).URL+"/api/issues/"+id[:8]+"/cycle/spec-verdict",
			map[string]string{"verdict": verdict, "on": on, "body": body})
	}
	status := func(id string) []string { th, _ := lastEvent(t, b, id); return th.Labels }

	// with the spec role, step 1 is its holder's
	id := issueAt(t, b, "backlog")
	var c cycleAnswer
	getJSON(t, serve(t, &apiServer{cfg: pm, b: b, me: "bob"}).URL+"/api/issues/"+id[:8]+"/cycle", &c)
	if c.Step != 1 || !c.Mine || c.Waits.Login != "bob" || !slices.Equal(c.Actions, []string{"spec-verdict"}) {
		t.Errorf("bob at step 1: %+v", c)
	}
	if code, body := as(pm, "carol", id, "approve", "body@v1", ""); code != http.StatusForbidden || !strings.Contains(body, "waits for the pm role: bob") {
		t.Errorf("the spec holder only: %d %s", code, body)
	}
	if code, body := as(pm, "bob", id, "request-changes", "body@v1", " "); code != http.StatusBadRequest {
		t.Errorf("request-changes without body: %d %s", code, body)
	}
	if code, body := as(pm, "bob", id, "request-changes", "body@v1", "Name the remote"); code != http.StatusOK || !slices.Contains(status(id), "status:backlog") {
		t.Errorf("request-changes: %d %s %v", code, body, status(id))
	}
	if code, body := as(pm, "bob", id, "approve", "body@v1", ""); code != http.StatusOK {
		t.Fatalf("approve: %d %s", code, body)
	}
	th, _ := lastEvent(t, b, id)
	e := th.Events[len(th.Events)-2]
	if e.Meta["spec:verdict"] != "approve" || e.Meta["spec:on"] != "body@v1" || e.Meta["cycle:step"] != "1" || e.Body != "**Approved v1 of the description**" || !slices.Contains(th.Labels, "status:ready") {
		t.Errorf("approval %+v, labels %v", e, th.Labels)
	}

	// a stale verdict: v2 boxes the items, a tick is none, v3 adds one
	stale := issueAt(t, b, "backlog")
	th, _ = lastEvent(t, b, stale)
	text := th.Events[0].Body
	ticked := strings.Replace(text, "- Exports", "- [x] Exports", 1)
	for _, next := range []string{strings.Replace(text, "- Exports", "- [ ] Exports", 1), ticked, ticked + "- Keeps the order\n"} {
		if err := b.EditComment(stale, th.Events[0].ID, next, text); err != nil {
			t.Fatal(err)
		}
		text = next
	}
	before, _ := lastEvent(t, b, stale)
	if code, body := as(pm, "bob", stale, "approve", "body@v2", ""); code != http.StatusConflict || !strings.Contains(body, "v3 is published") {
		t.Errorf("a stale verdict is refused: %d %s", code, body)
	}
	if th, _ := lastEvent(t, b, stale); len(th.Events) != len(before.Events) || !slices.Contains(th.Labels, "status:backlog") {
		t.Errorf("a refused verdict wrote %d events, labels %v", len(th.Events)-len(before.Events), th.Labels)
	}

	// without the role: anyone but the author, whom step 1 waits for
	alice := storetest.NewPerson(t, b, "Person A", "alice")
	mine := storetest.IssueAs(t, b, alice, 1, "Export CSV", twoCriteria, "status:backlog")
	getJSON(t, serve(t, &apiServer{cfg: open, b: b, me: "alice"}).URL+"/api/issues/"+mine[:8]+"/cycle", &c)
	author := c.Waits.Login
	if author != "alice" || !slices.Equal(c.Actions, []string{"move-ready"}) {
		t.Fatalf("step 1 without spec is today's: %+v", c)
	}
	if code, body := as(open, author, mine, "approve", "body@v1", ""); code != http.StatusForbidden || !strings.Contains(body, "author does not approve it") {
		t.Errorf("the author cannot approve: %d %s", code, body)
	}
	if code, body := as(open, "carol", mine, "approve", "body@v1", "Fine"); code != http.StatusOK || !slices.Contains(status(mine), "status:ready") {
		t.Errorf("carol approves: %d %s %v", code, body, status(mine))
	}

	// no criteria; an approval after ready moves nothing
	none := storetest.IssueAs(t, b, alice, 2, "Export CSV", "Just a line", "status:backlog")
	if code, body := as(open, "carol", none, "approve", "body@v1", ""); code != http.StatusBadRequest || !strings.Contains(body, "add acceptance criteria first") {
		t.Errorf("without criteria: %d %s", code, body)
	}
	later := storetest.IssueAs(t, b, alice, 3, "Export CSV", twoCriteria, "status:in-progress")
	if code, body := as(open, "carol", later, "approve", "body@v1", ""); code != http.StatusOK || !slices.Contains(status(later), "status:in-progress") {
		t.Errorf("approve after ready moves nothing: %d %s %v", code, body, status(later))
	}
}

func TestNoCriteria(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	open := cycleConfig(nil, config.Cycle{})
	pm := cycleConfig(map[string]string{"pm": "bob"}, config.Cycle{Spec: "pm"})
	as := func(cfg *config.Config, me, id string) (int, string) {
		return post(t, serve(t, &apiServer{cfg: cfg, b: b, me: me}).URL+"/api/issues/"+id[:8]+"/cycle/no-criteria", map[string]string{})
	}
	alice := storetest.NewPerson(t, b, "Person A", "alice")

	// an issue from before, at ready: anyone accepts it as it is, and it moves to Risk
	old := storetest.IssueAs(t, b, alice, 1, "Export CSV", "Just a line", "status:ready")
	if code, body := as(open, "carol", old); code != http.StatusOK {
		t.Fatalf("accept: %d %s", code, body)
	}
	th, e := lastEvent(t, b, old)
	if e.Meta["intent:criteria"] != "none" || e.Meta["cycle:step"] != "1" || e.Body != "Accepted without acceptance criteria" {
		t.Errorf("the comment: %+v", e)
	}
	var c cycleAnswer
	getJSON(t, serve(t, &apiServer{cfg: open, b: b, me: "carol"}).URL+"/api/issues/"+th.ID[:8]+"/cycle", &c)
	if c.Step != 2 {
		t.Errorf("accepted: step %d, want 2", c.Step)
	}
	if code, body := as(open, "carol", old); code != http.StatusForbidden {
		t.Errorf("twice: %d %s", code, body)
	}

	// with the spec role: its holder, not anyone
	spec := storetest.IssueAs(t, b, alice, 2, "Export CSV", "Just a line", "status:backlog")
	if code, body := as(pm, "carol", spec); code != http.StatusForbidden || !strings.Contains(body, "add acceptance criteria") {
		t.Errorf("carol with a spec role: %d %s", code, body)
	}
	if code, body := as(pm, "bob", spec); code != http.StatusOK {
		t.Errorf("bob holds the spec role: %d %s", code, body)
	}
	// then the verdict approves it without criteria
	if code, body := post(t, serve(t, &apiServer{cfg: pm, b: b, me: "bob"}).URL+"/api/issues/"+spec[:8]+"/cycle/spec-verdict",
		map[string]string{"verdict": "approve", "on": "body@v1"}); code != http.StatusOK {
		t.Errorf("approve after no-criteria: %d %s", code, body)
	}
}
