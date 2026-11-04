package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/store"
)

func newExportCmd() *cobra.Command {
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
		RunE: func(cmd *cobra.Command, args []string) error {
			if md == "" {
				return fmt.Errorf("export needs --md <dir>")
			}
			return runExport(cmd.OutOrStdout(), md)
		},
	}
	cmd.Flags().StringVar(&md, "md", "", "the folder to write the Markdown files in")
	return cmd
}

func runExport(out io.Writer, dir string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	b, err := store.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()
	flat := func(l string) bool { _, ok := cfg.Branches.Kinds[l]; return ok && cfg.FlatKinds() }
	written, err := b.ExportMarkdown(dir, flat)
	if err != nil {
		return err
	}
	for _, name := range written {
		fmt.Fprintln(out, name)
	}
	fmt.Fprintf(out, "%d written in %s\n", len(written), dir)
	return nil
}

func newImportCmd() *cobra.Command {
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImport(cmd.OutOrStdout(), md)
		},
	}
	cmd.Flags().StringVar(&md, "md", "", "the folder to read the Markdown files from")
	return cmd
}

func runImport(out io.Writer, dir string) error {
	b, err := store.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()
	if dir == "" {
		dir = b.MarkdownDir()
	}
	if dir == "" {
		return fmt.Errorf("import needs --md <dir>")
	}
	rep, err := b.ImportMarkdown(dir)
	if err != nil {
		return err
	}
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
}
