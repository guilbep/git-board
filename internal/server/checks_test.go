package server

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

type stepTestsAnswer struct {
	Rule       string `json:"rule"`
	Head       string `json:"head"`
	TestCommit string `json:"test_commit"`
	Suites     []struct {
		Name    string   `json:"name"`
		Touched []string `json:"touched"`
		Head    *struct {
			Verdict string `json:"verdict"`
			Proven  bool   `json:"proven"`
			Result  struct {
				Log string `json:"log"`
			} `json:"result"`
		} `json:"head"`
		AtTest *struct {
			Verdict string `json:"verdict"`
		} `json:"at_test"`
	} `json:"suites"`
	Gate struct {
		Proven bool   `json:"proven"`
		Reason string `json:"reason"`
	} `json:"gate"`
	Running []string `json:"running"`
	Error   string   `json:"error"`
}

// checksRepo is prRepo with a bug issue in progress, a base whose toml
// declares two suites and a check and whose .allowed_signers names a new
// key, git's user.signingkey that key, and fix/2518-x adding a test and
// then the fix.
func checksRepo(t *testing.T) (*apiServer, string, string) {
	t.Helper()
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("no ssh-keygen")
	}
	s, _, id := prRepo(t, "in-progress")
	if _, err := s.b.ApplyLabelChange(id, []string{"kind:bug"}, nil); err != nil {
		t.Fatal(err)
	}
	dir := s.root
	keyFile := filepath.Join(t.TempDir(), "key")
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "dev", "-f", keyFile).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v\n%s", err, out)
	}
	pub, _ := os.ReadFile(keyFile + ".pub")
	write := func(p, body string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, p), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".git-board.toml", `[cycle.tests.unit]
run = ["sh", "-c", "test -f fixed"]
files = ["**/*_test.go", "fixed"]

[cycle.tests.web]
run = ["true"]
files = ["web/**"]

[cycle.checks.vet]
run = ["sh", "-c", "test -f vetted"]
files = ["vetted", "fixed"]
`)
	write(".allowed_signers", `dev namespaces="git-board-ci" `+string(pub))
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "checks")
	git(t, dir, "update-ref", "refs/remotes/origin/dev", "dev")
	git(t, dir, "switch", "-q", "fix/2518-x")
	git(t, dir, "rebase", "-q", "dev")
	write("save_test.go", "package save")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "A failing test")
	write("fixed", "1")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "The fix")
	git(t, dir, "switch", "-q", "dev")
	git(t, dir, "update-ref", "refs/remotes/origin/fix/2518-x", "fix/2518-x")
	git(t, dir, "config", "user.signingkey", keyFile)
	return s, id, keyFile
}

// ran runs names at target and waits for the job and its cycle event.
func ran(t *testing.T, s *apiServer, url string, target string, names ...string) {
	t.Helper()
	events, stop := s.events.subscribe()
	defer stop()
	if code, body := post(t, url+"/tests/run", map[string]any{"names": names, "target": target}); code != http.StatusAccepted {
		t.Fatalf("run %v: %d %s", names, code, body)
	}
	for seen := 0; seen < len(names); {
		select {
		case data := <-events:
			var e event
			_ = json.Unmarshal(data, &e)
			if e.Kind == "cycle" {
				seen++
			}
		case <-time.After(time.Minute):
			t.Fatalf("no cycle event for %v", names)
		}
	}
}

func TestStepTests(t *testing.T) {
	s, id, _ := checksRepo(t)
	ts := serve(t, s)
	url := ts.URL + "/api/issues/" + id[:8]
	var a stepTestsAnswer
	getJSON(t, url+"/tests", &a)
	if a.Rule != "failing-test" || a.TestCommit == "" || len(a.Suites) != 2 || a.Gate.Proven || !strings.Contains(a.Gate.Reason, "vet not run") {
		t.Fatalf("before a run: %+v", a)
	}
	if unit, web := a.Suites[0], a.Suites[1]; unit.Name != "unit" || !slices.Equal(unit.Touched, []string{"fixed", "save_test.go"}) || len(web.Touched) != 0 {
		t.Errorf("touched: %+v", a.Suites)
	}

	// the bug rule: FAILED at the test's commit, PASSED at the head
	ran(t, s, url, "test", "unit")
	ran(t, s, url, "head", "unit", "vet")
	a = stepTestsAnswer{}
	getJSON(t, url+"/tests", &a)
	unit := a.Suites[0]
	if unit.AtTest == nil || unit.AtTest.Verdict != "FAILED" || unit.Head == nil || !unit.Head.Proven || len(a.Running) != 0 || a.Error != "" {
		t.Fatalf("after the runs: %+v", a)
	}
	if a.Gate.Proven || !strings.Contains(a.Gate.Reason, "vet FAILED, signed by dev") {
		t.Errorf("vet red: %+v", a.Gate)
	}
	if code, body := post(t, url+"/cycle/ready-for-review", nil); code != http.StatusConflict || !strings.Contains(body, "unproven") || !strings.Contains(body, "vet FAILED") {
		t.Errorf("ready for review with vet red: %d %s", code, body)
	}

	// the log, as this clone kept it
	resp, err := http.Get(url + "/tests/log/" + strings.TrimPrefix(unit.Head.Result.Log, "sha256:"))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Errorf("log: %v %v", resp, err)
	}
	if resp, _ := http.Get(url + "/tests/log/..%2f..%2fconfig"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("a path as the hash: %d", resp.StatusCode)
	}

	// green: vetted committed on the branch, vet run again
	git(t, s.root, "switch", "-q", "fix/2518-x")
	git(t, s.root, "commit", "-q", "--allow-empty", "-m", "noop")
	if err := os.WriteFile(filepath.Join(s.root, "vetted"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, s.root, "add", "vetted")
	git(t, s.root, "commit", "-q", "-m", "vetted")
	git(t, s.root, "switch", "-q", "dev")
	ran(t, s, url, "head", "vet")
	if code, body := post(t, url+"/cycle/ready-for-review", nil); code != http.StatusOK || !strings.Contains(body, `"stage":"in-review"`) {
		t.Errorf("ready for review with vet green: %d %s", code, body)
	}
}

