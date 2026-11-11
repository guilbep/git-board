package obs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// Off, a span is no span: its context is the caller's, and nothing goes
// anywhere.
func TestOff(t *testing.T) {
	ctx := context.Background()
	got, end := Start(ctx, "nothing")
	end(nil)
	if got != ctx || TraceID(got) != "" || On() {
		t.Fatal("tracing is off, yet a span was kept")
	}
}

// On, spans reach the collector as OTLP/JSON: a request's parent from a
// page's traceparent, a child under it with an attribute, an event and
// an error, a recorded interaction; the service named. Serial: it swaps
// the package's exporter.
func TestSpansSent(t *testing.T) {
	var mu sync.Mutex
	var got []payload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/traces" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("%s %s", r.URL.Path, r.Header.Get("Content-Type"))
		}
		data, _ := io.ReadAll(r.Body)
		var p payload
		if err := json.Unmarshal(data, &p); err != nil {
			t.Errorf("not OTLP/JSON: %v", err)
		}
		mu.Lock()
		got = append(got, p)
		mu.Unlock()
	}))
	defer srv.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", srv.URL+"/")
	t.Setenv("OTEL_SERVICE_NAME", "")
	shutdown := Init(Endpoint(), "git-board")
	defer shutdown()

	const page = "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	ctx := FromTraceparent(context.Background(), page)
	ctx, endReq := StartServer(ctx, "GET /api/issues", String("http.method", "GET"))
	child, endGit := Start(ctx, "git cat-file")
	Set(child, Int("objects", 12))
	Event(child, "waiting", String("why", "rate limit"))
	endGit(errors.New("exit 1"))
	Record(ctx, "interaction open-issue", time.Now().Add(-250*time.Millisecond), 250*time.Millisecond)
	endReq(nil)
	Flush()

	mu.Lock()
	defer mu.Unlock()
	var spans []spanJSON
	for _, p := range got {
		if n := p.ResourceSpans[0].Resource.Attributes[0]; n.Key != "service.name" || *n.Value.String != "git-board" {
			t.Errorf("resource %+v", n)
		}
		spans = append(spans, p.ResourceSpans[0].ScopeSpans[0].Spans...)
	}
	if len(spans) != 3 {
		t.Fatalf("%d spans, want 3: %+v", len(spans), spans)
	}
	by := map[string]spanJSON{}
	for _, s := range spans {
		by[s.Name] = s
		if s.TraceID != "0af7651916cd43dd8448eb211c80319c" {
			t.Errorf("%s in trace %s, not the page's", s.Name, s.TraceID)
		}
	}
	req, git := by["GET /api/issues"], by["git cat-file"]
	if req.ParentSpanID != "b7ad6b7169203331" || req.Kind != 2 {
		t.Errorf("request span %+v: its parent is the page's span", req)
	}
	if git.ParentSpanID != req.SpanID || git.Status == nil || git.Status.Code != 2 || len(git.Events) != 1 {
		t.Errorf("git span %+v", git)
	}
	if a := git.Attributes; len(a) != 1 || a[0].Key != "objects" || *a[0].Value.Int != "12" {
		t.Errorf("git attributes %+v", a)
	}
	if rec := by["interaction open-issue"]; rec.ParentSpanID != req.SpanID || rec.Start >= rec.End {
		t.Errorf("recorded span %+v", rec)
	}
	if Traceparent(ctx) == "" || TraceID(ctx) != "0af7651916cd43dd8448eb211c80319c" {
		t.Error("the request's traceparent")
	}
}
