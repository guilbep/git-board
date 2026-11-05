package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"

	"github.com/spf13/cobra"

	httpapi "github.com/guilbep/git-board/next/surface/http"
)

// The server: `git-board-next serve`, and a bare `git-board-next`, which
// opens this repository's board in the browser, where it already runs,
// else started here until Ctrl-C (docs/rewrite/api.md, Serving).

// logFlags are the server's logs' level and format: flags over
// GIT_BOARD_LOG_LEVEL and GIT_BOARD_LOG_FORMAT.
type logFlags struct{ level, format string }

func (l *logFlags) add(cmd *cobra.Command) {
	cmd.Flags().StringVar(&l.level, "log-level", "", "the server's logs: debug, info, warn or error (env "+httpapi.LogEnv+"; default info)")
	cmd.Flags().StringVar(&l.format, "log-format", "", "the server's logs: text or json (env "+httpapi.LogFormatEnv+"; default text)")
}

func (l *logFlags) options(stderr io.Writer, dir, addr string, ready func(string)) (httpapi.ServeOptions, error) {
	level, format := l.level, l.format
	if level == "" {
		level = os.Getenv(httpapi.LogEnv)
	}
	if format == "" {
		format = os.Getenv(httpapi.LogFormatEnv)
	}
	log, err := httpapi.NewLogger(stderr, level, format)
	if err != nil {
		return httpapi.ServeOptions{}, usageError{err}
	}
	return httpapi.ServeOptions{Dir: dir, Addr: addr, Log: log, Version: version(), Ready: ready}, nil
}

func newServe() *cobra.Command {
	var addr string
	var logs logFlags
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the web app and its API, without opening a browser",
		Long: `Serve the web app and its JSON API (docs/rewrite/api.md) for the board of
the repository in the working directory, until stopped. No login: it
listens on this machine only by default, and refuses what another site
could make a browser send (ADR 0013). Its logs go to stderr.`,
		Example: `  git-board-next serve
  git-board-next serve --addr 127.0.0.1:9090 --log-level debug --log-format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			o, err := logs.options(cmd.ErrOrStderr(), cwd, addr, func(url string) {
				fmt.Fprintf(cmd.OutOrStdout(), "git-board listening on %s\n", url)
				flush(cmd.OutOrStdout())
			})
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return httpapi.Serve(ctx, o)
		},
	}
	// this machine only by default: the server writes to git, and to the
	// forges with the user's tokens; anyone who can reach it can
	cmd.Flags().StringVar(&addr, "addr", "localhost:8080", "listen address; localhost is both loopback addresses (an address other machines reach exposes your board and forge tokens to them)")
	logs.add(cmd)
	return cmd
}

// runBoard is a bare git-board-next: this repository's board in the
// browser, where it already runs, else started here until Ctrl-C.
func runBoard(cmd *cobra.Command, logs *logFlags, open bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	show := func(url string) {
		if !open {
			return
		}
		if err := openURL(url); err != nil {
			fmt.Fprintf(out, "opening the browser: %v\n", err)
		}
	}
	gitDir := httpapi.GitDir(cmd.Context(), cwd)
	if gitDir == "" {
		return fmt.Errorf("not in a git repository: %s", cwd)
	}
	if inst, ok := httpapi.Running(gitDir); ok {
		fmt.Fprintf(out, "git-board: this repository's board runs at %s (pid %d)\n", inst.URL, inst.PID)
		show(inst.URL)
		return nil
	}
	o, err := logs.options(cmd.ErrOrStderr(), cwd, "", func(url string) {
		fmt.Fprintf(out, "git-board: this repository's board is at %s (Ctrl-C stops it)\n", url)
		flush(out)
		show(url)
	})
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return httpapi.Serve(ctx, o)
}

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

// flush writes what out holds now: a server's address is read while it
// runs.
func flush(out io.Writer) {
	if f, ok := out.(interface{ Flush() error }); ok {
		f.Flush()
	}
}

// version is the binary's: its module's, or its commit's.
func version() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	v := bi.Main.Version
	for _, s := range bi.Settings {
		if s.Key == "vcs.revision" && (v == "" || v == "(devel)") {
			v = s.Value[:min(12, len(s.Value))]
		}
	}
	if v == "" {
		v = "(devel)"
	}
	return v
}
