package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

func jsonBody(t *testing.T, v any) io.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return bytes.NewReader(b)
}

func mustLoadConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load("../../internal/config/testdata/git-board.toml")
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}

func newTestServer(t *testing.T, b *gitbugio.Backend, cfg *config.Config) *httptest.Server {
	t.Helper()
	s := &apiServer{cfg: cfg, b: b, me: "guilbep"}
	mux := newMux(s, http.NotFoundHandler())
	ts := httptest.NewServer(withCORS(mux))
	t.Cleanup(ts.Close)
	return ts
}

func getJSON(t *testing.T, url string, out any) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode %s: %v", url, err)
		}
	}
	return resp
}

func TestListIssuesFiltersAndBucketsFlat(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	open := gitbugiotest.NewIssue(t, b, "open one")
	closed := gitbugiotest.NewIssue(t, b, "closed one")
	gitbugiotest.CloseIssue(t, b, closed)
	ts := newTestServer(t, b, cfg)

	var buckets []board.Bucket
	resp := getJSON(t, ts.URL+"/api/issues?filter=is:open", &buckets)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(buckets) != 1 || len(buckets[0].Issues) != 1 {
		t.Fatalf("buckets = %+v, want one bucket with one issue", buckets)
	}
	if got := buckets[0].Issues[0].ID; got != open {
		t.Errorf("issue id = %q, want %q", got, open)
	}
}

func TestListIssuesGroupsByField(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	a := gitbugiotest.NewIssue(t, b, "a")
	bb := gitbugiotest.NewIssue(t, b, "b")
	if _, err := b.ApplyLabelChange(a, []string{"status:backlog"}, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := b.ApplyLabelChange(bb, []string{"status:ready"}, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ts := newTestServer(t, b, cfg)

	var buckets []board.Bucket
	getJSON(t, ts.URL+"/api/issues?filter=is:open&group=status", &buckets)
	if len(buckets) != 2 {
		t.Fatalf("buckets = %+v, want 2 groups", buckets)
	}
}

func TestPatchIssueSwapsSingleCardinalityField(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "x")
	if _, err := b.ApplyLabelChange(id, []string{"status:backlog"}, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ts := newTestServer(t, b, cfg)

	req, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/issues/"+id, jsonBody(t, map[string]string{"status": "ready"}))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var issue gitbugio.Issue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !contains(issue.Labels, "status:ready") {
		t.Errorf("labels = %v, want status:ready present", issue.Labels)
	}
	if contains(issue.Labels, "status:backlog") {
		t.Errorf("labels = %v, want status:backlog gone (single-cardinality swap)", issue.Labels)
	}
}

func TestConfigFieldsReturnsConfiguredOptions(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	ts := newTestServer(t, b, cfg)

	var got map[string]config.Field
	resp := getJSON(t, ts.URL+"/api/config/fields", &got)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	status, ok := got["status"]
	if !ok || !status.Single || len(status.Options) == 0 {
		t.Errorf("status field = %+v, want single with options", status)
	}
}

func TestCORSPreflightAnsweredBeforeMux(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	ts := newTestServer(t, b, cfg)

	req, _ := http.NewRequest(http.MethodOptions, ts.URL+"/api/issues/deadbeef", nil)
	req.Header.Set("Origin", "http://localhost:9000")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:9000" {
		t.Errorf("Access-Control-Allow-Origin = %q", got)
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
