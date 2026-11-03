package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/fromforge"
)

func newInitCmd() *cobra.Command {
	var template string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Write the default .git-board.toml at the repository's root",
		Long: `Writes the default .git-board.toml at the repository's root: the
statuses, priorities, kinds and estimates as fields, branch names, and
four views (all, backlog, board, mine), each explained in a comment.
Settings › Project file offers the same when the repository has none. It
never overwrites one.

When origin is on GitHub, the file starts as its labels and its first
Project's fields (names, colors, descriptions, values in order); the
template fills what they don't have. No token, or nothing read: the
template, and a line saying what wasn't read.

--template names the template, else the global file's, else the default: a built-in one by name
(enac, EPFL-ENAC's labels and GitHub Project), a file, or an http(s) URL.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd.OutOrStdout(), template)
		},
	}
	cmd.Flags().StringVar(&template, "template", "", "start from a template: enac, a file or a URL")
	return cmd
}

func runInit(out io.Writer, template string) error {
	path := config.Path()
	if path == config.FileName {
		return errors.New("not in a git repository")
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}
	content, err := config.Start(template)
	if err != nil {
		return err
	}
	if repo := originOnGitHub(filepath.Dir(path)); repo != "" {
		forge, notes := fromforge.Read(context.Background(), repo)
		if forge.Project != nil || len(forge.Labels) > 0 {
			var more []string
			if content, more, err = fromforge.Merge(content, forge); err != nil {
				return err
			}
			notes = append(notes, more...)
			fmt.Fprintf(out, "from %s: %d labels, Project %s\n", repo, len(forge.Labels), projectName(forge))
		}
		for _, n := range notes {
			fmt.Fprintln(out, n)
		}
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%s already exists", path)
	}
	if err != nil {
		return err
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Fprintf(out, "wrote %s: edit it in Settings › Project file (git board), or by hand\n", path)
	return nil
}

// originOnGitHub is origin's owner/name when it is on github.com, else "".
func originOnGitHub(root string) string {
	url, err := exec.Command("git", "-C", root, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	f, _ := config.FromRemote(strings.TrimSpace(string(url)))
	if f.Kind != "github" || f.Host != "github.com" {
		return ""
	}
	return f.Project
}

func projectName(f fromforge.Forge) string {
	if f.Project == nil {
		return "none"
	}
	return fmt.Sprintf("%s/%d, %s", f.Project.Owner, f.Project.Number, f.Project.Title)
}
