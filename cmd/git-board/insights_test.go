package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/insights"
)

func TestInsights(t *testing.T) {
	newTestRepo(t)
	out, err := execute(t, []string{"insights"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"1 open issues today", "open issues by status", "ready", "throughput", "WEEK OF"} {
		if !strings.Contains(out, want) {
			t.Errorf("insights lacks %q:\n%s", want, out)
		}
	}
	out, err = execute(t, []string{"insights", "--json", "--days", "7"})
	if err != nil {
		t.Fatal(err)
	}
	var rep insights.Report
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("%v:\n%s", err, out)
	}
	if rep.Field != "status" || len(rep.Days) != 8 || rep.Open["ready"][7] != 1 {
		t.Errorf("report = %+v", rep)
	}
	if out, err := execute(t, []string{"insights", "--days", "7", "--jq", ".field"}); err != nil || out != "status\n" {
		t.Errorf("insights --jq .field = %v\n%s", err, out)
	}
	if _, err := execute(t, []string{"insights", "--days", "0"}); exitCode(err) != exitUsage {
		t.Errorf("--days 0 = %v, want exit %d", err, exitUsage)
	}
}
