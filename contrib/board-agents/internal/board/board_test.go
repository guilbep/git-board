package board

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/board-agents/internal/hermetic"
)

// TestMain keeps the tests off the network and the user's tokens.
func TestMain(m *testing.M) { hermetic.Run(m) }

// showSample is what git-board show <id> --json prints (v0.11), cut down.
const showSample = `{
  "id": "23f4873dc97003ff2515d124ebd55c22cc67e205",
  "title": "Export CSV",
  "status": "open",
  "labels": ["status:ready", "risk:low"],
  "number": "CO2-3075",
  "counted": true,
  "forges": [{"name": "github", "url": "https://github.com/o/r/issues/12", "number": "12"}],
  "shown_title": "Export CSV",
  "comments": 1,
  "author": {"id": "c96b", "name": "T"},
  "events": [
    {"kind": "created", "id": "23f4873dc97003ff2515d124ebd55c22cc67e205:0", "author": {"id": "c96b", "name": "T"},
     "at": "2026-11-05T06:44:53+01:00", "body": "The last row."},
    {"kind": "comment", "id": "9c8b", "author": {"id": "c96b", "name": "T"}, "at": "2026-11-05T07:00:00+01:00",
     "body": "Here.", "meta": {"review:path": "a.go"}, "review": {"path": "a.go", "line": 3, "side": "new", "commit": "4e5f"}}
  ]
}`

