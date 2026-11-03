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
