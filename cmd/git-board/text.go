package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// textFlags are the ways a command takes a text, as gh does: -b, -F (a
// file, - for stdin) or -e ($EDITOR).
type textFlags struct {
	body   string
	file   string
	editor bool
}

func (t *textFlags) add(cmd *cobra.Command, what string) {
	cmd.Flags().StringVarP(&t.body, "body", "b", "", "the "+what+", as text")
	cmd.Flags().StringVarP(&t.file, "body-file", "F", "", "read the "+what+" from a file (- for stdin)")
	cmd.Flags().BoolVarP(&t.editor, "editor", "e", false, "write the "+what+" in your editor (git's: GIT_EDITOR, core.editor, VISUAL, EDITOR)")
	cmd.MarkFlagsMutuallyExclusive("body", "body-file", "editor")
}

// given tells a text was given (-b "" is: an empty one).
func (t *textFlags) given(cmd *cobra.Command) bool {
	return cmd.Flags().Changed("body") || t.file != "" || t.editor
}

// read is the text -b or -F gives; -e is the caller's, which knows what
// the buffer holds.
func (t *textFlags) read(in io.Reader) (string, error) {
	switch {
	case t.file == "-":
		data, err := io.ReadAll(in)
		return trimText(string(data)), err
	case t.file != "":
		data, err := os.ReadFile(t.file)
		return trimText(string(data)), err
	}
	return trimText(t.body), nil
}

// trimText drops the blank lines a text starts with and the space it ends
// with, as an editor or a file leaves them.
func trimText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimRight(strings.TrimLeft(s, "\n"), " \t\n")
}

// errAborted: the editor left nothing to write.
var errAborted = errors.New("aborted: nothing written")

// runEditor opens initial in the user's editor, as git does (git var
// GIT_EDITOR), and returns what they saved. Tests replace it.
var runEditor = func(name, initial string) (string, error) {
	out, err := exec.Command("git", "var", "GIT_EDITOR").Output()
	if err != nil {
		return "", fmt.Errorf("finding your editor (git var GIT_EDITOR): %w", err)
	}
	editor := strings.TrimSpace(string(out))
	f, err := os.CreateTemp("", "git-board-*-"+name+".md")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(initial); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	cmd := exec.Command("sh", "-c", editor+` "$@"`, editor, f.Name())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("the editor (%s): %w", editor, err)
	}
	data, err := os.ReadFile(f.Name())
	return string(data), err
}

// splitTitle reads an editor's buffer as gh -e does: the first line the
// title, the rest the body.
func splitTitle(buf string) (title, body string) {
	buf = trimText(buf)
	title, body, _ = strings.Cut(buf, "\n")
	return strings.TrimSpace(title), trimText(body)
}

// keep saves a text a write refused, so it isn't lost, and says where.
func keep(name, text string) string {
	f, err := os.CreateTemp("", "git-board-*-"+name+".md")
	if err != nil {
		return ""
	}
	defer f.Close()
	if _, err := f.WriteString(text + "\n"); err != nil {
		return ""
	}
	return f.Name()
}

// keptErr is err, naming where the text it refused is kept.
func keptErr(err error, name, text string) error {
	if path := keep(name, text); path != "" {
		return fmt.Errorf("%w (your text is kept in %s)", err, path)
	}
	return err
}