func TestRunTestsRefuses(t *testing.T) {
	s, id, _ := checksRepo(t)
	url := serve(t, s).URL + "/api/issues/" + id[:8]
	// the branch's own toml adds a check: only the base's runs
	git(t, s.root, "switch", "-q", "fix/2518-x")
	if err := os.WriteFile(filepath.Join(s.root, ".git-board.toml"), []byte("[cycle.checks.evil]\nrun = [\"touch\", \"/tmp/evil\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, s.root, "commit", "-q", "-am", "evil")
	git(t, s.root, "switch", "-q", "dev")
	for _, c := range []struct {
		body any
		code int
		want string
	}{
		{map[string]any{"names": []string{"evil"}}, http.StatusNotFound, "no suite or check"},
		{map[string]any{"names": []string{"unit"}, "target": "main"}, http.StatusBadRequest, "head or test"},
		{map[string]any{"names": []string{}}, http.StatusBadRequest, "names"},
	} {
		if code, body := post(t, url+"/tests/run", c.body); code != c.code || !strings.Contains(body, c.want) {
			t.Errorf("%v: %d %s", c.body, code, body)
		}
	}
	s.proofJob = &proofJob{ID: id, Running: []string{"unit"}}
	if code, body := post(t, url+"/tests/run", map[string]any{"names": []string{"unit"}}); code != http.StatusConflict || !strings.Contains(body, "busy") {
		t.Errorf("while one runs: %d %s", code, body)
	}
	s.proofJob = nil
	git(t, s.root, "config", "--unset", "user.signingkey")
	if code, body := post(t, url+"/tests/run", map[string]any{"names": []string{"unit"}}); code != http.StatusBadRequest || !strings.Contains(body, "no-signing-key") {
		t.Errorf("no signing key: %d %s", code, body)
	}
}

func TestChecksSetup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lefthook.yml"), []byte("pre-commit:\n  commands:\n    vet:\n      run: go vet ./...\n    fmt:\n      run: gofmt -l {staged_files}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &apiServer{cfg: &config.Config{}, b: gitbugiotest.New(t), root: dir, cfgPath: filepath.Join(dir, ".git-board.toml")}
	if err := os.WriteFile(s.cfgPath, []byte("# the board\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	url := serve(t, s).URL + "/api/checks/setup"
	var got struct {
		Source  string
		Checks  map[string]config.Suite
		Skipped []struct{ Name, Why string }
		TOML    string
	}
	getJSON(t, url, &got)
	if got.Source != "lefthook.yml" || len(got.Checks) != 1 || len(got.Skipped) != 1 || got.Skipped[0].Name != "fmt" || !strings.Contains(got.TOML, "[cycle.checks.vet]\nrun = [\"go\", \"vet\", \"./...\"]") {
		t.Fatalf("%+v", got)
	}
	if code, body := post(t, url, nil); code != http.StatusOK {
		t.Fatalf("%d %s", code, body)
	}
	raw, _ := os.ReadFile(s.cfgPath)
	if !strings.HasPrefix(string(raw), "# the board\n") || !slices.Equal(s.config().Cycle.Checks["vet"].Run, []string{"go", "vet", "./..."}) {
		t.Errorf("file:\n%s\nconfig %+v", raw, s.config().Cycle.Checks)
	}
	if code, body := post(t, url, nil); code != http.StatusConflict || !strings.Contains(body, "declared") {
		t.Errorf("twice: %d %s", code, body)
	}
}
