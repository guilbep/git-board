// Package golden runs a git-board binary over the golden boards and
// compares what it prints with what v1 printed (design.md, section 3,
// Golden boards). It knows the binary only by its command line: v1's
// git-board records the outputs, and git-board-next must give them again
// (make golden, make compat).
//
// The boards are in next/testdata/golden/boards, made by v1gen; the
// outputs in next/testdata/golden/cli/<board>: index.txt lists each case,
// a name, an exit code and a command line; <name>.out is its stdout and
// <name>.err its stderr, each written when not empty. next/testdata/compat
// /allowed.txt lists the differences a binary may have, each with its
// reason.
package golden

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Board is a golden board's folder.
type Board struct {
	Name      string
	Dir       string
	Me        string   // board.me
	Datastore string   // board.datastore; "" for git
	Commands  []string // the cases beyond the automatic ones, one command line each
}

// Boards are the boards under root, by name.
func Boards(root string) ([]Board, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []Board
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b := Board{Name: e.Name(), Dir: filepath.Join(root, e.Name())}
		me, err := os.ReadFile(filepath.Join(b.Dir, "me"))
		if err != nil {
			return nil, err
		}
		b.Me = strings.TrimSpace(string(me))
		if ds, err := os.ReadFile(filepath.Join(b.Dir, "datastore")); err == nil {
			b.Datastore = strings.TrimSpace(string(ds))
		}
		if cmds, err := os.ReadFile(filepath.Join(b.Dir, "commands")); err == nil {
			for _, line := range strings.Split(string(cmds), "\n") {
				if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
					b.Commands = append(b.Commands, line)
				}
			}
		}
		out = append(out, b)
	}
	return out, nil
}

// Checkout makes a checkout of the board at dir, a folder that doesn't
// exist yet: a git repository on main with the board's refs, or with its
// Markdown folder committed, its .git-board.toml, and board.me (and
// board.datastore) in its git config. No remote: nothing is fetched.
func (b Board) Checkout(ctx context.Context, dir string) error {
	toml, err := os.ReadFile(filepath.Join(b.Dir, "git-board.toml"))
	if err != nil {
		return err
	}
	if err := Git(ctx, "", "init", "-q", "-b", "main", dir); err != nil {
		return err
	}
	if bundle := filepath.Join(b.Dir, "board.bundle"); exists(bundle) {
		if err := Git(ctx, dir, "fetch", "-q", "--update-head-ok", bundle, "+refs/*:refs/*"); err != nil {
			return err
		}
		if err := Git(ctx, dir, "rev-parse", "-q", "--verify", "refs/heads/main"); err == nil {
			if err := Git(ctx, dir, "reset", "-q", "--hard", "main"); err != nil {
				return err
			}
		}
	}
	if err := os.WriteFile(filepath.Join(dir, ".git-board.toml"), toml, 0o644); err != nil {
		return err
	}
	if files := filepath.Join(b.Dir, "files"); exists(files) {
		folder := strings.TrimPrefix(b.Datastore, "markdown:")
		if err := copyDir(files, filepath.Join(dir, folder)); err != nil {
			return err
		}
		if err := Git(ctx, dir, "add", "-A"); err != nil {
			return err
		}
		if err := Git(ctx, dir, "commit", "-q", "-m", "golden"); err != nil {
			return err
		}
	}
	if err := Git(ctx, dir, "config", "board.me", b.Me); err != nil {
		return err
	}
	if b.Datastore != "" {
		return Git(ctx, dir, "config", "board.datastore", b.Datastore)
	}
	return nil
}

// IssueIDs are every issue of the checkout, deleted ones too, sorted:
// the ids of refs/board/issue/*, or of the Markdown folder's pack files.
func (b Board) IssueIDs(ctx context.Context, checkout string) ([]string, error) {
	var ids []string
	if b.Datastore != "" {
		folder := strings.TrimPrefix(b.Datastore, "markdown:")
		files, err := filepath.Glob(filepath.Join(checkout, folder, ".board", "issue", "*.jsonl"))
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			ids = append(ids, strings.TrimSuffix(filepath.Base(f), ".jsonl"))
		}
	} else {
		out, err := output(ctx, checkout, "git", "for-each-ref", "--format=%(refname)", "refs/board/issue/")
		if err != nil {
			return nil, err
		}
		for _, ref := range strings.Fields(out) {
			ids = append(ids, strings.TrimPrefix(ref, "refs/board/issue/"))
		}
	}
	ids = slices.DeleteFunc(ids, func(id string) bool { return !isID.MatchString(id) })
	slices.Sort(ids)
	return ids, nil
}

