// Command v1gen makes the golden boards (next/testdata/golden/boards) with
// v1's code: the boards v2 must read as v1 does. It is the one package of
// next/ that imports v1 (next/guard_test.go): goldens are made by v1 or
// they prove nothing. It goes with v1, at the switch (design.md, M7).
//
//	go run ./next/golden/v1gen        # make golden-boards
//
// Each board is a folder: board.bundle (every ref of a git repository)
// or files/ (a checkout's files, for the Markdown board); git-board.toml,
// its .git-board.toml; me, its board.me; datastore, its board.datastore
// when it isn't git; commands, the commands recorded on it beyond the
// automatic ones (next/golden). The packs are written with fixed nonces
// and times, so the boards' ids are the same at each run, but the one
// boardgen makes, whose nonces are random.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/core/syncer"
)

func main() {
	out := flag.String("out", "next/testdata/golden/boards", "the boards' folder")
	only := flag.String("only", "", "make this board alone")
	flag.Parse()
	ctx := context.Background()
	abs, err := filepath.Abs(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "v1gen:", err)
		os.Exit(1)
	}
	*out = abs
	boards := []struct {
		name string
		make func(ctx context.Context, dir string) (meta, error)
	}{
		{"basic", basic},
		{"merge", merge},
		{"broken", broken},
		{"generated", generated},
		{"markdown", markdown},
	}
	for _, b := range boards {
		if *only != "" && *only != b.name {
			continue
		}
		if err := build(ctx, filepath.Join(*out, b.name), b.make); err != nil {
			fmt.Fprintf(os.Stderr, "v1gen: %s: %v\n", b.name, err)
			os.Exit(1)
		}
		fmt.Println(filepath.Join(*out, b.name))
	}
}

// meta is what a board's folder holds beside the board.
type meta struct {
	toml      string
	me        string
	datastore string   // "" for git
	commands  []string // recorded beyond the automatic ones
	files     string   // the folder the Markdown board's files are copied from, else ""
}

