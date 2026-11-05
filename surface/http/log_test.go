package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// A request's line holds its method, path, status, duration and size,
// and never a header, a cookie, a token or a body; a query's secrets are
// redacted at debug.
func TestLogRedacts(t *testing.T) {
	t.Parallel()
	dir, ids := newBoard(t)
	a := newAPI(t, dir) // at debug
	a.do("POST", "/api/issues/"+string(ids[1])+"/comments?token=ghp_s3cr3t&x=1", map[string]any{"body": "body-s3cr3t"},
		"Authorization", "Bearer auth-s3cr3t", "Cookie", "session=cookie-s3cr3t", "X-Git-Board-Client", "page")
	logs := a.logs()
	for _, secret := range []string{"ghp_s3cr3t", "body-s3cr3t", "auth-s3cr3t", "cookie-s3cr3t"} {
		if strings.Contains(logs, secret) {
			t.Errorf("the log holds %s:\n%s", secret, logs)
		}
	}
	for _, want := range []string{"msg=request method=POST path=/api/issues/" + string(ids[1]) + "/comments status=201 duration=", "bytes=", "query=\"token=REDACTED&x=1\""} {
		if !strings.Contains(logs, want) {
			t.Errorf("no %q in:\n%s", want, logs)
		}
	}
}

// The levels: info leaves the debug lines out; an internal error is
// logged at error with its code; json is one object a line.
func TestLogLevels(t *testing.T) {
	t.Parallel()
	if _, err := NewLogger(nil, "loud", ""); err == nil {
		t.Error("a level that isn't one is taken")
	}
	if _, err := NewLogger(nil, "", "xml"); err == nil {
		t.Error("a format that isn't one is taken")
	}
	var buf bytes.Buffer
	log, err := NewLogger(&buf, "info", "json")
	if err != nil {
		t.Fatal(err)
	}
	h := withLog(log, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/boom" {
			writeError(w, r, logOf(r.Context()), errBoom)
			return
		}
		w.Write([]byte("{}"))
	}))
	for _, p := range []string{"/ok?token=x", "/boom"} {
		req, _ := http.NewRequest("GET", p, nil)
		h.ServeHTTP(&discard{header: http.Header{}}, req)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var got []map[string]any
	for _, l := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("not JSON: %s", l)
		}
		got = append(got, m)
	}
	if len(got) != 3 {
		t.Fatalf("%d lines, want 3 (two requests, one error):\n%s", len(got), buf.String())
	}
	if got[0]["query"] != nil || got[0]["level"] != "INFO" || got[0]["status"] != 200.0 {
		t.Errorf("an info request line = %v", got[0])
	}
	if got[1]["level"] != "ERROR" || got[1]["code"] != "internal" || got[1]["error"] != "boom" {
		t.Errorf("an internal error = %v", got[1])
	}
	if got[2]["status"] != 500.0 || got[2]["code"] != "internal" {
		t.Errorf("its request line = %v", got[2])
	}
	buf.Reset()
	quiet, _ := NewLogger(&buf, "error", "text")
	withLog(quiet, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(&discard{header: http.Header{}}, httpGet("/x"))
	if buf.Len() != 0 {
		t.Errorf("at error, a request is logged: %s", buf.String())
	}
}

var errBoom = boom{}

type boom struct{}

func (boom) Error() string { return "boom" }

func httpGet(p string) *http.Request {
	req, _ := http.NewRequest("GET", p, nil)
	return req
}

type discard struct{ header http.Header }

func (d *discard) Header() http.Header         { return d.header }
func (d *discard) Write(b []byte) (int, error) { return len(b), nil }
func (d *discard) WriteHeader(int)             {}
