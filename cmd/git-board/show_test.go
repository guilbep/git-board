package main

import (
	"encoding/json"
	"strings"
	"testing"
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
