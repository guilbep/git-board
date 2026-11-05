package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/issue/boardgen"
	"github.com/guilbep/git-board/core/mdstore"
	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/core/syncer"
	"github.com/guilbep/git-board/internal/store"
)

// openBoard writes into a repository that exists.
func openBoard(ctx context.Context, dir string) (*board, error) {
	r, err := git.PlainOpen(dir)
	if err != nil {
		return nil, err
	}
	return &board{ctx: ctx, dir: dir, repo: r, refs: map[string]plumbing.Hash{}}, nil
}

// entity is an entity of the repository, at its ref, whose clock is clock.
func (b *board) entity(kind, id string, clock uint64) (*entity, error) {
	ref, err := b.repo.Reference(plumbing.ReferenceName(gitstore.EntityRef(kind, id)), true)
	if err != nil {
		return nil, err
	}
	return &entity{b: b, kind: kind, id: id, head: ref.Hash(), clock: clock}, nil
}

// fetch fetches the four kinds of from into refs/board-remotes/<name>/,
// as storage.md's fetch does, and merges them with v1's merge.
func fetch(ctx context.Context, dir, from, name string) error {
	args := []string{"fetch", "-q", "--prune", from}
	for _, kind := range []string{"issue", "person", "sync", "release"} {
		args = append(args, fmt.Sprintf("+refs/board/%s/*:refs/board-remotes/%s/%s/*", kind, name, kind))
	}
	if err := run(ctx, dir, "git", args...); err != nil {
		return err
	}
	s, err := gitstore.Open(dir)
	if err != nil {
		return err
	}
	defer s.Close()
	_, err = oplog.MergeFetched(ctx, s, name)
	return err
}

// merge is two clones of one board that wrote the same entities offline,
// then merged each other's heads with v1's merge: an issue edited on both
// (its body twice), a sync entity made on both with its given id (two
// first commits), and the diverging pair storage.md's Known point names:
// both clones merge at once, then one merges the other's merge.
func merge(ctx context.Context, dir string) (meta, error) {
	a, err := newBoard(ctx, dir)
	if err != nil {
		return meta{}, err
	}
	p := persons(a)
	x := a.create("issue", "", "c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0",
		op("create", p.ada, at(0, 0), m{"title": "Edited on two clones", "body": "The first body."}),
		op("change-labels", p.ada, at(0, 0), m{"add": []string{"status:ready"}}))
	z := a.create("issue", "", "c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1",
		op("create", p.ada, at(0, 10), m{"title": "Appended on the other clone", "body": ""}))
	if err := a.finish(); err != nil {
		return meta{}, err
	}

	bdir := dir + "-b"
	defer os.RemoveAll(bdir)
	if err := run(ctx, "", "git", "init", "-q", "-b", "main", bdir); err != nil {
		return meta{}, err
	}
	if err := run(ctx, bdir, "git", "fetch", "-q", dir, "+refs/board/*:refs/board/*"); err != nil {
		return meta{}, err
	}

	// offline, on a
	a, _ = openBoard(ctx, dir)
	xa, _ := a.entity("issue", x.id, x.clock)
	xa.add(op("comment", p.ada, at(1, 0), m{"body": "Written on clone a."}))
	xa.add(op("edit-text", p.ada, at(1, 5), m{"target": x.id + ":0", "body": "The body, as clone a wrote it."}),
		op("change-labels", p.ada, at(1, 5), m{"add": []string{"prio:p1"}, "remove": []string{"status:ready"}}))
	a.create("sync", syncer.StateID("github.com/acme/merge"), "5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a",
		op("link", p.ada, at(1, 10), m{"issue": x.id, "forge_id": "100", "number": "1", "url": "https://github.com/acme/merge/issues/1"}))
	if err := a.finish(); err != nil {
		return meta{}, err
	}

	// offline, on b
	b, _ := openBoard(ctx, bdir)
	xb, _ := b.entity("issue", x.id, x.clock)
	xb.add(op("set-title", p.bob, at(1, 1), m{"title": "Edited on two clones (title from b)"}),
		op("comment", p.bob, at(1, 1), m{"body": "Written on clone b."}))
	xb.add(op("edit-text", p.bob, at(1, 6), m{"target": x.id + ":0", "body": "The body, as clone b wrote it."}),
		op("change-labels", p.bob, at(1, 6), m{"add": []string{"status:in-progress", "prio:p2"}}))
	zb, _ := b.entity("issue", z.id, z.clock)
	zb.add(op("comment", p.bob, at(1, 20), m{"body": "Only b wrote this: a fast-forwards."}))
	b.create("sync", syncer.StateID("github.com/acme/merge"), "5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b",
		op("link", p.bob, at(1, 11), m{"issue": x.id, "forge_id": "200", "number": "2", "url": "https://github.com/acme/merge/issues/2"}))
	b.create("issue", "", "c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2",
		op("create", p.bob, at(1, 30), m{"title": "Made on b alone", "body": "a creates its ref at b's head."}))
	if err := b.finish(); err != nil {
		return meta{}, err
	}

	// both fetch, then both merge, at once
	for _, step := range [][3]string{{dir, bdir, "b"}, {bdir, dir, "a"}} {
		args := []string{"fetch", "-q", step[1]}
		for _, kind := range []string{"issue", "person", "sync", "release"} {
			args = append(args, fmt.Sprintf("+refs/board/%s/*:refs/board-remotes/%s/%s/*", kind, step[2], kind))
		}
		if err := run(ctx, step[0], "git", args...); err != nil {
			return meta{}, err
		}
	}
	for _, step := range [][2]string{{dir, "b"}, {bdir, "a"}} {
		s, err := gitstore.Open(step[0])
		if err != nil {
			return meta{}, err
		}
		_, err = oplog.MergeFetched(ctx, s, step[1])
		s.Close()
		if err != nil {
			return meta{}, err
		}
	}
	// a merges b's merge: the merge of two merges
	if err := fetch(ctx, dir, bdir, "b"); err != nil {
		return meta{}, err
	}
	if err := run(ctx, dir, "sh", "-c", `git for-each-ref --format='delete %(refname)' refs/board-remotes/ | git update-ref --stdin`); err != nil {
		return meta{}, err
	}
	return meta{toml: config, me: p.ada, commands: []string{`ls "is:open,closed" --json`, `ls "#1" --json`, `ls "#2" --json`}}, nil
}

