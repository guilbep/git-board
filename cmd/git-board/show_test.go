package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/store/storetest"
)

func TestShowPrintsTheThread(t *testing.T) {
	newTestRepo(t)
	id := openIssueID(t)
	if out, err := execute(t, []string{"comment", id, "-b", "First comment"}); err != nil {
		t.Fatalf("comment: %v\n%s", err, out)
	}

	out, err := execute(t, []string{"show", id[:7]})
	if err != nil {
		t.Fatalf("show: %v\n%s", err, out)
	}
	for _, want := range []string{"Fix invoice bug", "open", "opened by guilbep", "1 comment", "Status:", "ready", "No description.", "First comment"} {
		if !strings.Contains(out, want) {
			t.Errorf("show lacks %q:\n%s", want, out)
		}
	}
	if out, _ := execute(t, []string{"show", id, "--no-comments"}); strings.Contains(out, "First comment") {
		t.Errorf("show --no-comments shows the comment:\n%s", out)
	}
	if out, _ := execute(t, []string{"show", id, "--md"}); !strings.HasPrefix(out, "# Fix invoice bug\n") || !strings.Contains(out, "| Status | ready |") {
		t.Errorf("show --md:\n%s", out)
	}

	out, err = execute(t, []string{"show", id, "--json"})
	if err != nil {
		t.Fatalf("show --json: %v", err)
	}
	var th struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Author struct {
			Name string `json:"name"`
		} `json:"author"`
		Events []struct {
			Kind string `json:"kind"`
			ID   string `json:"id"`
			Body string `json:"body"`
		} `json:"events"`
	}
	if err := json.Unmarshal([]byte(out), &th); err != nil {
		t.Fatalf("show --json: %v\n%s", err, out)
	}
	if th.ID != id || th.Author.Name != "guilbep" || len(th.Events) < 2 || th.Events[0].Kind != "created" || th.Events[len(th.Events)-1].Body != "First comment" {
		t.Errorf("show --json = %+v", th)
	}
	if _, err := execute(t, []string{"show", "0000000"}); exitCode(err) != exitNotFound {
		t.Errorf("show 0000000 = %v, want exit %d", err, exitNotFound)
	}
}

// show --web opens the issue's page in the board where it runs, else
// where it starts one; --forge its forge's page, exit 3 with none.
func TestShowWebAndForge(t *testing.T) {
	dir, b := seedRepo(t)
	linked := storetest.NewImportedIssue(t, b, "On GitHub", "https://github.com/acme/app/issues/12")
	local, err := b.NewIssue("Here only", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	urls := opened(t)

	wasRunning, wasServe := runningBoard, serveBoard
	t.Cleanup(func() { runningBoard, serveBoard = wasRunning, wasServe })
	runningBoard = func(string) (string, int, bool) { return "http://127.0.0.1:8123", 42, true }
	serveBoard = func(context.Context, string, string, func(string)) error {
		t.Error("served a board where one runs")
		return nil
	}
	if out, err := execute(t, []string{"show", local[:7], "--web"}); err != nil || !strings.Contains(out, "http://127.0.0.1:8123") {
		t.Fatalf("show --web: %v\n%s", err, out)
	}
	if want := "http://127.0.0.1:8123/#/issues/" + local; len(*urls) != 1 || (*urls)[0] != want {
		t.Errorf("opened %v, want %s", *urls, want)
	}

	runningBoard = func(string) (string, int, bool) { return "", 0, false }
	serveBoard = func(_ context.Context, _, _ string, ready func(string)) error {
		ready("http://127.0.0.1:8124")
		return nil
	}
	if _, err := execute(t, []string{"show", local[:7], "-w"}); err != nil {
		t.Fatalf("show -w, no board running: %v", err)
	}
	if want := "http://127.0.0.1:8124/#/issues/" + local; len(*urls) != 2 || (*urls)[1] != want {
		t.Errorf("opened %v, want %s last", *urls, want)
	}

	if out, err := execute(t, []string{"show", linked[:7], "--forge"}); err != nil || strings.TrimSpace(out) != "https://github.com/acme/app/issues/12" {
		t.Errorf("show --forge: %v\n%s", err, out)
	}
	if len(*urls) != 3 || (*urls)[2] != "https://github.com/acme/app/issues/12" {
		t.Errorf("opened %v", *urls)
	}
	if _, err := execute(t, []string{"show", local[:7], "--forge"}); exitCode(err) != exitNotFound {
		t.Errorf("show --forge, on no forge = %v, want exit %d", err, exitNotFound)
	}
	if _, err := execute(t, []string{"show", local[:7], "--web", "--json"}); exitCode(err) != exitUsage {
		t.Errorf("--web --json = %v, want exit %d", err, exitUsage)
	}
}