// fakeCLI is a git-board on PATH that records its argv in calls and
// answers show with showSample, branch with two lines, and exit code 3
// for the issue nope.
func fakeCLI(t *testing.T) (bin, calls string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "show.json"), []byte(showSample), 0o644); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
d=$(dirname "$0")
printf '%s\n' "$*" >> "$d/calls"
if [ "$2" = nope ]; then echo "error: no such issue: nope" >&2; exit 3; fi
case "$1" in
show) cat "$d/show.json" ;;
branch) printf '3075/1-export-csv\ngit switch -c 3075/1-export-csv main\n' ;;
set|comment) ;;
*) echo "error: unknown" >&2; exit 2 ;;
esac
`
	bin = filepath.Join(dir, "git-board")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, filepath.Join(dir, "calls")
}

func readCalls(t *testing.T, file string) []string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func TestCLI(t *testing.T) {
	t.Parallel()
	bin, calls := fakeCLI(t)
	c := &Client{Bin: bin, Dir: t.TempDir()}
	ctx := context.Background()
	th, err := c.Thread(ctx, "CO2-3075")
	if err != nil {
		t.Fatal(err)
	}
	if th.Name() != "CO2-3075" || th.PlanKey() != "3075" || th.Field("status") != "ready" || th.Description() != "The last row." ||
		len(th.Events) != 2 || th.Events[1].Review == nil || th.Events[1].Review.Line != 3 {
		t.Fatalf("thread = %+v", th)
	}
	if b, err := c.Branch(ctx, th.ID); err != nil || b != "3075/1-export-csv" {
		t.Fatalf("branch = %q, %v", b, err)
	}
	if err := c.SetStatus(ctx, th.ID, "in-progress"); err != nil {
		t.Fatal(err)
	}
	if w, err := c.Comment(ctx, th.ID, "a b; $(x)", "risk", ""); err != nil || w.Note != NoStep {
		t.Fatalf("comment: %+v, %v", w, err)
	}
	if _, err := c.Thread(ctx, "nope"); !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "no such issue: nope") {
		t.Fatalf("exit code 3: %v", err)
	}
	if _, err := c.Cycle(ctx, th.ID); err == nil || !strings.Contains(err.Error(), "no board server") {
		t.Fatalf("a cycle without a server: %v", err)
	}
	want := []string{"show CO2-3075 --json", "branch " + th.ID, "set " + th.ID + " status=in-progress", "comment " + th.ID + " -b a b; $(x)", "show nope --json"}
	if got := readCalls(t, calls); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("calls = %q, want %q", got, want)
	}
}

// Name and PlanKey without a project number: the forge's, else the id's.
func TestNames(t *testing.T) {
	t.Parallel()
	th := Thread{ID: "23f4873dc97003ff", Forges: []Forge{{Name: "github", Number: "12"}}}
	if th.Name() != "#12" || th.PlanKey() != "12" {
		t.Errorf("forge: %q %q", th.Name(), th.PlanKey())
	}
	th.Forges = nil
	if th.Name() != "23f4873" || th.PlanKey() != "23f4873" {
		t.Errorf("bare: %q %q", th.Name(), th.PlanKey())
	}
}

// fakeServer speaks git-board v2's API (docs/rewrite/api.md) for the
// routes the client reads, in next/surface/http's shapes: the error shape
// {"code", "message", "field"}, a version in the body and the ETag, 412
// stale on an If-Match the issue moved from, 415 on a write that isn't
// JSON, 400 on a field the route doesn't take (its decoder refuses
// unknown fields). It serves the repository at gitDir, and records the
// writes.
func fakeServer(t *testing.T, cycleOff bool, gitDir string) (*httptest.Server, *[]string) {
	t.Helper()
	var writes []string
	const version = "5c1e"
	fail := func(w http.ResponseWriter, status int, code, msg, field string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": msg, "field": field})
	}
	ok := func(w http.ResponseWriter, status int, body string) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `"`+version+`"`)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/board", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"git_dir": gitDir, "dir": "", "version": "test",
			"datastore": map[string]string{"name": "git"}, "experimental": []string{"cycle"}})
	})
	mux.HandleFunc("GET /api/issues/{id}/cycle", func(w http.ResponseWriter, r *http.Request) {
		if cycleOff {
			fail(w, http.StatusNotFound, "experimental", "the cycle is off: [experimental] features", "")
			return
		}
		ok(w, http.StatusOK, `{"step": 4, "at": "tests", "of": 7, "name": "Tests", "stage": "in-progress", "done": false, "gate": "g",
			"waits": {"role": "assignee", "login": "ada", "what": "write the failing test", "anyone": false}, "mine": true,
			"evidence": {"stages": {"before": [], "ready": "ready", "in_progress": ["in-progress"], "in_review": [], "validation": [], "closing": "done",
				"plans": "docs/plans", "plans_in_pr": true, "test_files": [], "criteria": [], "kinds": {}, "guessed": false,
				"steps": [{"id": "intent", "kind": "intent", "name": "Intent"}, {"id": "tests", "kind": "tests", "name": "Tests"}]},
				"status": "in-progress", "closed": false, "branch": "fix/1-x",
				"plan": {"path": "docs/plans/1-x.md", "status": "draft"}, "plan_approval": {"by": "ada", "at": "2026-11-05T07:00:00Z"}},
			"id": "`+r.PathValue("id")+`", "version": "`+version+`", "body_version": 1, "environments": []}`)
	})
	mux.HandleFunc("GET /api/issues/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") == "missing" {
			fail(w, http.StatusNotFound, "not-found", "no issue missing", "")
			return
		}
		ok(w, http.StatusOK, `{"id": "`+r.PathValue("id")+`", "title": "t", "status": "open", "labels": [], "shown_title": "t",
			"author": {"id": "c96b", "name": "T"}, "edited_at": "2026-11-05T07:00:00Z", "participants": [], "version": "`+version+`",
			"events": [{"kind": "comment", "id": "c1", "author": {"id": "c96b", "name": "T"}, "at": "2026-11-05T07:00:00Z", "body": "b", "html": "<p>b</p>",
				"meta": {"review:path": "a.go", "review:line": "3"}, "review": {"path": "a.go", "line": 3, "side": "new", "commit": "4e5f"}}]}`)
	})
	mux.HandleFunc("GET /api/cycle", func(w http.ResponseWriter, r *http.Request) {
		if who := r.URL.Query().Get("who"); who != "" && who != "me" && who != "team" {
			fail(w, http.StatusBadRequest, "invalid", "who "+who+": me or team", "who")
			return
		}
		ok(w, http.StatusOK, `{"needs": [{"issue": {"id": "abc", "title": "A", "status": "open", "labels": [], "number": "CO2-1", "shown_title": "A",
				"updated_at": "2026-11-05T07:00:00Z", "created_at": "2026-11-05T07:00:00Z"},
			"step": 4, "of": 7, "name": "Tests", "done": false, "waits": {"what": "write the failing test", "anyone": false}, "mine": true,
			"since": "2026-11-05T07:00:00Z", "involved": true, "branch": "fix/1-x"}],
			"waiting": [{"issue": {"id": "def", "title": "D", "status": "open", "labels": []}, "step": 6, "of": 7, "name": "Review",
			"waits": {"role": "review", "login": "bob", "what": "review", "anyone": false}, "mine": false, "branch": "feat/2-y"},
			{"issue": {"id": "ghi", "title": "G", "status": "open", "labels": []}, "step": 1, "of": 7, "name": "Intent", "waits": {"what": "x", "anyone": true}}]}`)
	})
	mux.HandleFunc("POST /api/issues/{id}/comments", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			fail(w, http.StatusUnsupportedMediaType, "not-json", "a write's body is JSON", "")
			return
		}
		var in struct {
			Body string `json:"body"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&in); err != nil {
			fail(w, http.StatusBadRequest, "invalid", err.Error(), "")
			return
		}
		if m := r.Header.Get("If-Match"); m != "" && m != `"`+version+`"` {
			fail(w, http.StatusPreconditionFailed, "stale", "the issue changed since you read it", "")
			return
		}
		if in.Body == "refuse" {
			fail(w, http.StatusConflict, "busy", "a sync runs; try again", "")
			return
		}
		writes = append(writes, r.PathValue("id")+" "+r.Header.Get("If-Match")+" "+in.Body)
		w.Header().Set("Location", "/api/issues/"+r.PathValue("id")+"/comments/9c8b:1")
		ok(w, http.StatusCreated, `{"id": "`+r.PathValue("id")+`", "events": [], "version": "6d2f"}`)
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		fail(w, http.StatusNotFound, "not-found", "no route "+r.Method+" "+r.URL.Path+" (docs/rewrite/api.md)", "")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &writes
}

