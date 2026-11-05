// Package server serves the git-board web UI (git board serve): a JSON API over
// internal/config + internal/store + core/fields + internal/board
// (the same packages the CLI and TUI use), plus the built frontend
// (web/dist, embedded via web/embed.go) as static assets.
//
// No auth in v1: it listens on this machine only by default, and refuses
// what another site could make a browser send (cors.go). See README.md.
package server

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghbridge"
	"github.com/guilbep/git-board/internal/store"
	boardweb "github.com/guilbep/git-board/web"
)

// Serve serves the board of the repository in the working directory until ctx
// ends, or Ctrl-C (SIGTERM too). addr "" is "git board": localhost on
// the first port from 8080 free on both loopback addresses. me is the current user for @me
// (GIT_BOARD_ME when empty). ready gets the board's URL once it listens.
func Serve(ctx context.Context, addr, me string, ready func(url string)) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfgPath := config.Path()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	b, err := store.OpenCwd()
	if err != nil {
		return fmt.Errorf("opening the repository: %w", err)
	}
	defer b.Close()
	gitDir, err := store.FindGitDir(".")
	if err != nil {
		return err
	}

	// Quasar's default build output is nested under dist/spa (the mode
	// name) — web/dist/.gitkeep lives in the parent dist/, which Quasar's
	// build never cleans, so the embed target survives repeated builds.
	dist, err := fs.Sub(boardweb.DistFS, "dist/spa")
	if err != nil {
		return fmt.Errorf("web/dist embed: %w", err)
	}

	var listeners []net.Listener
	var exposed []string
	host := "localhost"
	if addr == "" {
		if listeners, err = listenFree(); err != nil {
			return fmt.Errorf("no free port from %d: %w", firstPort, err)
		}
	} else {
		if listeners, exposed, err = listen(addr); err != nil {
			return fmt.Errorf("%w (another server on this port? pass --addr localhost:8090)", err)
		}
		host, _, _ = net.SplitHostPort(addr)
	}
	root, _ := os.Getwd()
	s := &apiServer{cfg: cfg, cfgPath: cfgPath, b: b, me: resolveMe(me), project: ghbridge.Gh, exposed: exposed, gitDir: gitDir, root: root}
	// the pages' event streams end with ctx
	srv := &http.Server{Handler: withCORS(newMux(s, http.FileServerFS(dist))), BaseContext: func(net.Listener) context.Context { return ctx }}
	go s.watchRefs(3*time.Second, ctx.Done()) // changes made outside the API reach the pages
	if b.Capabilities().ForgeOnly {
		s.startPushing(ctx) // each write goes to the forges at once
	}
	// a fresh clone opens on its remote's board; the writes wait from
	// here, so that a page's fetch on open can't go first and leave
	// origin fetched, the board still empty, and nothing pulled
	s.writeMu.Lock()
	go s.firstPull()

	errs := make(chan error, len(listeners))
	for _, l := range listeners {
		go func() { errs <- srv.Serve(l) }()
	}
	inst := Instance{URL: boardURL(host, listeners[0]), PID: os.Getpid()}
	if err = writeInstance(gitDir, inst); err == nil {
		defer os.Remove(instancePath(gitDir))
		ready(inst.URL)
		select {
		case err = <-errs:
		case <-ctx.Done():
		}
	}
	stop() // a second Ctrl-C quits at once
	wait, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// the requests running end first, and a forge sync, before the
	// repository closes
	_ = srv.Shutdown(wait)
	if !s.writeMu.TryLock() {
		log.Print("stopping once the write running ends; Ctrl-C again stops now")
		s.writeMu.Lock()
	}
	return err
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

// listen opens addr. localhost is both loopback addresses, either enough:
// so another server holding 127.0.0.1 (a Docker port, say) still leaves
// [::1], where browsers try localhost first. exposed are the names served
// beyond the loopback ones: "*" for every interface, else addr's host.
func listen(addr string) ([]net.Listener, []string, error) {
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
			return []net.Listener{l}, []string{"*"}, nil
		}
		return []net.Listener{l}, []string{host}, nil
	}
	ls, failed := loopbacks(port)
	for _, err := range failed {
		log.Printf("not listening: %v", err)
	}
	if len(ls) == 0 {
		return nil, nil, failed[0]
	}
	return ls, nil, nil
}

// loopbacks opens port on 127.0.0.1 and [::1], localhost; failed are the
// errors of those it couldn't open.
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

// resolveMe mirrors the CLI's --me/GIT_BOARD_ME resolution.
func resolveMe(flag string) string {
	if flag != "" {
		return flag
	}
	return os.Getenv("GIT_BOARD_ME")
}

// firstPull brings a fresh clone its remote's board, once, as the server
// starts (store.PullIfEmpty, docs/prd/clone-shows-board.md). What came,
// or why nothing did, is kept for /api/me: the page that opens next says
// it; a page already open hears it now. Called with writeMu held: it lets
// go once the pull is done.
func (s *apiServer) firstPull() {
	n, err := s.b.PullIfEmpty(store.BoardRemote)
	s.writeMu.Unlock()
	note := firstPullNote{Remote: store.BoardRemote, Issues: n}
	switch {
	case err != nil:
		log.Print(err)
		note.Error = err.Error()
	case n > 0:
		log.Printf("a fresh clone: pulled %d issue%s from %s", n, map[bool]string{true: "", false: "s"}[n == 1], store.BoardRemote)
	default:
		return // nothing to say
	}
	s.firstMu.Lock()
	s.first = &note
	s.firstMu.Unlock()
	s.events.publish(event{Kind: "settings"}) // the pages reload who you are, and the note with it
	if n > 0 {
		s.events.publish(event{Kind: "issues"})
	}
}

// firstPullNote is what the first open of a fresh clone did: how many
// issues it pulled from which remote, or why it couldn't.
type firstPullNote struct {
	Remote string `json:"remote"`
	Issues int    `json:"issues"`
	Error  string `json:"error,omitempty"`
}
