// Package server serves the git-board web UI (git board serve): a JSON API over
// internal/config + internal/gitbugio + internal/fields + internal/board
// (the same packages the CLI and TUI use), plus the built frontend
// (web/dist, embedded via web/embed.go) as static assets.
//
// No auth in v1: it listens on this machine only by default, and refuses
// what another site could make a browser send (cors.go). See README.md.
package server

import (
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghbridge"
	"github.com/guilbep/git-board/internal/gitbugio"
	boardweb "github.com/guilbep/git-board/web"
)

// Serve serves the git-bug repository in the working directory on addr,
// me the current user for @me (GIT_BOARD_ME when empty).
func Serve(addr, me string) error {
	cfgPath := config.Path()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	b, err := gitbugio.OpenCwd()
	if err != nil {
		return fmt.Errorf("opening git-bug repo: %w", err)
	}
	defer b.Close()

	// Quasar's default build output is nested under dist/spa (the mode
	// name) — web/dist/.gitkeep lives in the parent dist/, which Quasar's
	// build never cleans, so the embed target survives repeated builds.
	dist, err := fs.Sub(boardweb.DistFS, "dist/spa")
	if err != nil {
		return fmt.Errorf("web/dist embed: %w", err)
	}

	listeners, exposed, err := listen(addr)
	if err != nil {
		return fmt.Errorf("%w (another server on this port? pass --addr localhost:8090)", err)
	}
	s := &apiServer{cfg: cfg, cfgPath: cfgPath, b: b, me: resolveMe(me), project: ghbridge.Gh, exposed: exposed}
	handler := withCORS(newMux(s, http.FileServerFS(dist)))
	go s.watchRefs(3*time.Second, nil) // changes made outside the API reach the pages

	_, port, _ := net.SplitHostPort(addr)
	fmt.Printf("git-board listening on http://localhost:%s\n", port)
	errs := make(chan error, len(listeners))
	for _, l := range listeners {
		go func() { errs <- http.Serve(l, handler) }()
	}
	return <-errs
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
	var ls []net.Listener
	var firstErr error
	for _, a := range []string{net.JoinHostPort("127.0.0.1", port), net.JoinHostPort("::1", port)} {
		l, err := net.Listen("tcp", a)
		if err != nil {
			log.Printf("not listening on %s: %v", a, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		ls = append(ls, l)
	}
	if len(ls) == 0 {
		return nil, nil, firstErr
	}
	return ls, nil, nil
}

// resolveMe mirrors the CLI's --me/GIT_BOARD_ME resolution.
func resolveMe(flag string) string {
	if flag != "" {
		return flag
	}
	return os.Getenv("GIT_BOARD_ME")
}
