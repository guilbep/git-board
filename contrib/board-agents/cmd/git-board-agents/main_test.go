package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/board-agents/internal/hermetic"
)

// TestMain keeps the tests off the network and the user's tokens.
func TestMain(m *testing.M) { hermetic.Run(m) }

// setup is a repository, a fake git-board (show prints one issue, exit 3
// for nope) and a fake wtx on PATH, which copies its prompt to prompt.
func setup(t *testing.T) (repo, bin string) {
	t.Helper()
	repo = t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"commit", "-q", "--allow-empty", "-m", "init"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	bin = t.TempDir()
	files := map[string]string{
		"git-board": `#!/bin/sh
if [ "$2" = nope ]; then echo "error: no such issue: nope" >&2; exit 3; fi
case "$1" in
show) echo '{"id": "23f4873dc97003ff", "title": "Export CSV", "number": "CO2-3", "labels": [], "events": [{"kind": "created", "id": "e", "body": "Rows."}]}' ;;
branch) echo 3/1-export-csv ;;
esac
`,
		"wtx": `#!/bin/sh
d=$(dirname "$0")
case "$1" in
go|brief) while [ $# -gt 0 ]; do if [ "$1" = --prompt ]; then cp "$2" "$d/prompt"; fi; shift; done ;;
status) echo '[]' ;;
esac
`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GIT_BOARD_URL", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // no opencode config: AI off
	return repo, bin
}

func runIt(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(context.Background(), args, strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestShowJSON(t *testing.T) {
	repo, _ := setup(t)
	code, out, errOut := runIt(t, "", "show", "CO2-3", "-C", repo, "--json")
	var a struct {
		Issue, Branch string
	}
	if code != 0 || json.Unmarshal([]byte(out), &a) != nil || a.Issue != "CO2-3" || a.Branch != "3/1-export-csv" {
		t.Fatalf("show: %d %s %s", code, out, errOut)
	}
}

// A brief from stdin reaches the prompt; flags go anywhere.
func TestStartBriefFromStdin(t *testing.T) {
	repo, bin := setup(t)
	if code, out, errOut := runIt(t, "Start with the test.", "-C", repo, "start", "-F", "-", "CO2-3"); code != 0 {
		t.Fatalf("start: %d %s %s", code, out, errOut)
	}
	p, err := os.ReadFile(filepath.Join(bin, "prompt"))
	if err != nil || !strings.Contains(string(p), "## From the person\n\nStart with the test.\n") || !strings.Contains(string(p), "> Rows.") {
		t.Fatalf("prompt: %s %v", p, err)
	}
}

func TestExitCodes(t *testing.T) {
	repo, _ := setup(t)
	for _, c := range []struct {
		args []string
		code int
		err  string
	}{
		{nil, exitUsage, ""},
		{[]string{"help"}, 0, ""},
		{[]string{"fly"}, exitUsage, "unknown command"},
		{[]string{"show"}, exitUsage, "takes one issue"},
		{[]string{"show", "CO2-3", "--nope"}, exitUsage, "flag provided but not defined"},
		{[]string{"start", "CO2-3", "--comment", "c1"}, exitUsage, "brief only"},
		{[]string{"start", "CO2-3", "--brief", "x", "-F", "f"}, exitUsage, "not both"},
		{[]string{"init"}, exitUsage, "--answers"},
		{[]string{"show", "nope"}, exitNotFound, "no such issue"},
		{[]string{"brief", "CO2-3"}, exitRefused, "(no-worktree)"},
		{[]string{"risk", "CO2-3"}, exitError, "AI is off"},
	} {
		code, _, errOut := runIt(t, "", append(c.args, "-C", repo)...)
		if code != c.code || !strings.Contains(errOut, c.err) {
			t.Errorf("%v: %d %q, want %d %q", c.args, code, errOut, c.code, c.err)
		}
	}
	if code, _, errOut := runIt(t, "", "status", "-C", t.TempDir()); code != exitError || !strings.Contains(errOut, "not a git repository") {
		t.Errorf("outside a repository: %d %q", code, errOut)
	}
}

// From a linked worktree, wtx runs in the main checkout.
func TestMainCheckout(t *testing.T) {
	repo, _ := setup(t)
	wt := filepath.Join(t.TempDir(), "wt")
	if out, err := exec.Command("git", "-C", repo, "worktree", "add", "-q", "-b", "x", wt).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	got, err := mainCheckout(context.Background(), wt)
	want, _ := filepath.EvalSymlinks(repo)
	if real, _ := filepath.EvalSymlinks(got); err != nil || real != want {
		t.Fatalf("main checkout of the worktree: %q %v, want %q", got, err, want)
	}
}

// The module requires nothing, git-board least of all: it talks to a
// board through the CLI and the HTTP API alone (ADR 0021).
func TestNoDependency(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "require") || strings.Contains(string(data), "git-board") {
		t.Fatalf("go.mod requires a module:\n%s", data)
	}
}