func TestHTTP(t *testing.T) {
	t.Parallel()
	srv, writes := fakeServer(t, false, "")
	bin, _ := fakeCLI(t)
	c := &Client{Bin: bin, Dir: t.TempDir(), URL: srv.URL}
	ctx := context.Background()
	cyc, err := c.Cycle(ctx, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if cyc.Position() != 2 || cyc.Of != 7 || cyc.Waits.What != "write the failing test" || cyc.Evidence.Branch != "fix/1-x" || cyc.Version != "5c1e" ||
		cyc.Evidence.Plan.Path != "docs/plans/1-x.md" || cyc.Evidence.PlanApproval.By != "ada" || cyc.Evidence.Stages.InProgress[0] != "in-progress" || !cyc.Evidence.Stages.PlansInPR {
		t.Fatalf("cycle = %+v", cyc)
	}
	th, err := c.ReviewThread(ctx, "abc")
	if err != nil || th.ID != "abc" || th.Version != "5c1e" || th.Events[0].Review.Line != 3 || th.Events[0].Review.Located() {
		t.Fatalf("thread = %+v, %v", th, err)
	}
	agents, err := c.Agents(ctx)
	if err != nil || len(agents) != 2 || agents[0].Issue.Name() != "CO2-1" || agents[0].Branch != "fix/1-x" || agents[0].State != "Waits for you" ||
		agents[1].Branch != "feat/2-y" || agents[1].State != "Waits for @bob" || agents[1].Step != 6 {
		t.Fatalf("agents = %+v, %v", agents, err)
	}
	w, err := c.Comment(ctx, "abc", "Suggested: **low**", "risk", cyc.Version)
	if err != nil || w.Note != NoStep || w.ID != "9c8b:1" {
		t.Fatalf("comment: %+v, %v", w, err)
	}
	if w, err := c.Comment(ctx, "abc", "plain", "", ""); err != nil || w.Note != "" {
		t.Fatalf("a comment in no step: %+v, %v", w, err)
	}
	if len(*writes) != 2 || (*writes)[0] != `abc "5c1e" Suggested: **low**` || (*writes)[1] != "abc  plain" {
		t.Fatalf("writes = %q", *writes)
	}
	if _, err := c.Comment(ctx, "abc", "late", "", "0ld"); !errors.Is(err, ErrStale) {
		t.Fatalf("an If-Match the issue moved from: %v", err)
	}
	var api *APIError
	if _, err := c.Comment(ctx, "abc", "refuse", "", ""); !errors.As(err, &api) || api.Code != "busy" || api.Status != http.StatusConflict ||
		!strings.Contains(err.Error(), "a sync runs") {
		t.Fatalf("a refused write: %v", err)
	}
	if _, err := c.ReviewThread(ctx, "missing"); !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "no issue missing") {
		t.Fatalf("404: %v", err)
	}
	if err := c.get(ctx, "/api/issues/abc/thread", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("v1's route: %v", err)
	}
	off, _ := fakeServer(t, true, "")
	c.URL = off.URL
	if _, err := c.Cycle(ctx, "abc"); !errors.Is(err, ErrOff) || !strings.Contains(err.Error(), "the cycle is off") {
		t.Fatalf("cycle off: %v", err)
	}
}

