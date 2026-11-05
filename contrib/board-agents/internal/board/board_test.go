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
	if err := c.Comment(ctx, th.ID, "a b; $(x)", "risk"); err != nil {
		t.Fatal(err)
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

// fakeServer answers the v1 routes the client reads, and records the
// writes.
func fakeServer(t *testing.T, cycleOff bool) (*httptest.Server, *[]string) {
	t.Helper()
	var writes []string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/repository", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{}`)) })
	mux.HandleFunc("GET /api/issues/{id}/cycle", func(w http.ResponseWriter, r *http.Request) {
		if cycleOff {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error": "the cycle is off", "code": "experimental"}`))
			return
		}
		_, _ = w.Write([]byte(`{"step": 4, "at": "tests", "of": 7, "name": "Tests", "stage": "in-progress", "gate": "g",
			"waits": {"what": "write the failing test"}, "stages": {"ready": "ready", "in_progress": ["in-progress"], "plans": "docs/plans", "plans_in_pr": true},
			"evidence": {"branch": "fix/1-x", "plan": {"path": "docs/plans/1-x.md", "status": "draft"}, "plan_approval": {"by": "ada", "at": "2026-11-05T07:00:00Z"},
			"stages": {"steps": [{"id": "intent", "kind": "intent"}, {"id": "tests", "kind": "tests"}]}}}`))
	})
	mux.HandleFunc("GET /api/issues/{id}/thread", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id": "` + r.PathValue("id") + `", "events": [{"kind": "comment", "id": "c1", "review": {"path": "a.go", "line": 3, "head_line": 5, "outdated": true, "text": "x := 1"}}]}`))
	})
	mux.HandleFunc("GET /api/issues/{id}/thread/missing", http.NotFound)
	mux.HandleFunc("GET /api/work", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("who") != "team" {
			http.Error(w, "who", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"agents": [{"path": "/w/x", "branch": "fix/1-x", "issue": {"id": "abc", "number": "CO2-1"}, "step": 4, "name": "Tests", "state": "Waits for you"}]}`))
	})
	mux.HandleFunc("POST /api/issues/{id}/comments", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			_, _ = w.Write([]byte(`{"error": "not JSON"}`))
			return
		}
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["body"] == "refuse" {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error": "the board is syncing", "code": "syncing"}`))
			return
		}
		writes = append(writes, r.PathValue("id")+" "+in["step"]+" "+in["body"])
		_, _ = w.Write([]byte(`{}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &writes
}

func TestHTTP(t *testing.T) {
	t.Parallel()
	srv, writes := fakeServer(t, false)
	bin, _ := fakeCLI(t)
	c := &Client{Bin: bin, Dir: t.TempDir(), URL: srv.URL}
	ctx := context.Background()
	cyc, err := c.Cycle(ctx, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if cyc.Position() != 2 || cyc.Of != 7 || cyc.Waits.What != "write the failing test" || cyc.Evidence.Branch != "fix/1-x" ||
		cyc.Evidence.Plan.Path != "docs/plans/1-x.md" || cyc.Evidence.PlanApproval.By != "ada" || cyc.Stages.InProgress[0] != "in-progress" || !cyc.Stages.PlansInPR {
		t.Fatalf("cycle = %+v", cyc)
	}
	th, err := c.ReviewThread(ctx, "abc")
	if err != nil || th.ID != "abc" || th.Events[0].Review.HeadLine != 5 || !th.Events[0].Review.Outdated || th.Events[0].Review.Text != "x := 1" {
		t.Fatalf("thread = %+v, %v", th, err)
	}
	agents, err := c.Agents(ctx)
	if err != nil || len(agents) != 1 || agents[0].Issue.Name() != "CO2-1" || agents[0].State != "Waits for you" {
		t.Fatalf("agents = %+v, %v", agents, err)
	}
	if err := c.Comment(ctx, "abc", "Suggested: **low**", "risk"); err != nil {
		t.Fatal(err)
	}
	if len(*writes) != 1 || (*writes)[0] != "abc risk Suggested: **low**" {
		t.Fatalf("writes = %q", *writes)
	}
	if err := c.Comment(ctx, "abc", "refuse", ""); err == nil || !strings.Contains(err.Error(), "the board is syncing") {
		t.Fatalf("a refused write: %v", err)
	}
	if err := c.get(ctx, "/api/issues/abc/thread/missing", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("404: %v", err)
	}
	off, _ := fakeServer(t, true)
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
// checkout's git directory, from a linked worktree too, when it answers.
func TestDiscover(t *testing.T) {
	srv, _ := fakeServer(t, false)
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
