package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/kernel"
)

// stream is a page's change events.
func stream(t *testing.T, a *api) <-chan Event {
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	req, _ := http.NewRequestWithContext(ctx, "GET", a.srv.URL+"/api/events", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type %q", ct)
	}
	ch := make(chan Event, 64)
	go func() {
		defer res.Body.Close()
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			if data, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				var e Event
				if json.Unmarshal([]byte(data), &e) == nil {
					ch <- e
				}
			}
		}
		close(ch)
	}()
	return ch
}

// next is the first event kind names, within a few seconds.
func next(t *testing.T, ch <-chan Event, kind string) Event {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				t.Fatalf("the stream ended before a %s event", kind)
			}
			if e.Kind == kind {
				return e
			}
		case <-deadline:
			t.Fatalf("no %s event", kind)
		}
	}
}

// A write through the API is told at once, with the page that made it;
// one made outside, by another process, once the watcher sees it.
func TestEvents(t *testing.T) {
	t.Parallel()
	dir, ids := newBoard(t)
	a := newAPI(t, dir)
	wctx, stop := context.WithCancel(ctx)
	defer stop()
	go a.s.Watch(wctx, 50*time.Millisecond)
	ch := stream(t, a)
	time.Sleep(100 * time.Millisecond) // subscribed, and the watcher's first look taken

	th := want[threadBody](t, a.do("POST", "/api/issues/"+string(ids[1])+"/comments", map[string]any{"body": "hi"}, "X-Git-Board-Client", "page-1"), 201)
	e := next(t, ch, "issue")
	if e.ID != string(ids[1]) || e.By != "page-1" || e.Version != th.Version {
		t.Fatalf("the API's write = %+v, want issue %s by page-1 at %s", e, ids[1], th.Version)
	}

	// another process writes: the watcher tells it, by no page
	b := openKernel(t, dir)
	defer b.Close()
	ada := strings.TrimSpace(gitRun(t, dir, "config", "board.me"))
	moved, err := issues.Kind.Do(ctx, b, ids[0], func(kernel.Entity[issues.State]) ([]kernel.Op, error) {
		return []kernel.Op{op("comment", 1700000900, kernel.ID(ada), `{"body":"from the CLI"}`)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	e = next(t, ch, "issue")
	if e.ID != string(ids[0]) || e.By != "" || e.Version != string(moved.Version) {
		t.Fatalf("an outside write = %+v", e)
	}

	// .git-board.toml written by hand: the board opens again, config told
	want[configRes](t, a.do("PATCH", "/api/config", map[string]any{"set": map[string]any{"views.mine.sort": "updated"}}), 200)
	if e := next(t, ch, "config"); e.Kind != "config" {
		t.Fatal(e)
	}
}

func TestBrokerDropsTheSlow(t *testing.T) {
	t.Parallel()
	var b broker
	ch, cancel := b.subscribe()
	defer cancel()
	for range subBuffer + 1 {
		b.publish(Event{Kind: "issue"})
	}
	n := 0
	for range ch {
		n++
	}
	if n != subBuffer {
		t.Fatalf("%d events before the stream closed, want %d", n, subBuffer)
	}
}

// collect is every event the stream sends within d.
func collect(ch <-chan Event, d time.Duration) []Event {
	var got []Event
	deadline := time.After(d)
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				return got
			}
			got = append(got, e)
		case <-deadline:
			return got
		}
	}
}

// A kind the watcher doesn't read (a release record, a forge's sync
// state) is no "gone" event a tick later (review finding 3): the server
// tells those itself, by their run or tag.
func TestEventsNoGhostGone(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	a := newAPI(t, dir)
	a.s.heads(ctx, false) // the start: what is there, known
	sub, cancel := a.s.events.subscribe()
	defer cancel()
	a.s.observe(ctx, []kernel.Change{{Kind: "release", ID: "abc", Version: "v1"}, {Kind: "sync", ID: "st", Version: "v1"}})
	a.s.heads(ctx, true)
	a.s.heads(ctx, true)
	select {
	case data := <-sub:
		t.Fatalf("an event for a kind the pages hear from the server: %s", data)
	default:
	}
}

// A config write through the API is told once, by its page, and opens
// the board once: the watcher knows the server's own write (review
// finding 4).
func TestEventsConfigOnce(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	a := newAPI(t, dir)
	wctx, stop := context.WithCancel(ctx)
	defer stop()
	go a.s.Watch(wctx, 30*time.Millisecond)
	ch := stream(t, a)
	time.Sleep(100 * time.Millisecond)
	want[configRes](t, a.do("PATCH", "/api/config", map[string]any{"set": map[string]any{"views.mine.sort": "updated"}}, "X-Git-Board-Client", "page-A"), 200)
	var configs []Event
	for _, e := range collect(ch, 300*time.Millisecond) {
		if e.Kind == "config" {
			configs = append(configs, e)
		}
	}
	if len(configs) != 1 || configs[0].By != "page-A" {
		t.Fatalf("config events = %+v, want one by page-A", configs)
	}
	// written by hand: told, by no page
	if err := os.WriteFile(filepath.Join(dir, ".git-board.toml"), []byte(boardToml), 0o644); err != nil {
		t.Fatal(err)
	}
	if e := next(t, ch, "config"); e.By != "" {
		t.Fatalf("a hand edit = %+v", e)
	}
}