func TestApprovedPlan(t *testing.T) {
	t.Parallel()
	plan := []Step{{ID: "plan", Kind: "plan"}}
	for name, c := range map[string]struct {
		in   Evidence
		want bool
	}{
		"no plan step":           {Evidence{Stages: Stages{Steps: []Step{{ID: "code", Kind: "code"}}}}, true},
		"default list, nothing":  {Evidence{}, false},
		"accepted file":          {Evidence{Plan: &Plan{Status: "accepted"}}, true},
		"approved on the board":  {Evidence{Stages: Stages{Steps: plan}, PlanApproval: &Approval{By: "ada"}}, true},
		"structural, no PR":      {Evidence{Risk: "structural", PlanApproval: &Approval{By: "ada"}}, false},
		"structural, PR merged":  {Evidence{Risk: "structural", PlanPR: &PR{State: "MERGED"}}, true},
		"structural, PR approve": {Evidence{Risk: "structural", PlanPR: &PR{State: "OPEN", Decision: "APPROVED"}}, true},
	} {
		if got := ApprovedPlan(c.in); got != c.want {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
}

// Discover reads GIT_BOARD_URL, else the server.json of the main
// checkout's git directory, from a linked worktree too, when its server
// answers for that repository.
func TestDiscover(t *testing.T) {
	t.Setenv("GIT_BOARD_URL", "")
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("commit", "-q", "--allow-empty", "-m", "init")
	wt := filepath.Join(t.TempDir(), "wt")
	run("worktree", "add", "-q", "-b", "x", wt)
	ctx := context.Background()
	if u := Discover(ctx, wt); u != "" {
		t.Fatalf("no server: %q", u)
	}
	if err := os.MkdirAll(filepath.Join(root, ".git", "git-board"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(u string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, ".git", "git-board", "server.json"), []byte(`{"url": "`+u+`", "pid": 1}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	other, _ := fakeServer(t, false, t.TempDir())
	write(other.URL)
	if u := Discover(ctx, wt); u != "" {
		t.Fatalf("a server of another repository: %q", u)
	}
	srv, _ := fakeServer(t, false, filepath.Join(root, ".git"))
	write(srv.URL)
	if u := Discover(ctx, wt); u != srv.URL {
		t.Fatalf("from the worktree: %q, want %q", u, srv.URL)
	}
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	write(dead.URL)
	if u := Discover(ctx, root); u != "" {
		t.Fatalf("a server that died: %q", u)
	}
	t.Setenv("GIT_BOARD_URL", "http://127.0.0.1:1/")
	if u := Discover(ctx, root); u != "http://127.0.0.1:1" {
		t.Fatalf("GIT_BOARD_URL: %q", u)
	}
}
