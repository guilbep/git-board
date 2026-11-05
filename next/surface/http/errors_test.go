package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// One error shape, whatever went wrong: a code of the fixed list, a
// sentence, and the request's field when there is one.
func TestErrorShape(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	a := newAPI(t, dir)
	raw := func(method, path, ctype, body string) *http.Response {
		req, _ := http.NewRequest(method, a.srv.URL+path, strings.NewReader(body))
		if ctype != "" {
			req.Header.Set("Content-Type", ctype)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { res.Body.Close() })
		return res
	}
	for _, c := range []struct {
		name   string
		res    *http.Response
		status int
		code   string
		field  string
	}{
		{"no route", a.do("GET", "/api/nope", nil), 404, "not-found", ""},
		{"an M6 route", a.do("GET", "/api/releases", nil), 404, "not-found", ""},
		{"no issue", a.do("GET", "/api/issues/0000000", nil), 404, "not-found", ""},
		{"a filter that can't be read", a.do("GET", "/api/issues?filter=updated:%3Enope", nil), 400, "invalid", "filter"},
		{"a sort that is no field", a.do("GET", "/api/issues?sort=nope", nil), 400, "invalid", "sort"},
		{"a limit too high", a.do("GET", "/api/issues?limit=5000", nil), 400, "invalid", "limit"},
		{"a limit that isn't a number", a.do("GET", "/api/issues?limit=ten", nil), 400, "invalid", "limit"},
		{"a cursor not ours", a.do("GET", "/api/issues?cursor=!!", nil), 400, "invalid", "cursor"},
		{"a body field the route doesn't have", a.do("POST", "/api/issues", map[string]any{"title": "x", "titel": "y"}), 400, "invalid", "titel"},
		{"a field of another type", a.do("POST", "/api/issues", map[string]any{"title": 3}), 400, "invalid", "title"},
		{"JSON broken", raw("POST", "/api/issues", "application/json", "{"), 400, "invalid", ""},
		{"not JSON", raw("POST", "/api/issues", "text/plain", "title=x"), 415, "not-json", ""},
		{"a form", raw("POST", "/api/issues", "application/x-www-form-urlencoded", "title=x"), 415, "not-json", ""},
		{"an assignment malformed", a.do("POST", "/api/issues", map[string]any{"title": "x", "set": []string{"prio"}}), 400, "invalid", "set"},
		{"a remote action that isn't", a.do("POST", "/api/remotes/origin/nope", map[string]any{}), 400, "invalid", "action"},
	} {
		e := want[apiError](t, c.res, c.status)
		if e.Code != c.code || e.Field != c.field || e.Message == "" {
			t.Errorf("%s: %+v, want code %s, field %q", c.name, e, c.code, c.field)
		}
	}
}

// No one to write as: 409 no-identity, as the CLI's exit code 4.
func TestNoIdentity(t *testing.T) {
	t.Parallel()
	dir, ids := newBoard(t)
	gitRun(t, dir, "config", "--unset", "board.me")
	a := newAPI(t, dir)
	e := want[apiError](t, a.do("POST", "/api/issues/"+string(ids[1])+"/comments", map[string]any{"body": "hi"}), 409)
	if e.Code != "no-identity" {
		t.Fatalf("a comment as no one = %+v", e)
	}
}

// ADR 0013: a request must name this machine; a write must come from
// this server's own pages, or the dev server's, or no page at all.
func TestGuard(t *testing.T) {
	t.Parallel()
	dir, ids := newBoard(t)
	a := newAPI(t, dir)
	send := func(method, host, origin string) *http.Response {
		var body *bytes.Reader
		if method != "GET" {
			body = bytes.NewReader([]byte(`{"body":"hi"}`))
		} else {
			body = bytes.NewReader(nil)
		}
		req, _ := http.NewRequest(method, a.srv.URL+"/api/issues/"+string(ids[1])+"/comments", body)
		if method == "GET" {
			req.URL.Path = "/api/issues"
		}
		req.Header.Set("Content-Type", "application/json")
		if host != "" {
			req.Host = host
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { res.Body.Close() })
		return res
	}
	own := a.srv.URL
	for _, c := range []struct {
		name                 string
		method, host, origin string
		status               int
	}{
		{"a read", "GET", "", "", 200},
		{"a read from another site's page", "GET", "", "https://evil.example", 200},
		{"a rebound name, a read", "GET", "evil.example:80", "", 403},
		{"a rebound name, a write", "POST", "evil.example", "http://evil.example", 403},
		{"localhost", "GET", "localhost:1234", "", 200},
		{"a write from this server's page", "POST", "", own, 201},
		{"a write from the dev server", "POST", "", "http://localhost:9000", 201},
		{"a write from a script", "POST", "", "", 201},
		{"a write from another site's page", "POST", "", "https://evil.example", 403},
	} {
		res := send(c.method, c.host, c.origin)
		if res.StatusCode != c.status {
			t.Errorf("%s: %d, want %d", c.name, res.StatusCode, c.status)
			continue
		}
		if c.status == 403 {
			if e := want[apiError](t, res, 403); e.Code != "forbidden" {
				t.Errorf("%s: %+v", c.name, e)
			}
		}
	}
	logs := a.logs()
	if !strings.Contains(logs, "level=WARN msg=refused") || !strings.Contains(logs, "host=evil.example:80") || !strings.Contains(logs, "origin=https://evil.example") {
		t.Errorf("the refusals aren't logged as warnings:\n%s", logs)
	}
	// the dev server's preflight
	req, _ := http.NewRequest("OPTIONS", a.srv.URL+"/api/issues/x", nil)
	req.Header.Set("Origin", "http://localhost:9000")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 204 || res.Header.Get("Access-Control-Allow-Origin") != "http://localhost:9000" || !strings.Contains(res.Header.Get("Access-Control-Allow-Headers"), "If-Match") {
		t.Fatalf("preflight: %d %v", res.StatusCode, res.Header)
	}
}

func TestServedHost(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		host    string
		exposed []string
		ok      bool
	}{
		{"localhost:8080", nil, true},
		{"127.0.0.1:8080", nil, true},
		{"[::1]:8080", nil, true},
		{"evil.example", nil, false},
		{"192.168.1.5:8080", nil, false},
		{"192.168.1.5:8080", []string{"*"}, true},
		{"evil.example:8080", []string{"*"}, false},
		{"board.lan:8080", []string{"board.lan"}, true},
	} {
		if got := servedHost(c.host, c.exposed); got != c.ok {
			t.Errorf("servedHost(%q, %v) = %t", c.host, c.exposed, got)
		}
	}
}

// A request its client left (a page navigating away) cuts the work under
// it: that is no server error, and isn't logged as one.
func TestCanceledIsNoError(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := httptest.NewRequest("GET", "/api/sync", nil).WithContext(ctx)
	writeError(httptest.NewRecorder(), r, log, fmt.Errorf("git remote: signal: killed"))
	if strings.Contains(buf.String(), "level=ERROR") || !strings.Contains(buf.String(), "request canceled") {
		t.Errorf("logged:\n%s", buf.String())
	}
}
