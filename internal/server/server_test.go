package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
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

func newTestServer(t *testing.T, b *store.Backend, cfg *config.Config) *httptest.Server {
	t.Helper()
	s := &apiServer{cfg: cfg, b: b, me: "guilbep"}
	if b != nil && cfg != nil {
		b.SetFlatKinds(cfg) // as Open does from the file
		b.SetHeld(cfg)
	}
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
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	open := storetest.NewIssue(t, b, "open one")
	closed := storetest.NewIssue(t, b, "closed one")
	storetest.CloseIssue(t, b, closed)
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

// No filter: the open issues, as the CLI's ls; an empty one: every issue,
// as typed. Facets come with the same filter.
func TestListIssuesEmptyFilterIsEveryIssue(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	storetest.NewIssue(t, b, "open one")
	closed := storetest.NewIssue(t, b, "closed one")
	storetest.CloseIssue(t, b, closed)
	ts := newTestServer(t, b, cfg)

	for url, want := range map[string]int{"/api/issues": 1, "/api/issues?filter=": 2, "/api/issues?filter=is:closed": 1} {
		var buckets []board.Bucket
		getJSON(t, ts.URL+url, &buckets)
		if len(buckets) != 1 || len(buckets[0].Issues) != want {
			t.Errorf("%s: %+v, want %d issues", url, buckets, want)
		}
	}

	var facets []board.Facet
	getJSON(t, ts.URL+"/api/facets?filter=is:open", &facets)
	if len(facets) == 0 || facets[0].Key != "is" || len(facets[0].Selected) != 1 || facets[0].Selected[0] != "open" {
		t.Errorf("facets %+v", facets)
	}
}

func TestListIssuesGroupsByField(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	a := storetest.NewIssue(t, b, "a")
	bb := storetest.NewIssue(t, b, "b")
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
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")
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

	var issue store.Issue
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
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
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
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
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

// Without a .git-board.toml the page still gets objects, not null.
func TestNoConfigServesEmptyObjects(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, storetest.New(t), &config.Config{})
	for _, path := range []string{"/api/views", "/api/config/fields"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if got := strings.TrimSpace(string(body)); got != "{}" {
			t.Errorf("%s = %s, want {}", path, got)
		}
	}
}