var isID = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Case is one command run on a board.
type Case struct {
	Name string
	Line string // the command line, after the binary's name
}

// Cases are the board's cases: ls --json, show and log --json of every
// issue, then its commands.
func (b Board) Cases(ids []string) []Case {
	cs := []Case{{"ls", "ls --json"}}
	for _, id := range ids {
		cs = append(cs, Case{"show-" + id[:12], "show " + id + " --json"}, Case{"log-" + id[:12], "log " + id + " --json"})
	}
	for i, line := range b.Commands {
		cs = append(cs, Case{fmt.Sprintf("cmd%02d-%s", i+1, slug(line)), line})
	}
	return cs
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slug(line string) string {
	s := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(line), "-"), "-")
	if len(s) > 40 {
		s = strings.TrimRight(s[:40], "-")
	}
	return s
}

// Result is what a command did.
type Result struct {
	Exit           int
	Stdout, Stderr []byte
}

// Env is the environment every command runs in: a home of its own, no
// system git config, UTC, the C locale, no terminal, and none of
// git-board's own variables.
func Env(home string) []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch {
		case strings.HasPrefix(k, "GIT_"), strings.HasPrefix(k, "XDG_"), strings.HasPrefix(k, "LC_"),
			k == "HOME", k == "TZ", k == "LANG", k == "TERM", k == "GH_TOKEN", k == "GITHUB_TOKEN", k == "GITLAB_TOKEN", k == "EDITOR", k == "VISUAL", k == "PAGER":
		default:
			env = append(env, kv)
		}
	}
	return append(env, "HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"), "GIT_CONFIG_NOSYSTEM=1",
		"TZ=UTC", "LC_ALL=C", "LANG=C", "TERM=dumb", "NO_COLOR=1", "GIT_BOARD_NO_PROMPT=1",
		"GIT_AUTHOR_NAME=golden", "GIT_AUTHOR_EMAIL=golden@example.com", "GIT_COMMITTER_NAME=golden", "GIT_COMMITTER_EMAIL=golden@example.com")
}

// Run runs bin with the case's command line in checkout, and answers what
// it printed, normalised (Normalise).
func Run(ctx context.Context, bin, checkout, home string, c Case) (Result, error) {
	args, err := Split(c.Line)
	if err != nil {
		return Result{}, err
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = checkout
	cmd.Env = Env(home)
	cmd.Stdin = bytes.NewReader(nil)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	var ee *exec.ExitError
	r := Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	switch {
	case errors.As(err, &ee):
		r.Exit = ee.ExitCode()
	case err != nil:
		return r, err
	}
	return Normalise(r, checkout, home), nil
}

// Normalise takes out of a result what differs from run to run. Each
// normalisation is listed in docs/rewrite/compat.md, Normalisations
// (keep the two in step):
//
//  1. the checkout's path is $BOARD, and the home's $HOME;
//  2. a log line's date and time prefix (log.Printf's "2006/01/02
//     15:04:05 ") is taken off.
func Normalise(r Result, checkout, home string) Result {
	for _, p := range []*[]byte{&r.Stdout, &r.Stderr} {
		s := *p
		for _, dir := range paths(checkout) {
			s = bytes.ReplaceAll(s, []byte(dir), []byte("$BOARD"))
		}
		for _, dir := range paths(home) {
			s = bytes.ReplaceAll(s, []byte(dir), []byte("$HOME"))
		}
		*p = s
	}
	r.Stderr = logStamp.ReplaceAll(r.Stderr, nil)
	return r
}

var logStamp = regexp.MustCompile(`(?m)^\d{4}/\d\d/\d\d \d\d:\d\d:\d\d `)

// paths are dir as given and with its links resolved, longest first.
func paths(dir string) []string {
	out := []string{dir}
	if real, err := filepath.EvalSymlinks(dir); err == nil && real != dir {
		out = append(out, real)
	}
	slices.SortFunc(out, func(a, b string) int { return len(b) - len(a) })
	return out
}

// Split cuts a command line into arguments, as a shell would for the
// lines the goldens hold: words apart by spaces, "double quotes" around a
// word with spaces, \" and \\ inside them.
func Split(line string) ([]string, error) {
	var args []string
	var cur strings.Builder
	in, word := false, false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		switch {
		case in && ch == '\\' && i+1 < len(line) && (line[i+1] == '"' || line[i+1] == '\\'):
			i++
			cur.WriteByte(line[i])
		case ch == '"':
			in, word = !in, true
		case !in && ch == ' ':
			if word {
				args = append(args, cur.String())
				cur.Reset()
				word = false
			}
		default:
			cur.WriteByte(ch)
			word = true
		}
	}
	if in {
		return nil, fmt.Errorf("%s: a quote isn't closed", line)
	}
	if word {
		args = append(args, cur.String())
	}
	return args, nil
}

