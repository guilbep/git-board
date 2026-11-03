package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/config"
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

--template starts it from a template instead: a built-in one by name
(enac, EPFL-ENAC's labels and GitHub Project), a file, or an http(s) URL.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd.OutOrStdout(), template)
		},
	}
	cmd.Flags().StringVar(&template, "template", "", "start from a template: enac, a file or a URL")
	return cmd
}

// readTemplate is the file a template names: a built-in one (enac), an
// http(s) URL, or a path; "" is the default. It must check, or nothing is
// written.
func readTemplate(spec string) (string, error) {
	var s string
	var err error
	switch {
	case spec == "":
		return config.Default, nil
	case strings.HasPrefix(spec, "https://") || strings.HasPrefix(spec, "http://"):
		s, err = fetchTemplate(spec)
	case !strings.ContainsAny(spec, `/\.`):
		s, err = config.Template(spec)
	default:
		var b []byte
		b, err = os.ReadFile(spec)
		s = string(b)
	}
	if err != nil {
		return "", err
	}
	if _, _, err := config.Check([]byte(s)); err != nil {
		return "", fmt.Errorf("template %s: %w", spec, err)
	}
	return s, nil
}

func fetchTemplate(url string) (string, error) {
	c := http.Client{Timeout: 30 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("template %s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return string(b), err
}

func runInit(out io.Writer, template string) error {
	path := config.Path()
	if path == config.FileName {
		return errors.New("not in a git repository")
	}
	content, err := readTemplate(template)
	if err != nil {
		return err
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