// broken is a board some of whose entities no reader can fold: an edit of
// an operation that isn't there, an operation of an unknown kind, a pack
// that isn't JSON, a wrong clock, a log that doesn't start with a create.
// The others still list.
func broken(ctx context.Context, dir string) (meta, error) {
	b, err := newBoard(ctx, dir)
	if err != nil {
		return meta{}, err
	}
	p := persons(b)
	b.create("issue", "", "b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0",
		op("create", p.ada, at(0, 0), m{"title": "A good issue", "body": "Lists."}))
	e := b.create("issue", "", "b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1",
		op("create", p.ada, at(0, 1), m{"title": "Edits an operation that isn't there", "body": ""}))
	e.add(op("edit-text", p.ada, at(0, 2), m{"target": strings.Repeat("0", 40) + ":0", "body": "lost"}))
	k := b.create("issue", "", "b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2",
		op("create", p.ada, at(0, 3), m{"title": "An unknown operation", "body": ""}))
	k.add(op("set-priority", p.ada, at(0, 4), m{"priority": "high"}))
	j := b.create("issue", "", "b3b3b3b3b3b3b3b3b3b3b3b3b3b3b3b3",
		op("create", p.ada, at(0, 5), m{"title": "A pack that isn't JSON", "body": ""}))
	j.addRaw([]byte("this is not JSON\n"), at(0, 6))
	c := b.create("issue", "", "b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4",
		op("create", p.ada, at(0, 7), m{"title": "A wrong clock", "body": ""}))
	c.addRaw(encode(pack{Clock: 5, Ops: []oplog.Op{op("comment", p.ada, at(0, 8), m{"body": "clock 5 on clock 1"})}}), at(0, 8))
	b.create("issue", "", "b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5",
		op("comment", p.ada, at(0, 9), m{"body": "no create first"}))
	n := b.create("issue", "", "b6b6b6b6b6b6b6b6b6b6b6b6b6b6b6b6",
		op("create", p.ada, at(0, 10), m{"title": "An operation with no author", "body": ""}))
	n.add(op("comment", "", at(0, 11), m{"body": "who?"}))
	s := b.create("issue", "", "b7b7b7b7b7b7b7b7b7b7b7b7b7b7b7b7",
		op("create", p.ada, at(0, 12), m{"title": "A status that isn't one", "body": ""}))
	s.add(op("set-status", p.ada, at(0, 13), m{"status": "wontfix"}))
	if err := b.finish(); err != nil {
		return meta{}, err
	}
	return meta{toml: config, me: p.ada, commands: []string{`ls "is:open,closed" --json`}}, nil
}

