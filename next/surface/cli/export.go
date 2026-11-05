package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/next/domain/issues/mdfiles"
)

// git board export --md and import --md, as v1's: the Markdown folder's
// files (mdfiles), in a folder of one's choosing.

func newExport() *cobra.Command {
	var md string
	cmd := &cobra.Command{
		Use:   "export --md <dir>",
		Short: "Write every issue as a Markdown file, NNN-slug.md, in a folder",
		Long: `Write every issue as dir/NNN-slug.md: YAML front matter (id, number,
title, state, fields, labels, people, created, updated), the description,
then each comment with its author and date. A file is written only when it
changed, so a second run with no change writes nothing. A file whose issue
got a new title is removed; any other file in dir is left alone.`,
		Example: `  git board export --md issues`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if md == "" {
				return errors.New("export needs --md <dir>")
			}
			b, err := open(cmd, false)
			if err != nil {
				return err
			}
			defer b.Close()
			s := b.issues.Schema
			flat := func(l string) bool { _, ok := s.Kinds[l]; return ok && s.FlatKinds() }
			written, err := mdfiles.Export(cmd.Context(), b.issues, md, flat)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for _, name := range written {
				fmt.Fprintln(out, name)
			}
			fmt.Fprintf(out, "%d written in %s\n", len(written), md)
			return nil
		},
	}
	cmd.Flags().StringVar(&md, "md", "", "the folder to write the Markdown files in")
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
			var rep mdfiles.Report
			switch {
			case b.Files != nil && (md == "" || sameDir(md, b.Files.Dir())):
				rep, err = b.Files.Import(cmd.Context())
			case md == "":
				return errors.New("import needs --md <dir>")
			default:
				rep, err = mdfiles.Import(cmd.Context(), b.issues, md)
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

// sameDir says a and b are one folder, links resolved.
func sameDir(a, b string) bool {
	abs := func(p string) string {
		p, _ = filepath.Abs(p)
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}
		return p
	}
	return abs(a) == abs(b)
}
