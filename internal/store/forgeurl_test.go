package store

import (
	"testing"

	"github.com/guilbep/git-board/core/model"
	"github.com/guilbep/git-board/core/syncer"
)

// A sync record's URL travels through git: anyone who can push the board's
// refs can write one. The API gives an issue's forges an http(s) URL or
// none, so that no client gets a javascript: link (#89); the forge and its
// number stay.
func TestForgesOfKeepsOnlyWebURLs(t *testing.T) {
	t.Parallel()
	rec := func(url string) syncer.Record { return syncer.Record{Link: syncer.Link{URL: url}} }
	got := forgesOf([]syncer.Record{
		rec("https://github.com/o/r/issues/12"),
		rec("http://gitlab.example.org/g/p/-/issues/3"),
		rec("javascript://github.com/%0aalert(1)//14"),
		rec("JavaScript:alert(1)//15"),
		rec("data:text/html,x/16"),
	})
	want := []model.Forge{
		{Name: "GitHub", URL: "https://github.com/o/r/issues/12", Number: "12"},
		{Name: "GitLab", URL: "http://gitlab.example.org/g/p/-/issues/3", Number: "3"},
		{Name: "GitHub", URL: "", Number: "14"},
		{Name: "", URL: "", Number: "15"},
		{Name: "", URL: "", Number: "16"},
	}
	if len(got) != len(want) {
		t.Fatalf("forgesOf: %d forges, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("forgesOf[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
