// Package httpapi is git-board's HTTP API and the web app it serves
// (docs/rewrite/api.md): each route decodes a request, calls next/app,
// and encodes the answer. No rule lives here.
package httpapi

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/guilbep/git-board/next/app"
)

// Server serves one board.
type Server struct {
	log        *slog.Logger
	boards     *boards
	events     broker
	seen       seen
	configPath string
	gitDir     string
	version    string
	exposed    []string
	static     fs.FS
	done       chan struct{} // closed when the server stops: the streams end

	runMu sync.Mutex
	run   *Run // the sync running, or the last one
	runs  int
	base  context.Context // the server's: a run outlives its request

	pulls   *app.PullCache // the cycle's PRs, for the server's life
	project projectState   // the GitHub Project bridge's last run
	relMu   sync.Mutex     // the release runner's fetches and writes (releases.go)
	jobMu   sync.Mutex
	job     *Job // the release job running, or the last one

	firstMu sync.Mutex
	first   *app.FirstPull // what the first open of a fresh clone pulled (firstpull.go)
}

// Options say what a Server serves.
type Options struct {
	Dir     string // the repository's working directory
	Log     *slog.Logger
	Version string // the binary's
	Me      string // what @me means: serve's --me, GIT_BOARD_ME; "" for who writes
	Exposed []string
	Static  fs.FS // the web app's files; nil serves none
	// Open opens the board; nil is app.Open with who writes.
	Open func(ctx context.Context, dir string) (*app.Board, error)
	// Pulls is the cycle's PR cache; nil is a new one, on origin's forge.
	Pulls *app.PullCache
}

// New opens the board at o.Dir and makes its server. Close lets it go.
func New(ctx context.Context, o Options) (*Server, error) {
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	s := &Server{log: o.Log, version: o.Version, exposed: o.Exposed, static: o.Static, done: make(chan struct{}),
		seen: seen{at: map[string]string{}, pending: map[string]string{}}, base: context.WithoutCancel(ctx)}
	open := o.Open
	if open == nil {
		open = func(ctx context.Context, dir string) (*app.Board, error) {
			return app.Open(ctx, dir, app.Options{Who: true, Notice: func(format string, args ...any) {
				s.log.Warn(strings.TrimPrefix(sprintf(format, args...), "warning: "))
			}})
		}
	}
	s.pulls = o.Pulls
	if s.pulls == nil {
		s.pulls = app.NewPullCache()
	}
	opener := open
	open = func(ctx context.Context, dir string) (*app.Board, error) {
		b, err := opener(ctx, dir)
		if err == nil {
			b.PRs = s.pulls
			b.Issues.MeName = o.Me
		}
		return b, err
	}
	bs, err := openBoards(ctx, o.Dir, open, s.observe)
	if err != nil {
		return nil, err
	}
	s.boards = bs
	s.configPath = app.ConfigPath(o.Dir)
	s.gitDir = gitDir(ctx, o.Dir)
	return s, nil
}

// Close stops the streams and closes the board once its requests end.
func (s *Server) Close() error {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
	return s.boards.close()
}

// Watch tells the pages what changes outside the API, until ctx ends.
func (s *Server) Watch(ctx context.Context, interval time.Duration) { s.watch(ctx, interval) }

// Handler is the API and the web app, behind ADR 0013's checks, each
// request logged.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.routes(mux)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, s.log, &coded{code: "not-found", status: http.StatusNotFound, err: errNoRoute(r)})
	})
	if s.static != nil {
		mux.Handle("/", http.FileServerFS(s.static))
	}
	return withLog(s.log, withCORS(withGuard(s.log, s.exposed, s.withBoard(mux))))
}

// withBoard holds the board for the request, and makes the request one
// snapshot: its reads and writes check the heads once.
func (s *Server) withBoard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api/events" {
			next.ServeHTTP(w, r)
			return
		}
		h := s.boards.get()
		defer s.boards.release(h)
		ctx := context.WithValue(r.Context(), boardKey{}, h.b)
		ctx = h.b.Kernel.Snapshot(ctx)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type boardKey struct{}

// board is the request's board.
func board(ctx context.Context) *app.Board { return ctx.Value(boardKey{}).(*app.Board) }

// boards is the board open now, opened again when who writes or the
// config changes: a request holds the one it started with, which closes
// once the last request holding it ends.
type boards struct {
	dir     string
	open    func(ctx context.Context, dir string) (*app.Board, error)
	observe func(ctx context.Context, cs []kernelChange)

	mu  sync.Mutex
	cur *held
}

type held struct {
	b       *app.Board
	refs    int
	retired bool
}

func openBoards(ctx context.Context, dir string, open func(context.Context, string) (*app.Board, error), observe func(context.Context, []kernelChange)) (*boards, error) {
	bs := &boards{dir: dir, open: open, observe: observe}
	b, err := bs.opened(ctx)
	if err != nil {
		return nil, err
	}
	bs.cur = &held{b: b}
	return bs, nil
}

func (bs *boards) opened(ctx context.Context) (*app.Board, error) {
	b, err := bs.open(ctx, bs.dir)
	if err != nil {
		return nil, err
	}
	b.Kernel.Observe(bs.observe)
	return b, nil
}

func (bs *boards) get() *held {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	bs.cur.refs++
	return bs.cur
}

func (bs *boards) release(h *held) {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	h.refs--
	if h.retired && h.refs == 0 {
		h.b.Close()
	}
}

// reopen opens the board again, for the requests that start from now.
func (bs *boards) reopen(ctx context.Context) error {
	b, err := bs.opened(ctx)
	if err != nil {
		return err
	}
	bs.mu.Lock()
	defer bs.mu.Unlock()
	old := bs.cur
	bs.cur = &held{b: b}
	old.retired = true
	if old.refs == 0 {
		old.b.Close()
	}
	return nil
}

func (bs *boards) close() error {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	bs.cur.retired = true
	if bs.cur.refs == 0 {
		return bs.cur.b.Close()
	}
	return nil
}
