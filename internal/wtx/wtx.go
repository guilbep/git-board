// Package wtx runs the wtx CLI (one agent, one worktree, one branch, one
// PR) on the machine where git-board serves. Every call is an argv to
// exec, never a shell line.
package wtx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Available tells whether wtx is on PATH where the server runs.
func Available() bool {
	_, err := exec.LookPath("wtx")
	return err == nil
}

// Run runs wtx with args as an argv, in dir, and returns its stdout. A
// non-zero exit is an error carrying wtx's last lines.
func Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "wtx", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("wtx %s: %w: %s", args[0], err, Tail(stderr.String()+stdout.String(), 8))
	}
	return stdout.Bytes(), nil
}

// Tail is the last n non-empty lines of s.
func Tail(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// Check is one line of wtx doctor --json.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
	Fix    string `json:"fix"`
	Hard   bool   `json:"hard"`
}

// Doctor is wtx doctor --json.
type Doctor struct {
	OK     bool    `json:"ok"`
	Checks []Check `json:"checks"`
}

// Has tells whether the check named name, or starting with name+" in "
// ("wtx.toml in co2-calculator"), passed.
func (d Doctor) Has(name string) bool {
	for _, c := range d.Checks {
		if c.Name == name || strings.HasPrefix(c.Name, name+" in ") {
			return c.OK
		}
	}
	return false
}

// Session is one row of wtx status --json.
type Session struct {
	Branch  string `json:"branch"`
	Path    string `json:"path"`
	Main    bool   `json:"main"`
	Session string `json:"session"`
	Live    bool   `json:"live"`
	State   string `json:"state"` // permission, idle, stop, start, running, or "": stale when not live
	Agent   string `json:"agent"`
	LLM     string `json:"llm"`
}

// ParseStatus reads wtx status --json.
func ParseStatus(out []byte) ([]Session, error) {
	var rows []Session
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("wtx status: %w", err)
	}
	return rows, nil
}
