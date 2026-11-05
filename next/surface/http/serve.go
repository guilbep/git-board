package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	boardweb "github.com/guilbep/git-board/next/web"
)

// Serving a board (ADR 0012, ADR 0013): this machine's loopback, port
// 8080 or the next free one, the web app embedded, and a file in the
// repository saying where it runs, so that a `git board` finds it.

// FirstPort is where a board looks for a free port.
var FirstPort = 8080

// WebApp is the web app's files, embedded (next/web/dist/spa: Quasar's build;
// empty until `npm run build`).
func WebApp() fs.FS {
	dist, err := fs.Sub(boardweb.DistFS, "dist/spa")
	if err != nil {
		return nil
	}
	return dist
}

// ServeOptions say how to serve.
type ServeOptions struct {
	Dir     string
	Addr    string // "": localhost, FirstPort or the next free
	Log     *slog.Logger
	Version string
	// Ready is told the board's URL once it listens.
	Ready func(url string)
	// Watch is how often the changes made outside the API are looked
	// for; 0 is a second.
	Watch time.Duration
}

// Serve serves the board of the repository at o.Dir until ctx ends.
func Serve(ctx context.Context, o ServeOptions) error {
	log := o.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	var ls []net.Listener
	var exposed []string
	var err error
	host := "localhost"
	if o.Addr == "" {
		if ls, err = listenFree(log); err != nil {
			return fmt.Errorf("no free port from %d: %w", FirstPort, err)
		}
	} else {
		if ls, exposed, err = listen(log, o.Addr); err != nil {
			return fmt.Errorf("%w (another server on this port? pass --addr localhost:8090)", err)
		}
		host, _, _ = net.SplitHostPort(o.Addr)
	}
	defer func() {
		for _, l := range ls {
			l.Close()
		}
	}()
	s, err := New(ctx, Options{Dir: o.Dir, Log: log, Version: o.Version, Exposed: exposed, Static: WebApp()})
	if err != nil {
		return err
	}
	defer s.Close()
	srv := &http.Server{Handler: s.Handler(), BaseContext: func(net.Listener) context.Context { return ctx },
		ReadHeaderTimeout: 10 * time.Second, ErrorLog: slog.NewLogLogger(log.Handler(), slog.LevelWarn)}
	watch, stop := context.WithCancel(ctx)
	defer stop()
	go s.Watch(watch, cmpDuration(o.Watch, time.Second))
	errs := make(chan error, len(ls))
	for _, l := range ls {
		go func() { errs <- srv.Serve(l) }()
	}
	inst := Instance{URL: boardURL(host, ls[0]), PID: os.Getpid()}
	b := s.boards.get()
	log.Info("serving", "version", o.Version, "board", b.b.Dir, "datastore", b.b.DatastoreName(), "where", appWhere(b.b), "url", inst.URL)
	s.boards.release(b)
	if s.gitDir != "" {
		if err := writeInstance(s.gitDir, inst); err != nil {
			return err
		}
		defer os.Remove(instancePath(s.gitDir))
	}
	if o.Ready != nil {
		o.Ready(inst.URL)
	}
	select {
	case err = <-errs:
	case <-ctx.Done():
	}
	s.Close() // the event streams end, so that Shutdown doesn't wait on them
	wait, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(wait) // the requests running end first, then the board closes
	log.Info("stopped")
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return err
}

func cmpDuration(a, b time.Duration) time.Duration {
	if a > 0 {
		return a
	}
	return b
}

// boardURL is where this machine reaches the board: an address on every
// interface as localhost.
func boardURL(host string, l net.Listener) string {
	switch host {
	case "", "0.0.0.0", "::":
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(l.Addr().(*net.TCPAddr).Port))
}

