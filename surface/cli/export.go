package cli

import (
	"cmp"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	httpapi "github.com/guilbep/git-board/surface/http"
)

// git board export --md and import --md, as v1's: the Markdown folder's
// files (mdfiles), in a folder of one's choosing.

func newExport() *cobra.Command {
	var md, site, title string
	cmd := &cobra.Command{
		Use:   "export --md <dir> | --html <dir>",
		Short: "Write every issue as a Markdown file, NNN-slug.md, or as a read-only web site, in a folder",
		Long: `Write every issue as dir/NNN-slug.md: YAML front matter (id, number,
title, state, fields, labels, people, created, updated), the description,
then each comment with its author and date. A file is written only when it
changed, so a second run with no change writes nothing. A file whose issue
got a new title is removed; any other file in dir is left alone.

--html writes a read-only web site instead: dir/index.html lists the
issues, open first, and dir/NNN.html shows each one with its comments,
the text cleaned as the web app shows it. No page writes, and no script
runs, so GitHub Pages or any web server can serve dir. --title names the
site (default: the repository's folder name).`,
		Example: `  git board export --md issues
  git board export --html site --title "co2-calculator issues"`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			switch {
			case md == "" && site == "":
				return errors.New("export needs --md <dir>") // v1's words (golden)
			case md != "" && site != "":
				return usageError{errors.New("export takes --md <dir> or --html <dir>, not both")}
			}
			b, err := open(cmd, false)
			if err != nil {
				return err
			}
			defer b.Close()
			var written []string
			if site != "" {
				written, err = httpapi.WriteSite(cmd.Context(), b.Board, site, cmp.Or(title, filepath.Base(b.Dir)+" issues"))
			} else {
				written, err = b.ExportMarkdown(cmd.Context(), md)
			}
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for _, name := range written {
				fmt.Fprintln(out, name)
			}
			fmt.Fprintf(out, "%d written in %s\n", len(written), cmp.Or(md, site))
			return nil
		},
	}
	cmd.Flags().StringVar(&md, "md", "", "the folder to write the Markdown files in")
	cmd.Flags().StringVar(&site, "html", "", "the folder to write the read-only web site in")
	cmd.Flags().StringVar(&title, "title", "", "the site's title, with --html")
	return cmd
}

func newImport() *cobra.Command {
	var md string
	cmd := &cobra.Command{
		Use:   "import --md <dir>",
		Short: "Read a folder of Markdown issues into the board, as export writes them",
		Long: `Read every dir/*.md that export wrote, or that was written the same way.

A file whose id is an issue here gives it what differs, as you: the title,
the description, the state, the fields, labels and assignees, a comment's
text, and new comments (a "### <who> · <time>" heading with no marker).
When the issue changed since the file's updated, a field that differs is a
conflict: the board's value stays, and it is listed.

A file with no id, or one this board doesn't have, is a new issue, with the
author, comments and times it names; its new id is written back into it.
A Markdown file that isn't an issue file is skipped. On a board kept in
Markdown files (board.datastore markdown:<dir>), --md is that folder, and
the next write reads it in anyway.`,
		Example: `  git board export --md issues
  $EDITOR issues/0042-*.md
  git board import --md issues`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := open(cmd, true)
			if err != nil {
				return err
			}
			defer b.Close()
			rep, ok, err := b.ImportMarkdown(cmd.Context(), md)
			if !ok {
				return errors.New("import needs --md <dir>")
			}
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for _, n := range rep.Created {
				fmt.Fprintln(out, "created", n)
			}
			for _, n := range rep.Updated {
				fmt.Fprintln(out, "updated", n)
			}
			for _, c := range rep.Conflicts {
				fmt.Fprintln(out, "conflict", c)
			}
			for _, s := range rep.Skipped {
				fmt.Fprintln(out, "skipped", s)
			}
			fmt.Fprintf(out, "%d created, %d updated, %d unchanged, %d conflicts\n", len(rep.Created), len(rep.Updated), rep.Unchanged, len(rep.Conflicts))
			return nil
		},
	}
	cmd.Flags().StringVar(&md, "md", "", "the folder to read the Markdown files from")
	return cmd
}
