package server

import (
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store/storetest"
)

func TestColorsCoverLabelsAndOptions(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	s.cfg.Fields = map[string]config.Field{"status": {Options: []string{"backlog", "in-review"}, Colors: map[string]string{"in-review": "pink"}}}
	id := storetest.NewIssue(t, s.b, "one")
	if _, err := s.b.ApplyLabelChange(id, []string{"status:validated", "mod:travel", "plain", "est:3"}, nil); err != nil {
		t.Fatal(err)
	}
	var got map[string]map[string]string
	call(t, "GET", ts.URL+"/api/colors", nil, &got)
	want := map[string]string{"backlog": "grey", "in-review": "pink", "validated": "green"}
	for v, c := range want {
		if got["status"][v] != c {
			t.Errorf("status %s = %q, want %q (all: %v)", v, got["status"][v], c, got)
		}
	}
	if got["mod"]["travel"] == "" {
		t.Errorf("mod:travel has no color: %v", got)
	}
	if _, ok := got["est"]["3"]; ok {
		t.Errorf("a number got a color: %v", got["est"])
	}
	if _, ok := got["plain"]; ok {
		t.Errorf("a plain label got a field: %v", got)
	}
}