// generated is boardgen's board, as v1 writes a board of its own (15
// issues, 5 persons, a fixed seed, random nonces), and one issue with a
// long history: 160 packs.
func generated(ctx context.Context, dir string) (meta, error) {
	g, err := boardgen.Git(ctx, dir, boardgen.Options{Issues: 15, Persons: 5, Seed: 7})
	if err != nil {
		return meta{}, err
	}
	b, err := openBoard(ctx, dir)
	if err != nil {
		return meta{}, err
	}
	who := func(i int) string { return g.Persons[i%len(g.Persons)] }
	long := b.create("issue", "", "f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0",
		op("create", who(0), at(0, 0), m{"title": "A long history (0)", "body": "Version 0 of the body."}),
		op("change-labels", who(0), at(0, 0), m{"add": []string{"status:backlog", "prio:p3", "cat:chore"}}))
	prios := []string{"p0", "p1", "p2", "p3"}
	statuses := []string{"backlog", "ready", "in-progress", "in-review", "qa", "validated"}
	var comments []string
	for i := 1; i < 160; i++ {
		t := at(0, 7*i)
		switch {
		case i%50 == 0:
			long.add(op("set-title", who(i), t, m{"title": fmt.Sprintf("A long history (%d)", i)}))
		case i%40 == 0:
			long.add(op("set-status", who(i), t, m{"status": map[bool]string{true: "closed", false: "open"}[i%80 == 0]}))
		case i%7 == 0:
			long.add(op("change-labels", who(i), t, m{"add": []string{"prio:" + prios[i%4]}, "remove": []string{"prio:" + prios[(i+3)%4]}}),
				op("change-labels", who(i), t, m{"add": []string{"status:" + statuses[i%6]}, "remove": []string{"status:" + statuses[(i+5)%6]}}))
		case i%5 == 0 && len(comments) > 0:
			long.add(op("edit-text", who(i), t, m{"target": comments[len(comments)-1], "body": fmt.Sprintf("Comment edited at step %d.", i)}))
		case i%11 == 0:
			long.add(op("edit-text", who(i), t, m{"target": long.id + ":0", "body": fmt.Sprintf("Version %d of the body.", i)}))
		case i%13 == 0 && len(comments) > 1:
			long.add(op("delete", who(i), t, m{"target": comments[len(comments)-2]}))
		default:
			comments = append(comments, long.add(op("comment", who(i), t, m{"body": fmt.Sprintf("Comment %d.", i)}))+":0")
		}
	}
	if err := b.finish(); err != nil {
		return meta{}, err
	}
	return meta{toml: boardgen.Config, me: g.Persons[0], commands: []string{
		`ls "is:open,closed" --json`,
		`ls "status:ready" --json`,
		`ls "prio:p0 is:closed" --json`,
		`ls "sprint:delivery" --sort "est desc" --json`,
		`ls -- "-cat:bug area:backend" --json`,
		`ls "export invoice" --json`,
		`ls "in:title calculation" --json`,
		`ls "comments:0" --json`,
		`ls "assignee:dev1,dev2" --sort "updated asc" --json`,
		`ls "is:open,closed" --sort title --limit 5 --json id,title`,
		`ls "updated:>=2024-01-05" --json id`,
		`ls "is:open,closed" --at 2024-01-02 --json`,
	}}, nil
}

