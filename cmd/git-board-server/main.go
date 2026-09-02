// Command git-board-server serves the git-board web UI: a JSON API over
// internal/config + internal/gitbugio + internal/fields + internal/board
// (the same packages the CLI and TUI use), plus the built frontend
// (web/dist, embedded via web/embed.go) as static assets.
//
// No auth in v1 — same trusted-internal-network assumption as git-bug's own
// `git bug webui`. See README.md for the upgrade path.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
	boardweb "github.com/guilbep/git-board/web"
)

const configPath = ".git-board.toml"

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	me := flag.String("me", "", "current user, for @me filters/values (env GIT_BOARD_ME)")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("%s not found or invalid: %v", configPath, err)
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

	s := &apiServer{cfg: cfg, b: b, me: resolveMe(*me)}
	mux := newMux(s, http.FileServerFS(dist))

	fmt.Printf("git-board-server listening on %s\n", *addr)
	log.Fatal(http.ListenAndServe(*addr, withCORS(mux)))
}

// resolveMe mirrors cmd/git-board's --me/GIT_BOARD_ME resolution.
func resolveMe(flag string) string {
	if flag != "" {
		return flag
	}
	return os.Getenv("GIT_BOARD_ME")
}
