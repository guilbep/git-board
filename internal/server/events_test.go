package server

import (
	"bufio"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

func TestBrokerDropsASlowClient(t *testing.T) {
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
