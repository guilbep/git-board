package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/domain/people"
	"github.com/guilbep/git-board/next/kernel"
	"github.com/guilbep/git-board/next/store/git"
	"github.com/guilbep/git-board/next/view"
)

var ctx = context.Background()

// boardToml is the test board's settings.
const boardToml = `# the test board
[fields.status]
options = ["todo", "doing", "done"]

[views.mine]
filter = "assignee:@me"
`

func op(kind string, time int64, author kernel.ID, data string) kernel.Op {
	return kernel.Op{Kind: kind, Author: string(author), Time: time, Data: json.RawMessage(data)}
}

// newBoard is a repository with a board: Ada, who writes, and three
// issues, the first closed with a comment, the last deleted.
func newBoard(t testing.TB) (dir string, ids []kernel.ID) {
	t.Helper()
	dir = t.TempDir()
	gitRun(t, "", "init", "-q", dir)
	b := openKernel(t, dir)
	defer b.Close()
	ada, err := people.Kind.Create(ctx, b, "", op("create", 1700000000, "", `{"name":"Ada","email":"ada@example.com"}`),
		op("add-account", 1700000000, "", `{"server":"github.com","id":"42","login":"ada"}`))
	if err != nil {
		t.Fatal(err)
	}
	a := ada.ID
	for i, title := range []string{"Login fails", "Export crashes", "Gone"} {
		e, err := issues.Kind.Create(ctx, b, "", op("create", 1700000100+int64(i), a, fmt.Sprintf(`{"title":%q,"body":"Body %d"}`, title, i)),
			op("change-labels", 1700000100+int64(i), a, `{"add":["prio:p`+fmt.Sprint(i)+`","bug"]}`))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, e.ID)
	}
	if _, err := issues.Kind.Do(ctx, b, ids[0], func(kernel.Entity[issues.State]) ([]kernel.Op, error) {
		return []kernel.Op{op("comment", 1700000200, a, `{"body":"Seen on staging"}`), op("set-status", 1700000300, a, `{"status":"closed"}`)}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := issues.Kind.Do(ctx, b, ids[2], func(e kernel.Entity[issues.State]) ([]kernel.Op, error) {
		return []kernel.Op{op("delete", 1700000400, a, `{"target":"`+string(e.State.BodyOp)+`"}`)}, nil
	}); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "config", "board.me", string(a))
	if err := os.WriteFile(filepath.Join(dir, ".git-board.toml"), []byte(boardToml), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, ids
}

// openKernel opens the board at dir as another process would: its own
// view.
func openKernel(t testing.TB, dir string) *kernel.Board {
	t.Helper()
	ds, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	v, err := view.Open(ctx, filepath.Join(t.TempDir(), "view.db"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := kernel.Open(ctx, ds, v, issues.Kind, people.Kind)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func gitRun(t testing.TB, dir string, args ...string) string {
	t.Helper()
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

// api is a test server on a board, and its log.
type api struct {
	t   testing.TB
	srv *httptest.Server
	s   *Server
	log *syncWriter
	dir string
}

// logs is what the server logged so far.
func (a *api) logs() string {
	a.log.mu.Lock()
	defer a.log.mu.Unlock()
	return a.log.w.(*bytes.Buffer).String()
}

func newAPI(t testing.TB, dir string) *api {
	t.Helper()
	sw := &syncWriter{w: new(bytes.Buffer)}
	log, _ := NewLogger(sw, "debug", "text")
	s, err := New(ctx, Options{Dir: dir, Log: log, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(func() {
		s.Close()
		srv.Close()
	})
	return &api{t: t, srv: srv, s: s, log: sw, dir: dir}
}

// do sends a request: body, when not nil, as JSON; header pairs after.
func (a *api) do(method, path string, body any, header ...string) *http.Response {
	a.t.Helper()
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			a.t.Fatal(err)
		}
		r = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, a.srv.URL+path, r)
	if err != nil {
		a.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	a.t.Cleanup(func() { res.Body.Close() })
	return res
}

// want checks res's status, and decodes its body into v.
func want[T any](t testing.TB, res *http.Response, status int) T {
	t.Helper()
	var v T
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode != status {
		t.Fatalf("%s %s: %d, want %d: %s", res.Request.Method, res.Request.URL.Path, res.StatusCode, status, data)
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &v); err != nil {
			t.Fatalf("%s %s: %v: %s", res.Request.Method, res.Request.URL.Path, err, data)
		}
	}
	return v
}

type issuePage struct {
	Items []struct {
		ID, Title, Status, Version string
		Labels                     []string
	}
	Total int
	Next  string
}

type threadBody struct {
	ID, Title, Status, Version string
	Labels                     []string
	Events                     []struct {
		Kind, ID, Body, HTML string
	}
}

func etag(res *http.Response) string { return res.Header.Get("ETag") }
