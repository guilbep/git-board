package mdfiles

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/kernel"
)

// git board export --md and import --md (v1's ExportMarkdown and
// ImportMarkdown): the Markdown folder's files, in a folder of one's
// choosing, written or read once.

// Export writes each issue as dir/NNN-slug.md, in the folder's format, and
// answers the files it wrote: none when nothing changed. flat tells a
// label is a flat kind; nil for none. A file of an exported issue under
// another name, after a title change, is removed, and a deleted issue's;
// any other file is left.
func Export(ctx context.Context, svc *issues.Service, dir string, flat func(label string) bool) ([]string, error) {
	ctx = svc.Board.Snapshot(ctx)
	heads, _, err := svc.Heads(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]kernel.ID, len(heads))
	nameOf := map[string]string{}
	all, err := names(heads)
	if err != nil {
		return nil, err
	}
	for i, h := range heads {
		ids[i], nameOf[h.ID] = kernel.ID(h.ID), all[i]
	}
	ths, err := svc.Threads(ctx, ids)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	old, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, err
	}
	for _, p := range old {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		f, err := parse(data)
		if err != nil {
			continue // not ours
		}
		name, ok := nameOf[f.front.ID]
		deleted := false
		if !ok && isID(f.front.ID) {
			e, err := issues.Kind.Get(ctx, svc.Board, kernel.ID(f.front.ID))
			deleted = err == nil && e.State.Deleted
		}
		if ok && name != filepath.Base(p) || deleted {
			if err := os.Remove(p); err != nil {
				return nil, err
			}
		}
	}
	var written []string
	for i, th := range ths {
		name := all[i]
		data, err := RenderFlat(th, flat)
		if err != nil {
			return nil, err
		}
		p := filepath.Join(dir, name)
		if cur, err := os.ReadFile(p); err == nil && bytes.Equal(cur, data) {
			continue
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return nil, err
		}
		written = append(written, name)
	}
	return written, nil
}

// Report is what an import did, by file name.
type Report struct {
	Created, Updated, Conflicts, Skipped []string
	Unchanged                            int
}

// Import reads every dir/*.md, in the order of their names, into the
// board svc writes, as who writes: a file of an issue here gives it what
// differs, and any other a new issue, its new id written back into the
// file. A Markdown file that isn't an issue's is skipped. On a Markdown
// folder board, use the Folder's Import.
func Import(ctx context.Context, svc *issues.Service, dir string) (Report, error) {
	rep := Report{Created: []string{}, Updated: []string{}, Conflicts: []string{}, Skipped: []string{}}
	paths, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return rep, err
	}
	slices.Sort(paths)
	im := &importer{f: &Folder{Issues: svc}}
	for _, p := range paths {
		name := filepath.Base(p)
		data, err := os.ReadFile(p)
		if err != nil {
			return rep, err
		}
		f, err := parse(data)
		if err != nil {
			rep.Skipped = append(rep.Skipped, name+": "+err.Error())
			continue
		}
		var before kernel.Version
		here := false
		if isID(f.front.ID) {
			e, err := issues.Kind.Get(ctx, svc.Board, kernel.ID(f.front.ID))
			switch {
			case err == nil && e.State.Deleted:
				rep.Skipped = append(rep.Skipped, name+": its issue was deleted")
				continue
			case err == nil:
				before, here = e.Version, true
			case !errors.Is(err, kernel.ErrNotFound):
				return rep, err
			}
		}
		im.conflicts = nil
		id, rewrite, err := im.read(ctx, name, f)
		if err != nil {
			return rep, fmt.Errorf("%s: %w", name, err)
		}
		rep.Conflicts = append(rep.Conflicts, im.conflicts...)
		if !here {
			if err := os.WriteFile(p, rewrite, 0o644); err != nil {
				return rep, err
			}
			rep.Created = append(rep.Created, name)
			continue
		}
		e, err := issues.Kind.Get(ctx, svc.Board, id)
		switch {
		case err != nil:
			return rep, err
		case e.Version != before:
			rep.Updated = append(rep.Updated, name)
		case len(im.conflicts) == 0:
			rep.Unchanged++
		}
	}
	return rep, nil
}

// Import is Import on the folder's own files, under its lock, then
// rendered: the folder's next write reads them in anyway.
func (f *Folder) Import(ctx context.Context) (Report, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	unlock, err := f.lock(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("markdown: %w", err)
	}
	defer unlock()
	ctx = context.WithValue(ctx, within{f}, true)
	k := f.load()
	defer f.save(k)
	rep, err := Import(ctx, f.Issues, f.dir)
	if err != nil {
		return rep, err
	}
	if _, err := Export(ctx, f.Issues, f.dir, nil); err != nil {
		return rep, err
	}
	return rep, f.render(ctx, k)
}
