package main

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/server"
	"github.com/guilbep/git-board/internal/store"
)

// runBoard is a bare "git board": this repository's board in the browser,
// where it already runs, else started here until Ctrl-C.
func runBoard(ctx context.Context, out io.Writer, open bool) error {
	return runBoardAt(ctx, out, open, "")
}

// runBoardAt is runBoard opening page, a path after the board's URL
// ("/#/issues/<id>"; "" the board).
func runBoardAt(ctx context.Context, out io.Writer, open bool, page string) error {
	gitDir, err := store.FindGitDir(".")
	if err != nil {
		return err
	}
	show := func(url string) {
		if !open {
			return
		}
		if err := openURL(url + page); err != nil {
			fmt.Fprintf(out, "opening the browser: %v\n", err)
		}
	}
	if url, pid, ok := runningBoard(gitDir); ok {
		fmt.Fprintf(out, "git-board: this repository's board runs at %s (pid %d)\n", url, pid)
		show(url)
		return nil
	}
	return serveBoard(ctx, "", "", func(url string) {
		fmt.Fprintf(out, "git-board: this repository's board is at %s (Ctrl-C stops it)\n", url)
		show(url)
	})
}

// runningBoard is the board's server running for gitDir, if one does: a
// var for the tests.
var runningBoard = func(gitDir string) (url string, pid int, ok bool) {
	inst, ok := server.Running(gitDir)
	return inst.URL, inst.PID, ok
}

// serveBoard serves the board until ctx ends: a var for the tests.
var serveBoard = server.Serve

// openURL opens url in the browser: a var for the tests.
var openURL = func(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}
	return exec.Command("xdg-open", url).Start()
}

func newServeCmd() *cobra.Command {
	var addr, me string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the web UI and its API, without opening a browser",
		Long: `Serve the web UI and its JSON API for the board of the repository in
the working directory, until stopped. No auth: it listens on this machine
only by default, and refuses what another site could make a browser
send. See README.md, "Web UI".`,
		Example: `  git-board serve
  git-board serve --addr 127.0.0.1:9090 --me guilbep`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return server.Serve(cmd.Context(), addr, me, func(url string) {
				fmt.Fprintf(cmd.OutOrStdout(), "git-board listening on %s\n", url)
			})
		},
	}
	// This machine only by default: the server writes to git, and to the
	// forges with the user's tokens. Anyone who can reach it can.
	cmd.Flags().StringVar(&addr, "addr", "localhost:8080", "listen address; localhost is both loopback addresses (an address other machines reach exposes your git-board data and forge tokens to them)")
	meFlag(cmd, &me)
	return cmd
}