// listen opens addr. localhost is both loopback addresses, either
// enough. exposed are the names served beyond the loopback ones: "*" for
// every interface, else addr's host.
func listen(log *slog.Logger, addr string) ([]net.Listener, []string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, nil, err
	}
	if host != "localhost" {
		l, err := net.Listen("tcp", addr)
		if err != nil {
			return nil, nil, err
		}
		switch host {
		case "127.0.0.1", "::1":
			return []net.Listener{l}, nil, nil
		case "", "0.0.0.0", "::":
			log.Warn("listening on every interface: anyone who reaches this port reads and writes the board, and the forges as you", "addr", addr)
			return []net.Listener{l}, []string{"*"}, nil
		}
		return []net.Listener{l}, []string{host}, nil
	}
	ls, failed := loopbacks(port)
	for _, err := range failed {
		log.Warn("not listening", "error", err)
	}
	if len(ls) == 0 {
		return nil, nil, failed[0]
	}
	return ls, nil, nil
}

// loopbacks opens port on 127.0.0.1 and [::1].
func loopbacks(port string) (ls []net.Listener, failed []error) {
	for _, a := range []string{net.JoinHostPort("127.0.0.1", port), net.JoinHostPort("::1", port)} {
		l, err := net.Listen("tcp", a)
		if err != nil {
			failed = append(failed, err)
			continue
		}
		ls = append(ls, l)
	}
	return ls, failed
}

// listenFree takes localhost on FirstPort, else the next port free on
// both loopback addresses (one, on a machine without IPv6): a port
// another server holds on one would send http://localhost:N there.
func listenFree(log *slog.Logger) ([]net.Listener, error) {
	want := 2
	if l, err := net.Listen("tcp", "[::1]:0"); err != nil {
		want = 1
	} else {
		_ = l.Close()
	}
	var err error
	for p := FirstPort; p < FirstPort+100; p++ {
		ls, failed := loopbacks(strconv.Itoa(p))
		if len(ls) >= want {
			return ls, nil
		}
		for _, l := range ls {
			_ = l.Close()
		}
		err = failed[0]
		log.Debug("port taken", "port", p)
	}
	return nil, err
}

// Instance is a running server, as its file in the repository says
// (<git dir>/git-board/server.json, v1's): written once it listens,
// removed when it stops.
type Instance struct {
	URL string `json:"url"`
	PID int    `json:"pid"`
}

func instancePath(gitDir string) string {
	return filepath.Join(gitDir, "git-board", "server.json")
}

func writeInstance(gitDir string, inst Instance) error {
	data, err := json.Marshal(inst)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(instancePath(gitDir)), 0o755); err != nil {
		return err
	}
	return os.WriteFile(instancePath(gitDir), data, 0o644)
}

// Running is the board's server running for the repository at gitDir,
// true when one does: its file names a live process, whose server
// answers that repository. v1's server answers it too (/api/repository):
// one board per repository, whichever binary serves it.
func Running(gitDir string) (Instance, bool) {
	var inst Instance
	data, err := os.ReadFile(instancePath(gitDir))
	if err != nil || json.Unmarshal(data, &inst) != nil || !isRunning(inst.PID) {
		return inst, false
	}
	c := http.Client{Timeout: time.Second}
	for _, probe := range []struct{ path, field string }{{"/api/board", "git_dir"}, {"/api/repository", "gitDir"}} {
		resp, err := c.Get(inst.URL + probe.path)
		if err != nil {
			return inst, false
		}
		var body map[string]any
		ok := resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(&body) == nil
		resp.Body.Close()
		if dir, _ := body[probe.field].(string); ok && dir != "" {
			return inst, sameDir(dir, gitDir)
		}
	}
	return inst, false
}

// isRunning: signal 0 tests the process without touching it, and EPERM
// is one running as another user.
func isRunning(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

func sameDir(a, b string) bool {
	a, errA := filepath.EvalSymlinks(a)
	b, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && a == b
}

// GitDir is the repository's git directory at dir, absolute; "" outside
// one. A board's server answers it, and Running compares it.
func GitDir(ctx context.Context, dir string) string { return gitDir(ctx, dir) }