// Recorded is a board's recorded outputs.
type Recorded struct {
	Cases   []Case
	Results map[string]Result
}

// Write stores the results of cases in dir, replacing what it held.
func Write(dir string, cases []Case, results map[string]Result) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var index bytes.Buffer
	index.WriteString("# name\texit\tcommand (git-board <command>): <name>.out is its stdout, <name>.err its stderr\n")
	for _, c := range cases {
		r := results[c.Name]
		fmt.Fprintf(&index, "%s\t%d\t%s\n", c.Name, r.Exit, c.Line)
		for ext, data := range map[string][]byte{".out": r.Stdout, ".err": r.Stderr} {
			if len(data) == 0 {
				continue
			}
			if err := os.WriteFile(filepath.Join(dir, c.Name+ext), data, 0o644); err != nil {
				return err
			}
		}
	}
	return os.WriteFile(filepath.Join(dir, "index.txt"), index.Bytes(), 0o644)
}

// Read reads what Write stored.
func Read(dir string) (Recorded, error) {
	rec := Recorded{Results: map[string]Result{}}
	f, err := os.Open(filepath.Join(dir, "index.txt"))
	if err != nil {
		return rec, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			return rec, fmt.Errorf("%s: %q: name, exit and command, apart by tabs", f.Name(), line)
		}
		exit, err := strconv.Atoi(parts[1])
		if err != nil {
			return rec, fmt.Errorf("%s: %q: %w", f.Name(), line, err)
		}
		r := Result{Exit: exit}
		if r.Stdout, err = readIf(filepath.Join(dir, parts[0]+".out")); err != nil {
			return rec, err
		}
		if r.Stderr, err = readIf(filepath.Join(dir, parts[0]+".err")); err != nil {
			return rec, err
		}
		rec.Cases = append(rec.Cases, Case{parts[0], parts[2]})
		rec.Results[parts[0]] = r
	}
	return rec, sc.Err()
}

func readIf(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

// Allowed is allowed.txt: the differences a binary may have from the
// recorded outputs, each with its reason.
type Allowed []Allow

// Allow is one line: a board, a case (* for every case of the board, a
// prefix ending in * for those it starts), what differs (exit, stdout,
// stderr, or * for all), and why.
type Allow struct {
	Board, Case, What, Why string
}

// ReadAllowed reads allowed.txt: one difference a line, "board case what
// why…"; # starts a comment. A line without its reason is refused.
func ReadAllowed(path string) (Allowed, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out Allowed
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 4 {
			return nil, fmt.Errorf("%s:%d: board, case, what (exit, stdout, stderr or *), then the reason", path, i+1)
		}
		if !slices.Contains([]string{"exit", "stdout", "stderr", "*"}, f[2]) {
			return nil, fmt.Errorf("%s:%d: %q: exit, stdout, stderr or *", path, i+1, f[2])
		}
		out = append(out, Allow{f[0], f[1], f[2], strings.Join(f[3:], " ")})
	}
	return out, nil
}

// Allows says whether a difference in what is allowed, and why.
func (a Allowed) Allows(board, name, what string) (string, bool) {
	for _, x := range a {
		match := x.Case == name || x.Case == "*" || strings.HasSuffix(x.Case, "*") && strings.HasPrefix(name, strings.TrimSuffix(x.Case, "*"))
		if (x.Board == board || x.Board == "*") && match && (x.What == what || x.What == "*") {
			return x.Why, true
		}
	}
	return "", false
}

// Diff is a short account of where two outputs part: the first line that
// differs, with its number.
func Diff(want, got []byte) string {
	w, g := strings.Split(string(want), "\n"), strings.Split(string(got), "\n")
	for i := 0; i < max(len(w), len(g)); i++ {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl || i >= len(w) || i >= len(g) {
			return fmt.Sprintf("line %d:\n  want %q\n  got  %q", i+1, wl, gl)
		}
	}
	return "the same lines"
}

// Git runs git in dir with Env's settings.
func Git(ctx context.Context, dir string, args ...string) error {
	_, err := output(ctx, dir, "git", args...)
	return err
}

func output(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = Env(os.TempDir())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, stderr.Bytes())
	}
	return string(out), nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// copyDir copies the files of from into to, which it makes.
func copyDir(from, to string) error {
	return filepath.WalkDir(from, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
