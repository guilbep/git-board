// Command git-board-agents starts a coding agent on a git-board issue, in
// the issue's own worktree, through wtx; lists, briefs and stops them; and
// asks an AI for an issue's risk. It talks to the board through the
// git-board CLI and, when one runs, the board's server: see README.md.
//
// As git-board-agents on PATH, git runs it as git board-agents.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/guilbep/board-agents/internal/agent"
	"github.com/guilbep/board-agents/internal/ai"
	"github.com/guilbep/board-agents/internal/board"
)

// Exit codes.
const (
	exitError    = 1
	exitUsage    = 2
	exitNotFound = 3 // no such issue: git-board's 3
	exitRefused  = 4 // refused before wtx ran: the code says why
)

const usage = `git-board-agents: a coding agent per git-board issue, in its own worktree (wtx)

Usage:
  git board-agents status                 every worktree, its session and issue
  git board-agents show <issue>           the issue's branch, worktree and session
  git board-agents start <issue>          wtx go: a worktree, a session, the agent on a prompt
  git board-agents brief <issue>          wtx brief: a prompt to the running agent
  git board-agents approve <issue>        tell the agent its plan is approved on the board
  git board-agents stop <issue>           wtx teardown: end the session, keep the worktree
  git board-agents done <issue>           wtx done: close the worktree, keep the branch
  git board-agents doctor                 whether agents can start here (wtx doctor)
  git board-agents init --answers <file>  wtx init --from-json: preview, --write to write
  git board-agents risk <issue>           the AI's risk class for the issue

<issue> is what git board show takes: a short id, CO2-3075, #2528.

Flags:
  -C <dir>              the repository (default: the current directory)
  --board <url>         the board's server (default: GIT_BOARD_URL, else the one git board runs here)
  --git-board <path>    the git-board binary (default: GIT_BOARD_BIN, else git-board on PATH)
  --json                print JSON
  --brief <text>        start, brief: what you add to the prompt
  -F <file>             start, brief: the brief from a file, - for stdin
  --comment <ref>       brief: a line comment (its ref, 58f8d9b:0) whose thread the agent answers
  --force               done: discard uncommitted changes
  --write               init: write wtx.toml and .wt.toml (default: print them);
                        risk: write the class as a comment in the cycle's risk step

Exit codes: 0 done, 1 an error, 2 a wrong command line, 3 no such issue, 4 refused (its code says why).
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// opts are the command line's flags.
type opts struct {
	dir, url, bin   string
	json            bool
	brief, file     string
	comment         string
	force, write    bool
	answers         string
	positional      []string
	stdin           io.Reader
	stdout, stderrW io.Writer
}

// parse reads flags anywhere among the arguments, as git does.
func parse(args []string) (*opts, error) {
	o := &opts{}
	fs := flag.NewFlagSet("git-board-agents", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.dir, "C", ".", "")
	fs.StringVar(&o.url, "board", "", "")
	fs.StringVar(&o.bin, "git-board", "", "")
	fs.BoolVar(&o.json, "json", false, "")
	fs.StringVar(&o.brief, "brief", "", "")
	fs.StringVar(&o.file, "F", "", "")
	fs.StringVar(&o.comment, "comment", "", "")
	fs.BoolVar(&o.force, "force", false, "")
	fs.BoolVar(&o.write, "write", false, "")
	fs.StringVar(&o.answers, "answers", "", "")
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return o, nil
		}
		o.positional = append(o.positional, args[0])
		args = args[1:]
	}
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	o, err := parse(args)
	switch {
	case errors.Is(err, flag.ErrHelp):
		fmt.Fprint(stdout, usage)
		return 0
	case err != nil:
		fmt.Fprintf(stderr, "error: %v\n\n%s", err, usage)
		return exitUsage
	case len(o.positional) == 0:
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	o.stdin, o.stdout, o.stderrW = stdin, stdout, stderr
	cmd, rest := o.positional[0], o.positional[1:]
	if cmd == "help" {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if o.comment != "" && cmd != "brief" {
		fmt.Fprintln(stderr, "error: --comment goes with brief only")
		return exitUsage
	}
	if code, ok := wantArgs(cmd, rest); !ok {
		fmt.Fprintf(stderr, "error: %s\n", code)
		return exitUsage
	}
	svc, err := service(ctx, o)
	if err != nil {
		return fail(stderr, err)
	}
	if err := dispatch(ctx, svc, o, cmd, rest); err != nil {
		return fail(stderr, err)
	}
	return 0
}

// wantArgs checks the command and its argument count.
func wantArgs(cmd string, rest []string) (string, bool) {
	switch cmd {
	case "status", "doctor", "init":
		if len(rest) != 0 {
			return cmd + " takes no argument", false
		}
	case "show", "start", "go", "brief", "approve", "stop", "done", "risk":
		if len(rest) != 1 {
			return cmd + " takes one issue", false
		}
	default:
		return fmt.Sprintf("unknown command %q: git board-agents help", cmd), false
	}
	return "", true
}

// service finds the main checkout, the git-board binary and the board's
// server.
func service(ctx context.Context, o *opts) (*agent.Service, error) {
	root, err := mainCheckout(ctx, o.dir)
	if err != nil {
		return nil, err
	}
	bin := o.bin
	if bin == "" {
		bin = os.Getenv("GIT_BOARD_BIN")
	}
	if bin == "" {
		bin = "git-board"
	}
	u := strings.TrimRight(o.url, "/")
	if u == "" {
		u = board.Discover(ctx, root)
	}
	return &agent.Service{Board: &board.Client{Bin: bin, Dir: root, URL: u}, Root: root}, nil
}

// mainCheckout is the main working tree of the repository at dir, where
// wtx runs: the parent of the common git directory, else dir's top level.
func mainCheckout(ctx context.Context, dir string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return "", fmt.Errorf("%s: not a git repository", dir)
	}
	common := strings.TrimSpace(string(out))
	if filepath.Base(common) == ".git" {
		return filepath.Dir(common), nil
	}
	top, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("%s: no working tree", dir)
	}
	return strings.TrimSpace(string(top)), nil
}

func dispatch(ctx context.Context, svc *agent.Service, o *opts, cmd string, rest []string) error {
	switch cmd {
	case "status":
		rows, note, err := svc.List(ctx)
		if err != nil {
			return err
		}
		if o.json {
			return printJSON(o.stdout, map[string]any{"agents": orEmpty(rows), "note": note})
		}
		printRows(o.stdout, rows)
		if note != "" {
			fmt.Fprintln(o.stderrW, "note:", note)
		}
		return nil
	case "show":
		a, err := svc.Show(ctx, rest[0])
		return printAgent(o, a, err)
	case "start", "go", "brief", "approve":
		action := map[string]string{"start": "go", "go": "go", "brief": "brief", "approve": "approve"}[cmd]
		brief, err := readBrief(o)
		if err != nil {
			return err
		}
		a, err := svc.Act(ctx, action, rest[0], agent.Request{Brief: brief, Comment: o.comment})
		return printAgent(o, a, err)
	case "stop":
		a, err := svc.Stop(ctx, rest[0])
		return printAgent(o, a, err)
	case "done":
		a, err := svc.Done(ctx, rest[0], o.force)
		return printAgent(o, a, err)
	case "doctor":
		s, err := svc.Doctor(ctx)
		if err != nil {
			return err
		}
		if o.json {
			return printJSON(o.stdout, s)
		}
		printSetup(o.stdout, s)
		return nil
	case "init":
		if o.answers == "" {
			return usageError("init needs --answers <file>: doctor --json prints the current ones")
		}
		answers, err := readFile(o.stdin, o.answers)
		if err != nil {
			return err
		}
		files, err := svc.Init(ctx, answers, o.write)
		if err != nil {
			return err
		}
		if o.json {
			return printJSON(o.stdout, map[string]any{"files": files, "written": o.write})
		}
		for _, f := range files {
			fmt.Fprintf(o.stdout, "==> %s\n%s\n", f.Path, strings.TrimRight(f.Text, "\n"))
		}
		if !o.write {
			fmt.Fprintln(o.stderrW, "note: a preview; --write writes them in", svc.Root)
		}
		return nil
	case "risk":
		s, err := svc.SuggestRisk(ctx, ai.FromEnv(), rest[0], o.write)
		if err != nil {
			return err
		}
		if o.json {
			return printJSON(o.stdout, s)
		}
		fmt.Fprintf(o.stdout, "%s: %s (%s)\n%s\n", s.Issue, s.Class, s.Model, s.Why)
		return nil
	}
	return usageError("unknown command " + cmd)
}

type usageError string

func (e usageError) Error() string { return string(e) }

// readBrief is --brief, or -F's file.
func readBrief(o *opts) (string, error) {
	if o.file == "" {
		return o.brief, nil
	}
	if o.brief != "" {
		return "", usageError("--brief or -F, not both")
	}
	data, err := readFile(o.stdin, o.file)
	return string(data), err
}

func readFile(stdin io.Reader, name string) ([]byte, error) {
	if name == "-" {
		return io.ReadAll(stdin)
	}
	return os.ReadFile(name)
}

func fail(stderr io.Writer, err error) int {
	var r *agent.Refusal
	var u usageError
	switch {
	case errors.As(err, &r):
		fmt.Fprintf(stderr, "error: %s (%s)\n", r.Reason, r.Code)
		return exitRefused
	case errors.As(err, &u):
		fmt.Fprintf(stderr, "error: %s\n", u)
		return exitUsage
	case errors.Is(err, board.ErrNotFound):
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitNotFound
	}
	fmt.Fprintf(stderr, "error: %v\n", err)
	return exitError
}

func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func printAgent(o *opts, a agent.Agent, err error) error {
	if err != nil {
		return err
	}
	if o.json {
		return printJSON(o.stdout, a)
	}
	w := o.stdout
	fmt.Fprintf(w, "%s  %s\n", a.Issue, a.Title)
	if a.Step != "" {
		fmt.Fprintf(w, "step      %s\n", a.Step)
	}
	fmt.Fprintf(w, "branch    %s\n", a.Branch)
	if a.Worktree != nil {
		fmt.Fprintf(w, "worktree  %s\n", a.Worktree.Path)
	} else {
		fmt.Fprintln(w, "worktree  none")
	}
	if a.Session != nil {
		fmt.Fprintf(w, "session   %s, %s\n", a.Session.Session, sessionState(a.Session.Live, a.Session.State))
	}
	if a.Moved != "" {
		fmt.Fprintf(w, "status    moved to %s\n", a.Moved)
	}
	if a.Output != "" {
		fmt.Fprintf(w, "\n%s\n", a.Output)
	}
	return nil
}

func sessionState(live bool, state string) string {
	switch {
	case !live:
		return "stale"
	case state == "":
		return "live"
	}
	return state
}

func printRows(w io.Writer, rows []agent.Row) {
	if len(rows) == 0 {
		fmt.Fprintln(w, "No worktree: git board-agents start <issue> makes one.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "BRANCH\tISSUE\tSTEP\tSESSION\tSTATE\tPATH")
	for _, r := range rows {
		session := "-"
		if r.Session != nil {
			session = sessionState(r.Session.Live, r.Session.State)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", r.Branch, dash(r.Issue), dash(r.Step), session, dash(r.State), r.Path)
	}
	tw.Flush()
}

func printSetup(w io.Writer, s agent.Setup) {
	if s.Reason != "" {
		fmt.Fprintf(w, "Agents can't start here: %s (%s)\n", s.Reason, s.Code)
	} else {
		fmt.Fprintln(w, "Agents can start here.")
	}
	if s.Doctor != nil {
		for _, c := range s.Doctor.Checks {
			mark := "ok  "
			if !c.OK {
				mark = "FAIL"
			}
			fmt.Fprintf(w, "  %s %s", mark, c.Name)
			if !c.OK && c.Fix != "" {
				fmt.Fprintf(w, ": %s", c.Fix)
			}
			fmt.Fprintln(w)
		}
	}
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
