package proof

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/guilbep/git-board/internal/config"
)

// Timeout bounds a run; one that outlives it is FAILED.
const Timeout = 20 * time.Minute

// passEnv are the server's variables a check sees, beside its own env:
// where the tools and their caches are, nothing else (ADR 0001, section 4).
var passEnv = []string{"PATH", "HOME", "TMPDIR", "GOCACHE", "GOMODCACHE", "GOPATH", "GOFLAGS", "npm_config_cache"}

// Run runs a check at commit in a clean checkout — `git worktree add
// --detach`, so nothing of the working tree leaks in — with a cleared
// environment, and records its result: the output kept as the log, the
// line signed with keyFile and appended to the writer's note at the
// check's key. Exit 0 is PASSED, any other exit or the timeout FAILED; a
// check that can't start (no checkout, no binary) is an error and records
// nothing.
func Run(ctx context.Context, dir, commit, name string, s config.Suite, keyFile, writer string) (Result, error) {
	if s.Where == "ci" {
		return Result{}, fmt.Errorf("%s runs on the forge (where = \"ci\")", name)
	}
	if err := rev(commit); err != nil {
		return Result{}, err
	}
	out, err := git(ctx, dir, nil, "rev-parse", "--verify", commit+"^{commit}")
	if err != nil {
		return Result{}, err
	}
	commit = strings.TrimSpace(string(out))
	key, err := Key(ctx, dir, commit, name, s)
	if err != nil {
		return Result{}, err
	}
	logs, err := LogDir(ctx, dir)
	if err != nil {
		return Result{}, err
	}

	tree, err := os.MkdirTemp("", "git-board-ci-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(tree)
	if _, err := git(ctx, dir, nil, "worktree", "add", "--detach", tree, commit); err != nil {
		return Result{}, err
	}
	defer func() { _, _ = git(context.Background(), dir, nil, "worktree", "remove", "--force", tree) }()

	env := make([]string, 0, len(passEnv)+len(s.Env))
	for _, k := range passEnv {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	for k, v := range s.Env {
		env = append(env, k+"="+v)
	}
	cwd := filepath.Join(tree, s.Dir)

	var tools bytes.Buffer
	for _, argv := range s.Tools {
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
		cmd.Dir, cmd.Env = cwd, env
		o, err := cmd.CombinedOutput()
		if err != nil {
			return Result{}, fmt.Errorf("%s: tools %s: %w: %s", name, strings.Join(argv, " "), err, bytes.TrimSpace(o))
		}
		fmt.Fprintf(&tools, "%s\n%s", argv, o)
	}

	runCtx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, s.Run[0], s.Run[1:]...)
	var log bytes.Buffer
	cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = cwd, env, &log, &log
	start := time.Now()
	err = cmd.Run()
	dur := time.Since(start)
	verdict := Passed
	var exit *exec.ExitError
	switch {
	case err == nil:
	case ctx.Err() != nil: // the server stopping is no verdict
		return Result{}, ctx.Err()
	case runCtx.Err() != nil:
		fmt.Fprintf(&log, "\ngit-board: stopped after %s\n", Timeout)
		verdict = Failed
	case errors.As(err, &exit):
		verdict = Failed
	default:
		return Result{}, fmt.Errorf("%s: %w", name, err)
	}

	sum := sha256.Sum256(log.Bytes())
	logHash := hex.EncodeToString(sum[:])
	if err := os.WriteFile(filepath.Join(logs, logHash), log.Bytes(), 0o644); err != nil {
		return Result{}, err
	}
	toolsSum := sha256.Sum256(tools.Bytes())
	r := Result{Check: name, Verdict: verdict, Key: key, Platform: runtime.GOOS + "/" + runtime.GOARCH,
		Tools: "sha256:" + hex.EncodeToString(toolsSum[:]), Commit: commit, At: start.UTC().Truncate(time.Second),
		Dur: dur.Round(time.Second), Log: "sha256:" + logHash}
	if r.Sig, err = Sign(ctx, keyFile, Payload(r.Line())); err != nil {
		return Result{}, err
	}
	return r, Append(ctx, dir, writer, key, r.Line())
}

// LogDir is where this clone keeps its runs' output, by sha256: the
// common git directory's git-board/ci, shared by its worktrees.
func LogDir(ctx context.Context, dir string) (string, error) {
	out, err := git(ctx, dir, nil, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	logs := filepath.Join(strings.TrimSpace(string(out)), "git-board", "ci")
	return logs, os.MkdirAll(logs, 0o755)
}
