// Command git-board-server serves the git-board web UI: a JSON API over
// internal/config + internal/gitbugio + internal/fields + internal/board
// (the same packages the CLI and TUI use), plus the built frontend
// (web/dist, embedded via web/embed.go) as static assets.
//
// No auth in v1: it listens on this machine only by default, and refuses
// what another site could make a browser send (cors.go). See README.md.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghbridge"
	"github.com/guilbep/git-board/internal/gitbugio"
	boardweb "github.com/guilbep/git-board/web"
)

func main() {
	// This machine only by default: the server writes to git, and to the
	// forges with the user's tokens. Anyone who can reach it can.
	addr := flag.String("addr", "localhost:8080", "listen address; localhost is both loopback addresses (an address other machines reach exposes your git-bug data and forge tokens to them)")
	me := flag.String("me", "", "current user, for @me filters/values (env GIT_BOARD_ME)")
	flag.Parse()

	cfg, err := config.Load(config.Path())
	if err != nil {
		log.Fatal(err)
	}

	b, err := gitbugio.OpenCwd()
	if err != nil {
		log.Fatalf("opening git-bug repo: %v", err)
	}
	defer b.Close()

	// Quasar's default build output is nested under dist/spa (the mode
	// name) — web/dist/.gitkeep lives in the parent dist/, which Quasar's
	// build never cleans, so the embed target survives repeated builds.
	dist, err := fs.Sub(boardweb.DistFS, "dist/spa")
	if err != nil {
		log.Fatalf("web/dist embed: %v", err)
	}

	listeners, exposed, err := listen(*addr)
	if err != nil {
		log.Fatalf("%v (another server on this port? pass --addr localhost:8090)", err)
	}
	s := &apiServer{cfg: cfg, b: b, me: resolveMe(*me), project: ghbridge.Gh, exposed: exposed}
	handler := withCORS(newMux(s, http.FileServerFS(dist)))

	_, port, _ := net.SplitHostPort(*addr)
	fmt.Printf("git-board-server listening on http://localhost:%s\n", port)
	errs := make(chan error, len(listeners))
	for _, l := range listeners {
		go func() { errs <- http.Serve(l, handler) }()
	}
	log.Fatal(<-errs)
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

// resolveMe mirrors cmd/git-board's --me/GIT_BOARD_ME resolution.
func resolveMe(flag string) string {
	if flag != "" {
		return flag
	}
	return os.Getenv("GIT_BOARD_ME")
}
