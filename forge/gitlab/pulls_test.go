package gitlab

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestPulls reads a branch's merge request in forge.PR's words, the open
// one with its pipeline, and opens, edits and merges.
func TestPulls(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var writes []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodGet {
			mu.Lock()
			writes = append(writes, r.Method+" "+r.URL.Path+" "+string(body))
			mu.Unlock()
		}
		switch {
		case strings.HasSuffix(r.URL.EscapedPath(), "/merge_requests") && r.Method == http.MethodGet:
			if r.URL.Query().Get("source_branch") != "feat/x" {
				_, _ = io.WriteString(w, `[]`)
				return
			}
			_, _ = io.WriteString(w, `[{"iid":3,"state":"merged"},{"iid":4,"state":"opened","title":"T"}]`)
		case strings.HasSuffix(r.URL.EscapedPath(), "/merge_requests/4") && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"iid":4,"web_url":"https://gl/g/p/-/merge_requests/4","state":"opened","title":"T","target_branch":"main","sha":"abc",
			  "author":{"username":"ann"},"detailed_merge_status":"not_approved","head_pipeline":{"status":"running","web_url":"https://gl/p/1"}}`)
		case r.Method == http.MethodPost:
			_, _ = io.WriteString(w, `{"iid":5,"state":"opened","title":"New","target_branch":"main"}`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(srv.Close)
	p, err := newProject(srv.URL+"/", "g/p")
	if err != nil {
		t.Fatal(err)
	}
	p.http, p.pace = srv.Client(), 0
	p.sleep = func(context.Context, time.Duration) error { return nil }
	f := &Forge{p: p, token: "t"}
	ctx := t.Context()
	pr, err := f.FindPR(ctx, "feat/x")
	if err != nil {
		t.Fatal(err)
	}
	if pr == nil || pr.Number != 4 || pr.State != "OPEN" || pr.Author != "ann" || pr.Decision != "REVIEW_REQUIRED" || pr.CanMerge ||
		len(pr.Checks) != 1 || pr.Checks[0].State != "pending" {
		t.Fatalf("found %+v", pr)
	}
	if none, err := f.FindPR(ctx, "other"); err != nil || none != nil {
		t.Fatalf("none: %+v, %v", none, err)
	}
	if opened, err := f.OpenPR(ctx, "feat/x", "main", "New", "Refs: 1"); err != nil || opened.Number != 5 {
		t.Fatalf("opened %+v, %v", opened, err)
	}
	if err := f.EditPR(ctx, 5, "A", "B"); err != nil {
		t.Fatal(err)
	}
	if err := f.MergePR(ctx, 5, "abc"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	got := strings.Join(writes, "\n")
	for _, want := range []string{`{"description":"Refs: 1","source_branch":"feat/x","target_branch":"main","title":"New"}`,
		`/merge_requests/5 {"description":"B","title":"A"}`, `/merge_requests/5/merge {"sha":"abc"}`} {
		if !strings.Contains(got, want) {
			t.Errorf("no %s in\n%s", want, got)
		}
	}
}
