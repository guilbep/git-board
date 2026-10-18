package main

import (
	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/server"
)

func newServeCmd() *cobra.Command {
	var addr, me string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the web UI and its API, without opening a browser",
		Long: `Serve the web UI and its JSON API for the git-bug repository in the
working directory, until stopped. No auth: it listens on this machine
only by default, and refuses what another site could make a browser
send. See README.md, "Web UI".`,
		Example: `  git-board serve
  git-board serve --addr 127.0.0.1:9090 --me guilbep`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return server.Serve(addr, me)
		},
	}
	// This machine only by default: the server writes to git, and to the
	// forges with the user's tokens. Anyone who can reach it can.
	cmd.Flags().StringVar(&addr, "addr", "localhost:8080", "listen address; localhost is both loopback addresses (an address other machines reach exposes your git-bug data and forge tokens to them)")
	meFlag(cmd, &me)
	return cmd
}