// build makes a board in a temporary repository and stores it at dir.
func build(ctx context.Context, dir string, mk func(context.Context, string) (meta, error)) error {
	tmp, err := os.MkdirTemp("", "v1gen-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	repo := filepath.Join(tmp, "repo")
	m, err := mk(ctx, repo)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if m.files != "" {
		if err := run(ctx, "", "cp", "-R", m.files, filepath.Join(dir, "files")); err != nil {
			return err
		}
	} else if err := run(ctx, repo, "git", "bundle", "create", "-q", filepath.Join(dir, "board.bundle"), "--branches", "--tags", "--glob=refs/board/*"); err != nil {
		return err
	}
	write := map[string]string{"git-board.toml": m.toml, "me": m.me + "\n"}
	if m.datastore != "" {
		write["datastore"] = m.datastore + "\n"
	}
	if len(m.commands) > 0 {
		write["commands"] = strings.Join(m.commands, "\n") + "\n"
	}
	for name, text := range write {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func run(ctx context.Context, dir string, name string, args ...string) error {
	_, err := output(ctx, dir, nil, name, args...)
	return err
}

func output(ctx context.Context, dir string, stdin []byte, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "LC_ALL=C", "TZ=UTC", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=golden", "GIT_AUTHOR_EMAIL=golden@example.com", "GIT_COMMITTER_NAME=golden", "GIT_COMMITTER_EMAIL=golden@example.com")
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, stderr.Bytes())
	}
	return strings.TrimSpace(string(out)), nil
}

// t0 is the boards' first second: 2025-03-03 09:00 UTC, a Monday.
const t0 = 1740992400

// at is t0 plus some hours and minutes.
func at(hours, minutes int) int64 { return t0 + int64(hours)*3600 + int64(minutes)*60 }

// board is a repository being written: its commits as go-git writes
// them, with fixed times, and its refs, moved at the end.
type board struct {
	ctx  context.Context
	dir  string
	repo *git.Repository
	refs map[string]plumbing.Hash
	err  error
}

func newBoard(ctx context.Context, dir string) (*board, error) {
	if err := run(ctx, "", "git", "init", "-q", "-b", "main", dir); err != nil {
		return nil, err
	}
	r, err := git.PlainOpen(dir)
	if err != nil {
		return nil, err
	}
	return &board{ctx: ctx, dir: dir, repo: r, refs: map[string]plumbing.Hash{}}, nil
}

func (b *board) fail(err error) {
	if b.err == nil && err != nil {
		b.err = err
	}
}

func (b *board) object(t plumbing.ObjectType, encode func(plumbing.EncodedObject) error) plumbing.Hash {
	o := b.repo.Storer.NewEncodedObject()
	o.SetType(t)
	if err := encode(o); err != nil {
		b.fail(err)
		return plumbing.ZeroHash
	}
	h, err := b.repo.Storer.SetEncodedObject(o)
	b.fail(err)
	return h
}

func (b *board) blob(data []byte) plumbing.Hash {
	return b.object(plumbing.BlobObject, func(o plumbing.EncodedObject) error {
		w, err := o.Writer()
		if err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
		return w.Close()
	})
}

func (b *board) tree(files map[string][]byte) plumbing.Hash {
	t := object.Tree{}
	for _, name := range sortedKeys(files) {
		t.Entries = append(t.Entries, object.TreeEntry{Name: name, Mode: filemode.Regular, Hash: b.blob(files[name])})
	}
	return b.object(plumbing.TreeObject, t.Encode)
}

// commit is an entity commit as storage.md has it: no name, no email, no
// message; when is its time.
func (b *board) commit(tree plumbing.Hash, when int64, parents ...plumbing.Hash) plumbing.Hash {
	sig := object.Signature{When: time.Unix(when, 0).UTC()}
	c := object.Commit{Author: sig, Committer: sig, TreeHash: tree, ParentHashes: parents}
	return b.object(plumbing.CommitObject, c.Encode)
}

// code commits files on main, as a person would, and answers the commit.
func (b *board) code(when int64, message string, files map[string][]byte) plumbing.Hash {
	sig := object.Signature{Name: "Ada Lovelace", Email: "ada@example.com", When: time.Unix(when, 0).UTC()}
	var parents []plumbing.Hash
	if h, ok := b.refs["refs/heads/main"]; ok {
		parents = append(parents, h)
	}
	c := object.Commit{Author: sig, Committer: sig, Message: message, TreeHash: b.tree(files), ParentHashes: parents}
	h := b.object(plumbing.CommitObject, c.Encode)
	b.refs["refs/heads/main"] = h
	return h
}

// finish moves the refs, in one update-ref, and checks main out.
func (b *board) finish() error {
	if b.err != nil {
		return b.err
	}
	var in bytes.Buffer
	for _, name := range sortedKeys(b.refs) {
		fmt.Fprintf(&in, "update %s %s\n", name, b.refs[name])
	}
	if _, err := output(b.ctx, b.dir, in.Bytes(), "git", "update-ref", "--stdin"); err != nil {
		return err
	}
	if _, ok := b.refs["refs/heads/main"]; ok {
		return run(b.ctx, b.dir, "git", "reset", "-q", "--hard", "main")
	}
	return nil
}

// pack is a pack as oplog writes it: its fields in this order.
type pack struct {
	Clock uint64     `json:"clock"`
	Nonce string     `json:"nonce,omitempty"`
	Ops   []oplog.Op `json:"ops"`
}

// entity is one entity being written: its head and its clock.
type entity struct {
	b     *board
	kind  string
	id    string
	head  plumbing.Hash
	clock uint64
}

// encode is a pack's bytes, compact, as oplog encodes it.
func encode(p pack) []byte {
	data, err := json.Marshal(p)
	if err != nil {
		panic(err)
	}
	return data
}

// create writes an entity's first pack; id "" for its hash.
func (b *board) create(kind, id, nonce string, ops ...oplog.Op) *entity {
	return b.createRaw(kind, id, encode(pack{Clock: 1, Nonce: nonce, Ops: ops}), ops[0].Time)
}

// createRaw writes a first pack of these bytes.
func (b *board) createRaw(kind, id string, data []byte, when int64) *entity {
	if id == "" {
		id = oplog.Hash(data)
	}
	e := &entity{b: b, kind: kind, id: id}
	e.head = b.commit(b.tree(map[string][]byte{"pack": data}), when)
	e.clock = 1
	b.refs[gitstore.EntityRef(kind, id)] = e.head
	return e
}

// add appends a pack of ops and answers its hash.
func (e *entity) add(ops ...oplog.Op) string {
	return e.addRaw(encode(pack{Clock: e.clock + 1, Ops: ops}), ops[0].Time)
}

// addRaw appends a pack of these bytes, whose clock must be the next.
func (e *entity) addRaw(data []byte, when int64) string {
	e.head = e.b.commit(e.b.tree(map[string][]byte{"pack": data}), when, e.head)
	e.clock++
	e.b.refs[gitstore.EntityRef(e.kind, e.id)] = e.head
	return oplog.Hash(data)
}

// op is an operation; data is marshalled as v1's constructors do, a map
// with its keys sorted.
func op(kind, author string, when int64, data map[string]any) oplog.Op {
	raw, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	return oplog.Op{Kind: kind, Author: author, Time: when, Data: raw}
}

func from(o oplog.Op, forge, event string) oplog.Op {
	o.Origin = &oplog.Origin{Forge: forge, Event: event}
	return o
}

type m = map[string]any

func sortedKeys[V any](mp map[string]V) []string {
	keys := make([]string, 0, len(mp))
	for k := range mp {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// The boards' configuration: fields, flat kinds ([branches] kinds
// without [fields.kind]), a role every issue inherits, sprints and a view.
const config = `[fields.status]
single = true
options = ["backlog", "ready", "in-progress", "in-review", "done"]

[fields.prio]
single = true
options = ["p0", "p1", "p2", "p3"]

[fields.est]
single = true
numeric = true

[fields.assignee]
single = false

[fields.sprint]
single = true

[fields.reviewer]
single = true

[branches]
kinds = { bug = "bug", feature = "feature" }

[roles]
reviewer = "chloe"

[sprints]
length_days = 14
start_weekday = "monday"

[[sprints.list]]
slug = "s1"
title = "Sprint 1"
start = 2025-03-03

[[sprints.list]]
slug = "s2"
title = "Sprint 2"
start = 2025-03-17

[views.ready]
filter = "status:ready is:open"
sort = "prio asc"
`

// people are the persons every hand-made board starts with.
type people struct {
	ada, bob, chloe, octo, dup string
}

// persons writes the people: ada, the board's first, made with no author;
// bob, whose email and GitHub account went; chloé, a name in Unicode;
// octocat, made by a pull; and dup, a second person holding ada's GitHub
// account (data-model.md, Who owns an account).
func persons(b *board) people {
	var p people
	ada := b.create("person", "", "0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a",
		op("create", "", at(-48, 0), m{"name": "Ada Lovelace", "email": "ada@example.com"}),
		op("add-account", "", at(-48, 0), m{"server": "github.com", "id": "1001", "login": "ada"}))
	p.ada = ada.id
	ada.add(op("set-name", p.ada, at(-47, 0), m{"name": "Ada King"}))
	ada.add(op("add-account", p.ada, at(-47, 5), m{"server": "gitlab.epfl.ch", "id": "77", "login": "aking"}),
		op("add-account", p.ada, at(-47, 5), m{"server": "github.com", "id": "1001", "login": "ada-l"}))

	bob := b.create("person", "", "0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b",
		op("create", p.ada, at(-46, 0), m{"name": "Bob", "email": "bob@example.com"}),
		op("add-account", p.ada, at(-46, 0), m{"server": "github.com", "id": "1002", "login": "bob"}))
	p.bob = bob.id
	bob.add(op("set-email", p.bob, at(-45, 0), m{"email": ""}))
	bob.add(op("remove-account", p.bob, at(-45, 1), m{"server": "github.com", "id": "1002"}))
	bob.add(op("add-account", p.bob, at(-45, 2), m{"server": "github.com", "id": "1003", "login": "bobby"}),
		op("remove-account", p.bob, at(-45, 2), m{"server": "gitlab.com", "id": "404"}))

	p.chloe = b.create("person", "", "0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c",
		op("create", p.ada, at(-44, 0), m{"name": "Chloé Dubois-Ångström", "email": "chloe@exämple.org"}),
		op("add-account", p.ada, at(-44, 0), m{"server": "github.com", "id": "1004", "login": "chloe"})).id
	p.octo = b.create("person", "", "0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d",
		op("create", "", at(-43, 0), m{"name": "octocat", "email": ""}),
		op("add-account", "", at(-43, 0), m{"server": "github.com", "id": "583231", "login": "octocat"})).id
	p.dup = b.create("person", "", "0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e",
		op("create", "", at(-42, 0), m{"name": "ada (pulled twice)", "email": ""}),
		op("add-account", "", at(-42, 0), m{"server": "github.com", "id": "1001", "login": "ada"})).id
	return p
}

const (
	github = "github.com/acme/widgets"
	gitlab = "gitlab.epfl.ch/acme/widgets"
)

// basic is the format's corners, one issue each, on a git board: edits,
// fields and labels, close and reopen, deletes (ADR 0019), people, an
// issue pulled from a forge, unicode, lenient packs; project numbers, two
// forges' sync records, releases on tagged commits; a ref under
// refs/board/issue/ that isn't an id.
func basic(ctx context.Context, dir string) (meta, error) {
	b, err := newBoard(ctx, dir)
	if err != nil {
		return meta{}, err
	}
	first := b.code(at(-50, 0), "Start\n", map[string][]byte{"README.md": []byte("# widgets\n"), ".git-board.toml": []byte(config)})
	p := persons(b)

	// 1: every issue operation, and a close then a reopen
	search := b.create("issue", "", "11111111111111111111111111111111",
		op("create", p.ada, at(0, 0), m{"title": "Search ignores closed issues", "body": "Searching for a closed issue finds nothing."}),
		op("change-labels", p.ada, at(0, 0), m{"add": []string{"status:ready", "prio:p1", "assignee:ada", "bug", "sprint:s1"}, "remove": []string{}}))
	c1 := search.add(op("comment", p.bob, at(1, 0), m{"body": "It finds them with is:closed."})) + ":0"
	c2 := search.add(op("comment", p.chloe, at(1, 30), m{"body": "This line looks wrong.", "meta": map[string]string{"review:path": "core/search.go", "review:line": "42"}})) + ":0"
	search.add(op("edit-text", p.ada, at(2, 0), m{"target": search.id + ":0", "body": "Searching for a closed issue finds nothing, unless the query says is:closed."}))
	search.add(op("edit-text", p.bob, at(2, 10), m{"target": c1, "body": "It finds them with `is:closed`."}))
	search.add(op("edit-text", p.bob, at(2, 20), m{"target": c1, "body": "It finds them with `is:closed`, and `-is:open`."}))
	search.add(op("set-title", p.ada, at(3, 0), m{"title": "Search hides closed issues by default"}))
	search.add(op("change-labels", p.bob, at(3, 30), m{"add": []string{"status:in-progress", "assignee:bobby"}, "remove": []string{"status:ready"}}))
	search.add(op("set-status", p.ada, at(4, 0), m{"status": "closed"}))
	search.add(op("set-status", p.bob, at(5, 0), m{"status": "open"}))
	search.add(op("set-meta", p.ada, at(5, 30), m{"target": c2, "meta": map[string]string{"review:resolved": "true"}}),
		op("set-meta", p.ada, at(5, 30), m{"target": search.id + ":0", "meta": map[string]string{"project:status": "Todo"}}))
	search.add(op("comment", p.ada, at(6, 0), m{"body": "@bobby can you check the inbox counts too?"}),
		op("change-labels", p.ada, at(6, 0), m{"add": []string{"est:3", "prio:p0"}, "remove": []string{"prio:p1", "never-there"}}))

	// 2: closed, with its own forge number at the end of its title
	export := b.create("issue", "", "22222222222222222222222222222222",
		op("create", p.bob, at(7, 0), m{"title": "Export CSV (#13)", "body": "Export the board as CSV."}),
		op("change-labels", p.bob, at(7, 0), m{"add": []string{"prio:p2", "est:0.5", "feature", "status:done"}}))
	export.add(op("comment", p.ada, at(8, 0), m{"body": "Done in 4e5f6a7."}))
	export.add(op("set-status", p.ada, at(8, 1), m{"status": "closed"}))

	// 3: deleted: hidden, its history kept
	spam := b.create("issue", "", "33333333333333333333333333333333",
		op("create", p.bob, at(9, 0), m{"title": "Buy cheap watches", "body": "spam"}))
	spam.add(op("comment", p.bob, at(9, 5), m{"body": "more spam"}))
	spam.add(op("delete", p.ada, at(9, 10), m{"target": spam.id + ":0"}))

	// 4: a deleted comment, edited and deleted again after: nothing changes
	thread := b.create("issue", "", "44444444444444444444444444444444",
		op("create", p.chloe, at(10, 0), m{"title": "Thread with a deleted comment", "body": ""}))
	k1 := thread.add(op("comment", p.ada, at(10, 5), m{"body": "first"})) + ":0"
	k2 := thread.add(op("comment", p.bob, at(10, 6), m{"body": "rude"}),
		op("comment", p.bob, at(10, 6), m{"body": "sorry"}))
	thread.add(op("delete", p.ada, at(10, 10), m{"target": k2 + ":0"}))
	thread.add(op("edit-text", p.bob, at(10, 11), m{"target": k2 + ":0", "body": "edited after its delete"}),
		op("set-meta", p.bob, at(10, 11), m{"target": k2 + ":0", "meta": map[string]string{"x": "y"}}))
	thread.add(op("delete", p.ada, at(10, 12), m{"target": k2 + ":0"}))
	thread.add(op("edit-text", p.ada, at(10, 13), m{"target": k1, "body": "first, edited"}))

	// 5: unicode everywhere
	uni := b.create("issue", "", "55555555555555555555555555555555",
		op("create", p.chloe, at(11, 0), m{
			"title": "Ünïcödé: 日本語のタイトル 🚀 — שלום & <b>",
			"body":  "Combining: é vs é. ZWJ: 👩‍👩‍👧. Tabs:\tend.\r\nCRLF line, trailing spaces   \n\n```go\nfmt.Println(\"<>&\")\n```",
		}),
		op("change-labels", p.chloe, at(11, 0), m{"add": []string{"étiquette", "area:données", "assignee:chloe", "status:backlog"}}))
	uni.add(op("comment", p.ada, at(11, 30), m{"body": "مرحبا بالعالم — RTL text, and a NUL-free \\u0000 escape."}))
	uni.add(op("set-title", p.chloe, at(12, 0), m{"title": "Ünïcödé: 日本語のタイトル 🚀 — שלום & <b> (renamed)"}))

	// 6: pulled from GitHub; one forge comment pulled twice, by two clones:
	// one comment, two ids (data-model.md, Issue state 3)
	pulled := b.create("issue", "", "66666666666666666666666666666666",
		from(op("create", p.octo, at(12, 30), m{"title": "Crash on empty report", "body": "Steps: open an empty report."}), github, "issue:5550007"),
		from(op("change-labels", p.octo, at(12, 30), m{"add": []string{"bug", "status:backlog"}}), github, "issue:5550007"))
	d1 := pulled.add(from(op("comment", p.octo, at(13, 0), m{"body": "Still happens on 0.3."}), github, "comment:9001")) + ":0"
	d2 := pulled.add(from(op("comment", p.octo, at(13, 0), m{"body": "Still happens on 0.3."}), github, "comment:9001"),
		op("comment", p.ada, at(13, 5), m{"body": "Confirmed."})) + ":0"
	pulled.add(op("edit-text", p.ada, at(13, 10), m{"target": d2, "body": "Still happens on 0.3 (edited through the second id)."}))
	_ = d1

	// 7: lenient packs: pretty JSON, unknown fields, keys in another case,
	// a key twice (data-model.md: Go's decoder, a known ceiling)
	lenientFirst := fmt.Sprintf(`{
  "clock": 1,
  "nonce": "77777777777777777777777777777777",
  "x-future": {"any": [1, 2]},
  "ops": [
    {
      "kind": "create",
      "author": %q,
      "time": %d,
      "signature": "ignored",
      "data": {"title": "Lenient packs", "body": "Written by a later git-board.", "priority": "ignored"}
    }
  ]
}
`, p.ada, at(14, 0))
	lenient := b.createRaw("issue", "", []byte(lenientFirst), at(14, 0))
	lenient.addRaw([]byte(fmt.Sprintf(`{"clock":2,"ops":[{"KIND":"comment","Author":%q,"TIME":%d,"Data":{"BODY":"keys in capitals"}}]}`, p.bob, at(14, 5))), at(14, 5))
	lenient.addRaw([]byte(fmt.Sprintf(`{"clock":3,"ops":[{"kind":"set-title","author":%q,"time":%d,"data":{"title":"first of two","title":"Lenient packs: the second title wins"}}]}`, p.ada, at(14, 10))), at(14, 10))
	lenient.addRaw([]byte(fmt.Sprintf(`{"ops":[{"kind":"change-labels","author":%q,"time":%d,"data":{"add":["prio:p3"]}}],"clock":4}`, p.ada, at(14, 15))), at(14, 15))

	// 8: nothing but a title, from GitLab
	bare := b.create("issue", "", "88888888888888888888888888888888",
		from(op("create", p.ada, at(15, 0), m{"title": "Bare", "body": ""}), gitlab, "issue:31"))

	// 9: many labels, a number field, a closed one reopened twice
	labels := b.create("issue", "", "99999999999999999999999999999999",
		op("create", p.ada, at(16, 0), m{"title": "Label churn", "body": "Labels come and go."}),
		op("change-labels", p.ada, at(16, 0), m{"add": []string{"a", "b", "c", "status:ready", "est:8", "assignee:ada", "assignee:chloe", "sprint:s2"}}))
	labels.add(op("change-labels", p.bob, at(16, 30), m{"add": []string{"a", "d"}, "remove": []string{"b", "assignee:ada"}}))
	labels.add(op("set-status", p.bob, at(17, 0), m{"status": "closed"}))
	labels.add(op("set-status", p.bob, at(17, 1), m{"status": "open"}))
	labels.add(op("set-status", p.ada, at(17, 2), m{"status": "closed"}))
	labels.add(op("set-status", p.ada, at(17, 3), m{"status": "open"}))
	labels.add(op("change-labels", p.ada, at(18, 0), m{"add": []string{"status:in-review"}, "remove": []string{"status:ready", "c"}}))

	// project numbers: GB-100 on; the pulled issue keeps its GitHub number,
	// 7, below the start. A line a later git-board may write is skipped.
	numbers := fmt.Sprintf("prefix GB\nstart 100\n100 %s\n101 %s\n\nx-later anything\n102 %s\n103 %s\n", search.id, export.id, uni.id, labels.id)
	seq := b.commit(b.tree(map[string][]byte{"numbers": []byte(numbers)}), at(19, 0))
	b.refs["refs/board/seq"] = seq

	// the sync records of two forges
	gh := b.create("sync", syncer.StateID(github), "",
		op("link", p.ada, at(19, 0), m{"issue": search.id, "forge_id": "8812", "number": "12", "url": "https://github.com/acme/widgets/issues/12"}),
		op("link", p.ada, at(19, 0), m{"issue": export.id, "forge_id": "8813", "number": "13", "url": "https://github.com/acme/widgets/issues/13"}),
		op("link", p.ada, at(19, 0), m{"issue": pulled.id, "forge_id": "5550007", "number": "7", "url": "https://github.com/acme/widgets/issues/7"}),
		op("link", p.ada, at(19, 0), m{"issue": uni.id, "forge_id": "8814", "number": "", "url": "javascript:alert(1)"}),
		op("link", p.ada, at(19, 0), m{"issue": search.id, "forge_id": "9999", "number": "99", "url": "https://github.com/acme/widgets/issues/99"}),
		op("link", p.ada, at(19, 0), m{"issue": strings.Repeat("ab", 20), "forge_id": "8815", "number": "15", "url": "https://github.com/acme/widgets/issues/15"}))
	gh.add(op("seen", p.ada, at(19, 1), m{"issue": search.id, "field": "title", "value": "Search hides closed issues by default"}),
		op("seen", p.ada, at(19, 1), m{"issue": search.id, "field": "labels", "value": "bug\nprio:p0"}),
		op("posted", p.ada, at(19, 1), m{"issue": search.id, "comment": c1, "forge_id": "3318822104"}),
		op("refused", p.ada, at(19, 1), m{"issue": search.id, "field": "assignees", "value": "+bobby", "reason": "no account on github.com"}),
		op("seen", p.ada, at(19, 1), m{"issue": search.id, "field": "milestone", "value": "ignored: not a field"}),
		op("run", p.ada, at(19, 2), m{"started": at(18, 59)}))
	b.create("sync", syncer.StateID(gitlab), "",
		op("link", p.ada, at(20, 0), m{"issue": search.id, "forge_id": "4403", "number": "3", "url": "https://gitlab.epfl.ch/acme/widgets/-/issues/3"}),
		op("link", p.ada, at(20, 0), m{"issue": bare.id, "forge_id": "4431", "number": "31", "url": "https://gitlab.epfl.ch/acme/widgets/-/issues/31"}),
		op("run", p.ada, at(20, 1), m{"started": at(20, 0)}))

	// releases: a final and a candidate, each its tag; one yanked
	second := b.code(at(21, 0), "Search fix\n", map[string][]byte{"README.md": []byte("# widgets\n\nSearch fixed.\n"), ".git-board.toml": []byte(config)})
	b.refs["refs/tags/v0.1.0"] = first
	b.refs["refs/tags/v0.2.0-rc.1"] = second
	r1 := b.create("release", "", "a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1",
		op("create", p.ada, at(21, 10), m{"commit": first.String(), "previous": "", "assets": []m{{"name": "widgets_linux_amd64.tar.gz", "sha256": strings.Repeat("0f", 32)}}}))
	r1.add(op("set-notes", p.ada, at(21, 11), m{"notes": "First release.\n\n- search\n- export"}))
	r1.add(op("comment", p.bob, at(21, 12), m{"message": "Deployed to staging."}))
	r2 := b.create("release", "", "a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2",
		op("create", p.ada, at(21, 20), m{"commit": second.String(), "previous": "v0.1.0", "artifacts": []m{{"image": "widgets", "registry": "ghcr.io/acme", "digest": "sha256:" + strings.Repeat("ab", 32)}}}))
	r2.add(op("yank", p.ada, at(21, 30), m{"reason": "breaks the export"}))

	// a ref under refs/board/issue/ that isn't an id: ignored by readers
	b.refs["refs/board/issue/notes"] = search.head

	if err := b.finish(); err != nil {
		return meta{}, err
	}
	sid, eid, uid, pid := search.id[:7], export.id[:7], uni.id[:7], pulled.id[:7]
	return meta{toml: config, me: p.ada, commands: []string{
		`ls is:closed --json`,
		`ls "is:open,closed" --json`,
		`ls "status:ready,in-review" --json`,
		`ls "-status:in-progress is:open" --json`,
		`ls -- "-status:in-progress is:open" --json`,
		`ls "assignee:chloe" --json`,
		`ls "assignee:@me" --json`,
		`ls "author:bob" --json`,
		`ls "involves:bobby" --json`,
		`ls "label:étiquette" --json`,
		`ls "no:prio" --json`,
		`ls "prio:p0,p2 is:closed" --json`,
		`ls "kind:bug" --json`,
		`ls "title:*search*" --json`,
		`ls closed --json`,
		`ls "\"unless the query\"" --json`,
		`ls "inbox in:comments" --json`,
		`ls "日本語" --json`,
		`ls "#12" --json`,
		`ls "GB-7" --json`,
		`ls "comments:>=2" --json`,
		`ls "est:>1" --json`,
		`ls "created:2025-03-03" --json`,
		`ls "created:>=2025-03-03T12:00 created:<2025-03-04" --json`,
		`ls "updated:2025-03-03..2025-03-03" --json`,
		`ls "closed:2025-03-03" --json`,
		`ls "created:>=@today-1d" --at 2025-03-04 --json`,
		`ls "sprint:@current" --at 2025-03-05 --json`,
		`ls --at 2025-03-03T10:00 --json`,
		`ls --sort "prio asc" --json`,
		`ls "is:open,closed" --sort "updated desc" --limit 3 --json`,
		`ls --json id,title,number,forges`,
		`ls "is:open,closed" --json number --jq ".[].number"`,
		`ls "nosuchfield:x" --json`,
		`ls "created:yesterday" --json`,
		`view ready --json`,
		`show ` + sid + ` --at 2025-03-03T03:15 --json`,
		`show ` + sid + ` --at 2025-03-03T11:15 --json`,
		`show GB-100 --json`,
		`show "#7" --json`,
		`show ` + pid + ` --no-comments --json`,
		`show ` + uid + ` --md`,
		`show 0000000 --json`,
		`log ` + sid + ` --field status --json`,
		`log ` + sid + ` --since 2025-03-03T04:00 --json`,
		`log ` + sid + ` --since 2025-03-04 --json`,
		`log ` + eid + ` --field prio --json`,
		`labels --json`,
		`me --ls --json`,
	}}, nil
}
