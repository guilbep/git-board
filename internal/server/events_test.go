package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
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
	id := storetest.NewIssue(t, s.b, "Live")
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

// A forge run says where it is: a step at each progress the run tells,
// done of total, streamed as it goes and kept on the job for a page
// loading mid-run. One issue's own sync names the issue, for its drawer.
func TestForgeSyncStreamsItsSteps(t *testing.T) {
	t.Parallel()
	ts, s := forgeServer(t) // no forge to ask who can be assigned, whose answer would be an event too
	a := storetest.NewIssue(t, s.b, "Alpha")
	b := storetest.NewIssue(t, s.b, "Beta")
	release := make(chan struct{})
	s.runSync = fakeSync([]store.SyncEvent{
		{Forge: "gh", Way: "pull", What: "progress", Done: 1, Total: 2, Line: "#1 Alpha"},
		{Forge: "gh", What: "waiting", Line: "GitHub doesn't answer: trying again in 30 s"}, // the step, waiting
		{Forge: "gh", Way: "pull", What: "updated", Issue: a, Ops: 2},                       // a write: counted, no step
		{Forge: "gh", Way: "push", What: "progress", Done: 2, Total: 2, Line: "#2 Beta"},
		{Forge: "gh", Way: "push", What: "posted", Issue: b},
		{Forge: "gh", Way: "push", What: "comment", Issue: "1000"},
		{Forge: "gl", What: "warning", Line: "oops"},
	}, release)
	s.runIssueSync = func(_ context.Context, _ []config.Forge, _ bool, id string, see func(store.SyncEvent)) error {
		see(store.SyncEvent{Forge: "gh", Way: "pull", What: "progress", Done: 1, Total: 1, Line: "#1 Alpha"})
		see(store.SyncEvent{Forge: "gh", Way: "pull", What: "updated", Issue: id, Ops: 1})
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
		{Bridge: "gh", Way: "pull", Done: 1, Total: 2, Issue: "#1 Alpha"},
		{Bridge: "gh", Way: "pull", Done: 1, Total: 2, Issue: "#1 Alpha", Wait: "GitHub doesn't answer: trying again in 30 s"},
		{Bridge: "gh", Way: "push", Done: 2, Total: 2, Issue: "#2 Beta"},
	}
	for _, w := range want {
		if e := readEvent(t, sc); e.Kind != "progress" || e.Step == nil || *e.Step != w {
			t.Errorf("step %+v, want %+v", e, w)
		}
	}
	if e := readEvent(t, sc); e.Kind != "sync" {
		t.Errorf("the end: %+v", e)
	}
	if st := waitJob(t, ts); st.Job.Step == nil || *st.Job.Step != want[2] {
		t.Errorf("the job keeps its last step: %+v", st.Job.Step)
	}

	if code := call(t, "POST", ts.URL+"/api/issues/"+a+"/sync", map[string]bool{"dry_run": true}, nil); code != http.StatusOK {
		t.Fatalf("one issue's sync: %d", code)
	}
	if e := readEvent(t, sc); e.Kind != "progress" || e.ID != a || e.Step == nil || e.Step.Issue != want[0].Issue {
		t.Errorf("one issue's step names it: %+v", e)
	}
}

// A sync cut, as by a restart, is said with the forges, by name where the
// journal's key is the forge's, until a run goes through.
func TestForgesSayACutSync(t *testing.T) {
	t.Parallel()
	ts, s := forgeServer(t, config.Forge{Name: "gh", Kind: "github", Project: "o/r", Sync: true})
	board := filepath.Join(s.b.Store().CommonDir(), "board")
	if err := os.MkdirAll(board, 0o755); err != nil {
		t.Fatal(err)
	}
	journal := `{"pid":1,"started":5}
{"forge":"github.com/o/r","total":3}
{"forge":"github.com/o/r","id":"100","run":5}
{"forge":"gitlab.example.com/g/p","total":2}
`
	if err := os.WriteFile(filepath.Join(board, "sync.lock"), []byte(journal), 0o644); err != nil {
		t.Fatal(err)
	}
	var st forgeStatus
	if getJSON(t, ts.URL+"/api/forges", &st); st.Cut == nil {
		t.Fatal("no cut said")
	}
	want := []cutForge{{Name: "gh", Key: "github.com/o/r", Read: 1, Total: 3}, {Key: "gitlab.example.com/g/p", Total: 2}}
	if !slices.Equal(st.Cut.Forges, want) || st.Cut.At.IsZero() {
		t.Fatalf("cut = %+v, want %+v", st.Cut, want)
	}
}
