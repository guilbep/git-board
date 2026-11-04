package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLabels(t *testing.T) {
	newTestRepo(t)
	id := openIssueID(t)
	if _, err := execute(t, []string{"set", id, "label=+bug"}); err != nil {
		t.Fatal(err)
	}
	out, err := execute(t, []string{"labels", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	var got []labelCount
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v:\n%s", err, out)
	}
	want := []labelCount{{Label: "bug", Value: "bug", Open: 1}, {Label: "status:ready", Field: "status", Value: "ready", Open: 1}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("labels = %+v, want %+v", got, want)
	}
	if out, _ := execute(t, []string{"labels", "--plain"}); strings.Contains(out, "status") || !strings.Contains(out, "bug") {
		t.Errorf("labels --plain:\n%s", out)
	}

	if out, err := execute(t, []string{"labels", "rename", "bug", "defect"}); err != nil || !strings.Contains(out, "on 1 issue") {
		t.Fatalf("rename = %q, %v", out, err)
	}
	if out, _ := execute(t, []string{"get", id}); !strings.Contains(out, "defect") || strings.Contains(out, "bug") {
		t.Errorf("labels after rename:\n%s", out)
	}
	if _, err := execute(t, []string{"labels", "rename", "nope", "x"}); exitCode(err) != exitNotFound {
		t.Errorf("rename of a label no issue has = %v, want exit %d", err, exitNotFound)
	}
	if _, err := execute(t, []string{"labels", "rename", "defect", " x"}); exitCode(err) != exitUsage {
		t.Errorf("rename to a bad label = %v, want exit %d", err, exitUsage)
	}
}
