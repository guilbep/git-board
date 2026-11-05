package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/guilbep/board-agents/internal/wtx"
)

// Setup is whether agents can start here: wtx doctor --json, wtx.toml,
// and wtx init --edit's answers (what wtx.toml says now, as JSON),
// git-board v1's Settings › Worktrees.
type Setup struct {
	Wtx     bool            `json:"wtx"`               // on PATH
	Reason  string          `json:"reason,omitempty"`  // why nothing can start; "" when it can
	Code    string          `json:"code,omitempty"`    // no-wtx, no-config
	Doctor  *wtx.Doctor     `json:"doctor,omitempty"`  // wtx doctor --json
	Config  bool            `json:"config"`            // wtx.toml in the repository
	Answers json.RawMessage `json:"answers,omitempty"` // wtx init --edit --no-notes
}

// Doctor reads the setup.
func (s *Service) Doctor(ctx context.Context) (Setup, error) {
	out := Setup{Wtx: wtx.Available()}
	if !out.Wtx {
		out.Code, out.Reason = errNoWtx.(*Refusal).Code, errNoWtx.Error()
		return out, nil
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	// doctor exits 1 on a hard failure, its report still on stdout
	report, _ := wtx.Run(ctx, s.Root, "doctor", "--json")
	var d wtx.Doctor
	if err := json.Unmarshal(report, &d); err != nil {
		return out, errors.New("wtx doctor --json: " + err.Error())
	}
	out.Doctor, out.Config = &d, d.Has("wtx.toml")
	answers, err := wtx.Run(ctx, s.Root, "init", "--edit", "--no-notes")
	if err == nil && !json.Valid(answers) {
		err = errors.New("wtx init --edit: not JSON")
	}
	if err != nil {
		return out, err
	}
	out.Answers = answers
	if !out.Config {
		out.Code, out.Reason = "no-config", "No wtx.toml in this repository yet"
	}
	return out, nil
}

// File is a file wtx init writes.
type File struct {
	Path string `json:"path"`
	Text string `json:"text"`
}

// Init runs wtx init --from-json on answers (Doctor's Answers, edited).
// Without write it runs in a scratch repository and answers what it
// wrote: the bytes a write puts in the checkout. With write it writes
// wtx.toml and .wt.toml in the main checkout, uncommitted, replacing them.
func (s *Service) Init(ctx context.Context, answers []byte, write bool) ([]File, error) {
	if !wtx.Available() {
		return nil, errNoWtx
	}
	if !json.Valid(answers) || string(answers) == "null" {
		return nil, errors.New("the answers are not JSON")
	}
	file, err := tempFile("board-agents-wtx-*.json", answers)
	if err != nil {
		return nil, err
	}
	defer os.Remove(file)
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	dir, args := s.Root, []string{"init", "--from-json", file}
	if write {
		// ponytail: --force always; the preview is what replaces the
		// file, and wtx's --edit carried its values.
		args = append(args, "--force")
	} else {
		if dir, err = os.MkdirTemp("", "board-agents-wtx-preview-"); err != nil {
			return nil, err
		}
		defer os.RemoveAll(dir)
		if out, err := exec.CommandContext(ctx, "git", "init", "-q", dir).CombinedOutput(); err != nil {
			return nil, errors.New("git init: " + string(out))
		}
	}
	if _, err := wtx.Run(ctx, dir, args...); err != nil {
		return nil, err
	}
	var out []File
	for _, name := range []string{"wtx.toml", ".wt.toml"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		out = append(out, File{Path: name, Text: string(data)})
	}
	return out, nil
}
