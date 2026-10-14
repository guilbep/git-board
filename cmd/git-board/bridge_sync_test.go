package main

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/ghbridge"
)

// recordedEdits is a Project that records the edits it gets.
type recordedEdits struct{ edits []ghbridge.ProjectEdit }

func (r *recordedEdits) Edit(_ string, e ghbridge.ProjectEdit) error {
	r.edits = append(r.edits, e)
	return nil
}

// withFakeProject serves the fixture items of Project 40, with a Status
// field, and records edits instead of calling gh.
func withFakeProject(t *testing.T, write bool) *recordedEdits {
	t.Helper()
	withFixtureFetch(t)
	meta := ghbridge.ProjectMeta{ID: "P", Fields: map[string]ghbridge.ProjectField{
		"Status": {ID: "F-status", Options: map[string]string{"QA Validation": "o-qa", "In review": "o-review"}}}}
	origMeta, origWriter := fetchProjectMeta, projectWriter
	fetchProjectMeta = func(string, int) (ghbridge.ProjectMeta, error) { return meta, nil }
	rec := &recordedEdits{}
	projectWriter = rec
	t.Cleanup(func() { fetchProjectMeta, projectWriter = origMeta, origWriter })

	cfg, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	extra := "\n[github-project]\nowner = \"EPFL-ENAC\"\nnumber = 40\n"
	if write {
		extra += "write = true\n"
	}
	if err := os.WriteFile(configPath, append(cfg, extra...), 0o644); err != nil {
		t.Fatal(err)
	}
	return rec
}

func mustExecute(t *testing.T, args ...string) string {
	t.Helper()
	out, err := execute(t, args)
	if err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
	return out
}

// onlyIssue returns the id and labels of the test repo's one bug.
func onlyIssue(t *testing.T) (string, []string) {
	t.Helper()
	var issues []struct {
		ID     string   `json:"id"`
		Labels []string `json:"labels"`
	}
	if err := json.Unmarshal([]byte(mustExecute(t, "ls", "", "--json")), &issues); err != nil || len(issues) != 1 {
		t.Fatalf("ls: %v %v", issues, err)
	}
	return issues[0].ID, issues[0].Labels
}

func TestBridgeSyncTwoWay(t *testing.T) {
	newBridgeTestRepo(t)
	rec := withFakeProject(t, true)

	out := mustExecute(t, "bridge", "sync")
	if !strings.Contains(out, "dry run") {
		t.Errorf("no dry-run notice:\n%s", out)
	}
	if _, labels := onlyIssue(t); len(labels) != 0 {
		t.Errorf("dry run wrote labels %v", labels)
	}

	mustExecute(t, "bridge", "sync", "--apply")
	id, labels := onlyIssue(t)
	if !slices.Contains(labels, "status:qa-validation") || len(rec.edits) != 0 {
		t.Fatalf("first sync: labels %v edits %v", labels, rec.edits)
	}

	// a change in git-board reaches the Project
	mustExecute(t, "set", id, "status=in-review")
	mustExecute(t, "bridge", "sync", "--apply")
	if len(rec.edits) != 1 || rec.edits[0].Arg != "o-review" {
		t.Fatalf("edits %+v, want status set to In review", rec.edits)
	}

	// the fake Project still says QA Validation, and git-board moves on to
	// ready: both changed since they agreed on in-review, so it's held
	mustExecute(t, "set", id, "status=ready")
	out = mustExecute(t, "bridge", "sync", "--apply")
	if !strings.Contains(out, "conflict on status") || len(rec.edits) != 1 {
		t.Fatalf("not held: edits %+v\n%s", rec.edits, out)
	}
	mustExecute(t, "bridge", "resolve", "2605", "status", "--theirs")
	if _, labels := onlyIssue(t); !slices.Contains(labels, "status:qa-validation") {
		t.Errorf("theirs not kept: %v", labels)
	}
	if out := mustExecute(t, "bridge", "sync"); strings.Contains(out, "conflict on status") {
		t.Errorf("still a conflict after resolving:\n%s", out)
	}
}

func TestBridgeSyncNeverWritesProjectUnlessEnabled(t *testing.T) {
	newBridgeTestRepo(t)
	rec := withFakeProject(t, false)
	mustExecute(t, "bridge", "sync", "--apply")
	id, _ := onlyIssue(t)
	mustExecute(t, "set", id, "status=in-review")

	out := mustExecute(t, "bridge", "sync", "--apply")
	if len(rec.edits) != 0 {
		t.Errorf("wrote to the Project: %+v", rec.edits)
	}
	if !strings.Contains(out, "write = true") {
		t.Errorf("no hint about write:\n%s", out)
	}
	if _, err := execute(t, []string{"bridge", "resolve", "2605", "status", "--ours"}); err == nil {
		t.Error("resolve --ours wrote without write = true")
	}
}