// markdown is a board kept in a folder of Markdown files (board.datastore
// markdown:issues): copied from a git board by v1's datastore copy, with
// its counter, then an issue written on two branches and merged by git's
// union driver, then rendered by v1.
func markdown(ctx context.Context, dir string) (meta, error) {
	b, err := newBoard(ctx, dir)
	if err != nil {
		return meta{}, err
	}
	b.code(at(-50, 0), "Start\n", map[string][]byte{"README.md": []byte("# widgets\n"), ".git-board.toml": []byte(config)})
	p := persons(b)
	one := b.create("issue", "", "d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0",
		op("create", p.ada, at(0, 0), m{"title": "Kept in a folder", "body": "A body with a heading:\n\n### not a comment · 2025-01-01T00:00:00Z\n\nstill the body."}),
		op("change-labels", p.ada, at(0, 0), m{"add": []string{"status:ready", "prio:p1", "assignee:ada", "bug", "plain"}}))
	one.add(op("comment", p.bob, at(1, 0), m{"body": "A comment."}))
	uni := b.create("issue", "", "d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1",
		op("create", p.chloe, at(2, 0), m{"title": "Ünïcödé 日本語 🚀", "body": "é é"}))
	uni.add(op("set-status", p.chloe, at(2, 5), m{"status": "closed"}))
	gone := b.create("issue", "", "d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2",
		op("create", p.bob, at(3, 0), m{"title": "Deleted", "body": ""}))
	gone.add(op("delete", p.ada, at(3, 1), m{"target": gone.id + ":0"}))
	seq := b.commit(b.tree(map[string][]byte{"numbers": []byte(fmt.Sprintf("prefix MD\nstart 1\n1 %s\n2 %s\n", one.id, uni.id))}), at(4, 0))
	b.refs["refs/board/seq"] = seq
	if err := b.finish(); err != nil {
		return meta{}, err
	}

	v1, err := store.Open(dir)
	if err != nil {
		return meta{}, err
	}
	if _, err := v1.CopyGitTo(ctx, "markdown:issues"); err != nil {
		v1.Close()
		return meta{}, err
	}
	v1.Close()
	if err := run(ctx, dir, "git", "add", "issues"); err != nil {
		return meta{}, err
	}
	if err := run(ctx, dir, "git", "commit", "-q", "-m", "The board, in issues/"); err != nil {
		return meta{}, err
	}

	// one issue written on two branches, each its next pack, then merged
	md, err := mdstore.Open(filepath.Join(dir, "issues"), filepath.Join(dir, ".git", "board"))
	if err != nil {
		return meta{}, err
	}
	appendPack := func(o oplog.Op) error {
		packs, v, err := md.Packs(ctx, "issue", one.id)
		if err != nil {
			return err
		}
		_ = packs
		_, err = md.Add(ctx, "issue", one.id, encode(pack{Clock: one.clock + 1, Ops: []oplog.Op{o}}), v)
		return err
	}
	steps := []func() error{
		func() error { return run(ctx, dir, "git", "checkout", "-q", "-b", "side") },
		func() error {
			return appendPack(op("comment", p.chloe, at(5, 0), m{"body": "Written on the branch side."}))
		},
		func() error { return run(ctx, dir, "git", "commit", "-q", "-am", "side") },
		func() error { return run(ctx, dir, "git", "checkout", "-q", "main") },
		func() error {
			return appendPack(op("set-title", p.ada, at(5, 1), m{"title": "Kept in a folder, merged by union"}))
		},
		func() error { return run(ctx, dir, "git", "commit", "-q", "-am", "main") },
		func() error { return run(ctx, dir, "git", "merge", "-q", "--no-edit", "side") },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return meta{}, err
		}
	}
	if err := run(ctx, dir, "git", "config", "board.datastore", "markdown:issues"); err != nil {
		return meta{}, err
	}
	v1, err = store.Open(dir)
	if err != nil {
		return meta{}, err
	}
	_, err = v1.ExportMarkdown(v1.MarkdownDir(), nil)
	v1.Close()
	if err != nil {
		return meta{}, err
	}
	// what git ignores in the folder, the lock files, isn't the board
	if err := run(ctx, dir, "git", "clean", "-qfdX", "issues"); err != nil {
		return meta{}, err
	}
	return meta{toml: config, me: p.ada, datastore: "markdown:issues", files: filepath.Join(dir, "issues"),
		commands: []string{`ls "is:open,closed" --json`, `ls "MD-1" --json`, `ls "folder" --json`}}, nil
}
