package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeWtx puts a fake wtx first on PATH, as wtx's own tests fake its
// tools, and returns its directory. Each run appends "cwd<TAB>argv" to
// calls; go and brief copy their --prompt file to prompt; doctor, status
// and init --edit print doctor.json, status.json and edit.json; init
// --from-json F writes wtx.toml (F's bytes behind a "# fake wtx" line)
// and .wt.toml in its cwd. A file fail makes every run print it on stderr
// and exit 1.
func fakeWtx(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
d=$(dirname "$0")
printf '%s\t%s\n' "$(pwd -P)" "$*" >> "$d/calls"
if [ -f "$d/fail" ]; then cat "$d/fail" >&2; exit 1; fi
case "$1" in
doctor) cat "$d/doctor.json" ;;
status) cat "$d/status.json" ;;
init)
  case "$2" in
  --edit) cat "$d/edit.json" ;;
  --from-json) { echo "# fake wtx"; cat "$3"; } > wtx.toml; echo 'post_create = ["wtx hook post-create"]' > .wt.toml ;;
  esac ;;
go|brief)
  while [ $# -gt 0 ]; do
    if [ "$1" = --prompt ]; then cp "$2" "$d/prompt"; fi
    shift
  done ;;
esac
`
	files := map[string]string{
		"wtx": script,
		"doctor.json": `{"ok": true, "checks": [
			{"name": "wt", "ok": true, "detail": "/bin/wt", "fix": "", "hard": true},
			{"name": "tmux", "ok": true, "detail": "/bin/tmux", "fix": "", "hard": true},
			{"name": "wtx.toml in r", "ok": true, "detail": "", "fix": "", "hard": true}]}`,
		"status.json": `[{"branch": "fix/2518-x", "path": "/w/fix/2518-x", "main": false,
			"session": "r/fix/2518-x", "live": true, "state": "idle", "agent": "claude", "llm": "opus"}]`,
		"edit.json": `{"repo": {"name": "r", "base_branch": "dev", "protected_branches": ["main", "dev"]},
			"permissions": {"deny": ["Bash(git commit *)", "Bash(git push *)", "Bash(gh pr create *)"]},
			"_changes": []}`,
	}
	for name, body := range files {
		mode := os.FileMode(0o644)
		if name == "wtx" {
			mode = 0o755
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

// wtxCalls is each run of the fake: its cwd and its argv.
func wtxCalls(t *testing.T, fake string) [][2]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fake, "calls"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var calls [][2]string
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		cwd, argv, _ := strings.Cut(line, "\t")
		calls = append(calls, [2]string{cwd, argv})
	}
	return calls
}

// sendFrom is a JSON request with an Origin ("" for none): the status, the
// error code, the body.
func sendFrom(t *testing.T, method, url, origin string, body any) (int, string, []byte) {
	t.Helper()
	data, _ := json.Marshal(body)
	req, err := http.NewRequest(method, url, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	var coded struct{ Code string }
	_ = json.Unmarshal(buf.Bytes(), &coded)
	return resp.StatusCode, coded.Code, buf.Bytes()
}

type worktreesAnswer struct {
	Local  bool   `json:"local"`
	Wtx    bool   `json:"wtx"`
	Reason string `json:"reason"`
	Code   string `json:"code"`
	Doctor *struct {
		Checks []struct{ Name string } `json:"checks"`
	} `json:"doctor"`
	Config  bool `json:"config"`
	Answers *struct {
		Repo struct {
			BaseBranch string `json:"base_branch"`
		} `json:"repo"`
	} `json:"answers"`
}

var someAnswers = map[string]any{"answers": map[string]any{"repo": map[string]any{"name": "r", "base_branch": "dev"}}}

func TestWorktreesRefusedExposed(t *testing.T) {
	fake := fakeWtx(t)
	s, _, _ := gateRepo(t, "https://github.com/o/r.git", false)
	s.exposed = []string{"*"} // what --addr 0.0.0.0:8080 sets
	ts := serve(t, s)
	var a worktreesAnswer
	getJSON(t, ts.URL+"/api/worktrees", &a)
	if a.Local || a.Code != "exposed" || a.Reason == "" || a.Doctor != nil || a.Answers != nil {
		t.Fatalf("exposed: %+v", a)
	}
	if code, c, _ := sendFrom(t, "POST", ts.URL+"/api/worktrees/preview", ts.URL, someAnswers); code != http.StatusForbidden || c != "exposed" {
		t.Fatalf("preview exposed: %d %q", code, c)
	}
	if code, c, _ := sendFrom(t, "POST", ts.URL+"/api/worktrees/init", ts.URL, someAnswers); code != http.StatusForbidden || c != "exposed" {
		t.Fatalf("init exposed: %d %q", code, c)
	}
	if calls := wtxCalls(t, fake); calls != nil {
		t.Fatalf("wtx ran on an exposed server: %v", calls)
	}
}

func TestWorktreesRefusedOrigin(t *testing.T) {
	fake := fakeWtx(t)
	s, _, _ := gateRepo(t, "https://github.com/o/r.git", false)
	ts := serve(t, s)
	if code, c, _ := sendFrom(t, "POST", ts.URL+"/api/worktrees/preview", "", someAnswers); code != http.StatusForbidden || c != "origin" {
		t.Fatalf("no Origin: %d %q", code, c)
	}
	// withGuard refuses a foreign one before the handler
	if code, _, _ := sendFrom(t, "POST", ts.URL+"/api/worktrees/preview", "http://evil.example", someAnswers); code != http.StatusForbidden {
		t.Fatalf("foreign Origin: %d", code)
	}
	if calls := wtxCalls(t, fake); calls != nil {
		t.Fatalf("wtx ran without a same-host Origin: %v", calls)
	}
	if code, _, body := sendFrom(t, "POST", ts.URL+"/api/worktrees/preview", ts.URL, someAnswers); code != http.StatusOK {
		t.Fatalf("same host: %d %s", code, body)
	}
}

func TestWorktreesRefusedNoWtx(t *testing.T) {
	s, _, _ := gateRepo(t, "https://github.com/o/r.git", false)
	ts := serve(t, s)
	t.Setenv("PATH", t.TempDir()) // after git ran: no wtx, no git either
	var a worktreesAnswer
	getJSON(t, ts.URL+"/api/worktrees", &a)
	if !a.Local || a.Wtx || a.Code != "no-wtx" || a.Reason == "" {
		t.Fatalf("no wtx: %+v", a)
	}
	if code, c, _ := sendFrom(t, "POST", ts.URL+"/api/worktrees/init", ts.URL, someAnswers); code != http.StatusConflict || c != "no-wtx" {
		t.Fatalf("init without wtx: %d %q", code, c)
	}
}

func TestWorktreesSettings(t *testing.T) {
	fake := fakeWtx(t)
	s, _, _ := gateRepo(t, "https://github.com/o/r.git", false)
	var a worktreesAnswer
	getJSON(t, serve(t, s).URL+"/api/worktrees", &a)
	if !a.Local || !a.Wtx || a.Reason != "" || a.Code != "" || !a.Config || a.Doctor == nil || len(a.Doctor.Checks) != 3 || a.Answers == nil || a.Answers.Repo.BaseBranch != "dev" {
		t.Fatalf("settings: %+v", a)
	}
	var argv []string
	for _, c := range wtxCalls(t, fake) {
		argv = append(argv, c[1])
	}
	if strings.Join(argv, "|") != "doctor --json|init --edit --no-notes" {
		t.Fatalf("calls = %q", argv)
	}
}

func TestWorktreesNoConfig(t *testing.T) {
	fake := fakeWtx(t)
	_ = os.WriteFile(filepath.Join(fake, "doctor.json"), []byte(`{"ok": false, "checks": [{"name": "wtx.toml in r", "ok": false, "hard": true}]}`), 0o644)
	s, _, _ := gateRepo(t, "https://github.com/o/r.git", false)
	var a worktreesAnswer
	getJSON(t, serve(t, s).URL+"/api/worktrees", &a)
	if a.Config || a.Code != "no-config" || a.Reason != "No wtx.toml in this repository yet" || a.Answers == nil {
		t.Fatalf("no wtx.toml: %+v", a)
	}
}

type filesAnswer struct {
	Files []struct{ Path, Text string } `json:"files"`
}

func TestWtxPreviewThenWrite(t *testing.T) {
	fake := fakeWtx(t)
	s, _, _ := gateRepo(t, "https://github.com/o/r.git", false)
	root := filepath.Dir(s.gitDir)
	ts := serve(t, s)

	code, _, body := sendFrom(t, "POST", ts.URL+"/api/worktrees/preview", ts.URL, someAnswers)
	var preview filesAnswer
	if code != http.StatusOK || json.Unmarshal(body, &preview) != nil || len(preview.Files) != 2 {
		t.Fatalf("preview: %d %s", code, body)
	}
	if preview.Files[0].Path != "wtx.toml" || !strings.HasPrefix(preview.Files[0].Text, "# fake wtx\n") || preview.Files[1].Path != ".wt.toml" {
		t.Fatalf("preview files: %+v", preview.Files)
	}
	if _, err := os.Stat(filepath.Join(root, "wtx.toml")); !os.IsNotExist(err) {
		t.Fatal("the preview wrote in the repository")
	}

	code, _, body = sendFrom(t, "POST", ts.URL+"/api/worktrees/init", ts.URL, someAnswers)
	var written filesAnswer
	if code != http.StatusOK || json.Unmarshal(body, &written) != nil {
		t.Fatalf("init: %d %s", code, body)
	}
	for i, f := range written.Files {
		disk, err := os.ReadFile(filepath.Join(root, f.Path))
		if err != nil || string(disk) != f.Text || f.Text != preview.Files[i].Text {
			t.Fatalf("%s: written %q, on disk %q (%v), previewed %q", f.Path, f.Text, disk, err, preview.Files[i].Text)
		}
	}
	if st := git(t, root, "status", "--porcelain"); !strings.Contains(st, "?? wtx.toml") || !strings.Contains(st, "?? .wt.toml") {
		t.Fatalf("git status = %q, want both files untracked", st)
	}

	calls := wtxCalls(t, fake)
	realRoot, _ := filepath.EvalSymlinks(root)
	if len(calls) != 2 || calls[0][0] == realRoot || !strings.HasPrefix(calls[0][1], "init --from-json ") || strings.HasSuffix(calls[0][1], "--force") {
		t.Fatalf("preview call = %v", calls)
	}
	if calls[1][0] != realRoot || !strings.HasPrefix(calls[1][1], "init --from-json ") || !strings.HasSuffix(calls[1][1], " --force") {
		t.Fatalf("init call = %v, want in %s with --force", calls[1], realRoot)
	}
}
