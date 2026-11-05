package app

import (
	"context"
	"path/filepath"

	"github.com/guilbep/git-board/domain/issues/mdfiles"
)

// git board export --md and import --md: the Markdown folder's files
// (mdfiles), in a folder of one's choosing.

// ImportReport is what an import did, by file name.
type ImportReport = mdfiles.Report

// ExportMarkdown writes every issue as dir/NNN-slug.md, flat kinds as the
// kind field, and answers the files it wrote.
func (b *Board) ExportMarkdown(ctx context.Context, dir string) ([]string, error) {
	s := b.Issues.Schema
	flat := func(l string) bool { _, ok := s.Kinds[l]; return ok && s.FlatKinds() }
	return mdfiles.Export(ctx, b.Issues, dir, flat)
}

// ImportMarkdown reads every dir/*.md into the board; on a Markdown
// folder board, dir "" or the folder's own is read under its lock.
// ok is false when there is no folder to read.
func (b *Board) ImportMarkdown(ctx context.Context, dir string) (rep ImportReport, ok bool, err error) {
	switch {
	case b.Files != nil && (dir == "" || sameDir(dir, b.Files.Dir())):
		rep, err = b.Files.Import(ctx)
	case dir == "":
		return rep, false, nil
	default:
		rep, err = mdfiles.Import(ctx, b.Issues, dir)
	}
	return rep, true, err
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
