package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

func TestBrokerDropsASlowClient(t *testing.T) {
	t.Parallel()
	var b broker
	fast, cancelFast := b.subscribe()
	defer cancelFast()
	slow, cancelSlow := b.subscribe()
	defer cancelSlow()
	for i := 0; i < subBuffer+5; i++ {
		b.publish(event{Kind: "issues"})
		<-fast // the fast one reads every event
	}
	n := 0
	for range slow { // closed once full: it reconnects and reloads
		n++
	}
	if n != subBuffer {
		t.Errorf("the slow client got %d events before being dropped, want %d", n, subBuffer)
	}
	b.publish(event{Kind: "config"})
	if e := <-fast; !strings.Contains(string(e), `"config"`) {
		t.Errorf("the fast client after the drop: %s", e)
	}
}

// readEvent reads the next data line of an event stream.
func readEvent(t *testing.T, sc *bufio.Scanner) event {
	t.Helper()
	got := make(chan event, 1)
	go func() {
		for sc.Scan() {
			if data, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				var e event
				_ = json.Unmarshal([]byte(data), &e)
				got <- e
				return
			}
		}
	}()
	select {
	case e := <-got:
		return e
	case <-time.After(3 * time.Second):
		t.Fatal("no event within 3s")
		return event{}
	}
}

// A write through the API reaches every open page, naming the issue and
// the page that made it (which then ignores its own change).
func TestWritesAreStreamed(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	s.cfgPath = filepath.Join(t.TempDir(), ".git-board.toml") // the view save writes there
	id := gitbugiotest.NewIssue(t, s.b, "Live")
	resp, err := http.Get(ts.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	sc := bufio.NewScanner(resp.Body)

	req, _ := http.NewRequest("POST", ts.URL+"/api/issues/"+id+"/labels", strings.NewReader(`{"add":["status:ready"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Git-Board-Client", "page-1")
	if r, err := http.DefaultClient.Do(req); err != nil || r.StatusCode != http.StatusOK {
		t.Fatalf("label: %v %v", err, r.Status)
	}
	if e := readEvent(t, sc); e.Kind != "issues" || e.ID != id || e.By != "page-1" {
		t.Errorf("event %+v", e)
	}

	// a refused write tells no one; a preview isn't a write
	call(t, "POST", ts.URL+"/api/issues/0000000/labels", map[string]any{"add": []string{"x"}}, nil)
	call(t, "POST", ts.URL+"/api/markdown", map[string]string{"body": "hi"}, nil)
	call(t, "PUT", ts.URL+"/api/views/live", map[string]string{"layout": "board", "filter": "is:open"}, nil)
	if e := readEvent(t, sc); e.Kind != "config" {
		t.Errorf("after a refused write, a preview and a view save: %+v, want the view save", e)
	}
}

// A forge run says where it is: a step each time a way of a bridge comes
// to another issue, streamed as it goes and kept on the job for a page
// loading mid-run. One issue's own sync names the issue, for its drawer.
func TestForgeSyncStreamsItsSteps(t *testing.T) {
	t.Parallel()
	ts, s := forgeServer(t) // no forge to ask who can be assigned, whose answer would be an event too
	a := gitbugiotest.NewIssue(t, s.b, "Alpha")
	b := gitbugiotest.NewIssue(t, s.b, "Beta")
	release := make(chan struct{})
	s.runSync = fakeSync([]gitbugio.SyncEvent{
		{Forge: "gh", Way: "pull", What: "updated", Issue: a, Ops: 2},
		{Forge: "gh", Way: "pull", What: "linked", Issue: a}, // the same issue: no new step
		{Forge: "gh", Way: "push", What: "posted", Issue: b},
		{Forge: "gh", Way: "push", What: "comment", Issue: "1000"}, // a write: no step
		{Forge: "gl", What: "warning", Line: "oops"},               // names no issue: no step
	}, release)
	s.runIssueSync = func(_ context.Context, _ []config.Forge, _ bool, id string, see func(gitbugio.SyncEvent)) error {
		see(gitbugio.SyncEvent{Forge: "gh", Way: "pull", What: "updated", Issue: id, Ops: 1})
		return nil
	}
	resp, err := http.Get(ts.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)

	if code := call(t, "POST", ts.URL+"/api/forges/sync", map[string]any{"dry_run": true}, nil); code != http.StatusAccepted {
		t.Fatalf("start: %d", code)
	}
	if e := readEvent(t, sc); e.Kind != "sync" {
		t.Fatalf("the start: %+v", e)
	}
	close(release)
	want := []syncStep{
		{Bridge: "gh", Way: "pull", Done: 1, Issue: a[:7] + " Alpha"},
		{Bridge: "gh", Way: "push", Done: 1, Issue: b[:7] + " Beta"},
	}
	for _, w := range want {
		if e := readEvent(t, sc); e.Kind != "progress" || e.Step == nil || *e.Step != w {
			t.Errorf("step %+v, want %+v", e, w)
		}
	}
	if e := readEvent(t, sc); e.Kind != "sync" {
		t.Errorf("the end: %+v", e)
	}
	if st := waitJob(t, ts); st.Job.Step == nil || *st.Job.Step != want[1] {
		t.Errorf("the job keeps its last step: %+v", st.Job.Step)
	}

	if code := call(t, "POST", ts.URL+"/api/issues/"+a+"/sync", map[string]bool{"dry_run": true}, nil); code != http.StatusOK {
		t.Fatalf("one issue's sync: %d", code)
	}
	if e := readEvent(t, sc); e.Kind != "progress" || e.ID != a || e.Step == nil || e.Step.Issue != want[0].Issue {
		t.Errorf("one issue's step names it: %+v", e)
	}
}
