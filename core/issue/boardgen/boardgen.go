// Package boardgen makes a synthetic board for benchmarks: people, and
// issues with labels, a body, comments and a few edits, written through
// the core (issue.Create, issue.Do) into any datastore. The same options
// make the same board, ids aside (a create's nonce is random).
//
// Git writes each pack with a process for its ref: Git writes the board
// in memory first, then into a repository with one update-ref for all.
package boardgen

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/guilbep/git-board/core/datastore"
	"github.com/guilbep/git-board/core/datastore/memstore"
	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/core/people"
)

// Options say what to make. Zero fields take the defaults.
type Options struct {
	Issues  int    // 1,000 by default
	Persons int    // 25 by default
	Seed    uint64 // the random choices'
	// Start is the first issue's time; the others follow, an hour or so
	// apart. 2024-01-01 by default.
	Start time.Time
}

func (o Options) withDefaults() Options {
	if o.Issues == 0 {
		o.Issues = 1000
	}
	if o.Persons == 0 {
		o.Persons = 25
	}
	if o.Start.IsZero() {
		o.Start = time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	}
	return o
}

// Board is what Make wrote: the persons' and the issues' ids, and the
// logins, person by person.
type Board struct {
	Persons, Issues []string
	Logins          []string
}

// The labels, as internal/config/testdata/git-board.toml's fields have them.
var (
	statuses = []string{"backlog", "comite", "ready", "in-progress", "in-review", "ready-for-validation", "qa", "tests-metier", "validated"}
	prios    = []string{"p0", "p1", "p2", "p3"}
	ests     = []string{"0.5", "1", "2", "3", "4", "6", "8", "16"}
	cats     = []string{"bug", "feature", "chore"}
	sprints  = []string{"delivery", "post-delivery"}
	areas    = []string{"area:backend", "area:frontend", "area:infra", "area:docs", "area:data"}
	words    = strings.Fields(`the export invoice crashes when report has no lines user page loads
		slowly after login backend returns error on save button form field shows wrong value for
		date unit total emissions factor travel building energy purchase calculation module table
		chart filter sort column missing translation french english should could would we need to
		check fix update refactor test deploy staging production database migration query index
		cache api endpoint response request timeout retry token session role permission admin
		laboratory unit year scope upload csv file import validation message spinner modal dialog`)
)

// Make writes a board into ds through the core: o.Persons persons with a
// GitHub account each, then o.Issues issues. Each issue is a create with
// its labels, then each of: zero to six comments, an edit of the body or
// a comment, a change of status, sometimes a new title, a close for a
// third of them, each its own pack.
func Make(ctx context.Context, ds datastore.Datastore, o Options) (Board, error) {
	o = o.withDefaults()
	r := rand.New(rand.NewPCG(o.Seed, 0x9e3779b97f4a7c15))
	var b Board
	for i := range o.Persons {
		login := fmt.Sprintf("dev%d", i)
		at := o.Start.Add(-time.Hour).Unix()
		ops := []oplog.Op{
			people.NewCreate("", fmt.Sprintf("Developer %d", i), login+"@example.com"),
			people.AddAccount("", people.Account{Server: "github.com", ID: fmt.Sprint(1000 + i), Login: login}),
		}
		for j := range ops {
			ops[j].Time = at
		}
		p, err := people.Create(ctx, ds, ops...)
		if err != nil {
			return b, err
		}
		b.Persons, b.Logins = append(b.Persons, p.ID), append(b.Logins, login)
	}
	t := o.Start
	for i := range o.Issues {
		t = t.Add(time.Duration(30+r.IntN(60)) * time.Minute)
		id, err := makeIssue(ctx, ds, r, b, i, t)
		if err != nil {
			return b, fmt.Errorf("boardgen: issue %d: %w", i, err)
		}
		b.Issues = append(b.Issues, id)
	}
	return b, nil
}

// makeIssue writes issue i, created at t, and its later packs.
func makeIssue(ctx context.Context, ds datastore.Datastore, r *rand.Rand, b Board, i int, t time.Time) (string, error) {
	who := func() string { return b.Persons[r.IntN(len(b.Persons))] }
	at := t.Unix()
	stamp := func(op oplog.Op) oplog.Op {
		at += int64(60 + r.IntN(3*86400))
		op.Time = at
		return op
	}
	author := who()
	labels := []string{
		"status:" + statuses[r.IntN(len(statuses))], "prio:" + prios[r.IntN(len(prios))],
		"cat:" + cats[r.IntN(len(cats))], "assignee:" + b.Logins[r.IntN(len(b.Logins))],
		areas[r.IntN(len(areas))],
	}
	if r.IntN(2) == 0 {
		labels = append(labels, "est:"+ests[r.IntN(len(ests))])
	}
	if r.IntN(3) == 0 {
		labels = append(labels, "sprint:"+sprints[r.IntN(len(sprints))])
	}
	create := issue.NewCreate(author, title(r, i), text(r, 2+r.IntN(4)))
	create.Time = at
	tag := issue.ChangeLabels(author, labels, nil)
	tag.Time = at
	is, err := issue.Create(ctx, ds, create, tag)
	if err != nil {
		return "", err
	}
	l, err := oplog.LoadContext(ctx, ds, "issue", is.ID)
	if err != nil {
		return "", err
	}
	write := func(op oplog.Op) error { return l.Append(ctx, ds, []oplog.Op{stamp(op)}) }
	for range r.IntN(7) {
		if err := write(issue.NewComment(who(), text(r, 1+r.IntN(3)))); err != nil {
			return "", err
		}
	}
	target := is.BodyOp
	if n := len(l.Entries); n > 2 && r.IntN(2) == 0 {
		target = l.Entries[2+r.IntN(n-2)].ID // a comment
	}
	if err := write(issue.EditText(who(), target, text(r, 2+r.IntN(3)))); err != nil {
		return "", err
	}
	moved := "status:" + statuses[r.IntN(len(statuses))]
	if moved == labels[0] {
		moved = "status:" + statuses[0]
		if moved == labels[0] {
			moved = "status:" + statuses[1]
		}
	}
	if err := write(issue.ChangeLabels(who(), []string{moved}, []string{labels[0]})); err != nil {
		return "", err
	}
	if r.IntN(4) == 0 {
		if err := write(issue.SetTitle(who(), title(r, i))); err != nil {
			return "", err
		}
	}
	if r.IntN(3) == 0 {
		if err := write(issue.SetStatus(who(), issue.Closed)); err != nil {
			return "", err
		}
	}
	return is.ID, nil
}

func title(r *rand.Rand, i int) string {
	return fmt.Sprintf("%s (%d)", sentence(r, 4+r.IntN(6)), i)
}

// text is paragraphs of a few sentences.
func text(r *rand.Rand, paragraphs int) string {
	ps := make([]string, paragraphs)
	for i := range ps {
		ss := make([]string, 2+r.IntN(4))
		for j := range ss {
			ss[j] = sentence(r, 6+r.IntN(14)) + "."
		}
		ps[i] = strings.Join(ss, " ")
	}
	return strings.Join(ps, "\n\n")
}

func sentence(r *rand.Rand, n int) string {
	ws := make([]string, n)
	for i := range ws {
		ws[i] = words[r.IntN(len(words))]
	}
	ws[0] = strings.ToUpper(ws[0][:1]) + ws[0][1:]
	return strings.Join(ws, " ")
}

// Config is a .git-board.toml for the board: internal/config/testdata's
// fields, which the labels use.
const Config = `[fields.status]
single = true
options = ["backlog", "comite", "ready", "in-progress", "in-review", "ready-for-validation", "qa", "tests-metier", "validated"]

[fields.prio]
single = true
options = ["p0", "p1", "p2", "p3"]

[fields.est]
single = true
options = ["0.5", "1", "2", "3", "4", "6", "8", "16"]
numeric = true

[fields.assignee]
single = false

[fields.cat]
single = true
options = ["bug", "feature", "chore"]

[sprints]
length_days = 14
start_weekday = "monday"

[[sprints.list]]
slug = "delivery"
title = "Delivery"
start = 2026-08-24

[[sprints.list]]
slug = "post-delivery"
title = "Post-delivery"
start = 2026-09-07
`

// Init makes a git repository at dir, with Config as its .git-board.toml.
func Init(ctx context.Context, dir string) error {
	if out, err := exec.CommandContext(ctx, "git", "init", "-q", dir).CombinedOutput(); err != nil {
		return fmt.Errorf("boardgen: git init: %w: %s", err, out)
	}
	return os.WriteFile(filepath.Join(dir, ".git-board.toml"), []byte(Config), 0o644)
}

// Git makes the board in a new git repository at dir (Init), as Make, and sets who
// writes there (board.me) to its first person. The packs are written as
// gitstore writes them, each in a commit on the one before; the refs move
// in one update-ref, and the repository is packed, refs and objects, as a
// clone's are.
func Git(ctx context.Context, dir string, o Options) (Board, error) {
	if err := Init(ctx, dir); err != nil {
		return Board{}, err
	}
	mem := memstore.New(dir)
	b, err := Make(ctx, mem, o)
	if err != nil {
		return b, err
	}
	s, err := gitstore.Open(dir)
	if err != nil {
		return b, err
	}
	var refs bytes.Buffer
	for _, kind := range []string{"person", "issue"} {
		heads, err := mem.Heads(ctx, kind)
		if err != nil {
			return b, err
		}
		for _, id := range slices.Sorted(maps.Keys(heads)) {
			packs, _, err := mem.Packs(ctx, kind, id)
			if err != nil {
				return b, err
			}
			head, err := chain(s, packs)
			if err != nil {
				return b, err
			}
			fmt.Fprintf(&refs, "create %s %s\n", gitstore.EntityRef(kind, id), head)
		}
	}
	steps := [][]string{{"update-ref", "--stdin"}, {"pack-refs", "--all"}, {"repack", "-adq"}, {"prune-packed"},
		{"config", "--local", "board.me", b.Persons[0]}}
	for _, args := range steps {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
		if args[0] == "update-ref" {
			cmd.Stdin = &refs
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			return b, fmt.Errorf("boardgen: git %s: %w: %s", args[0], err, out)
		}
	}
	return b, nil
}

// chain writes packs, in clock order, each in a commit on the one before,
// and answers the last.
func chain(s *gitstore.Store, packs [][]byte) (gitstore.Hash, error) {
	slices.SortFunc(packs, func(a, b []byte) int { return cmp.Compare(clockOf(a), clockOf(b)) })
	head := gitstore.ZeroHash
	for _, p := range packs {
		var parents []gitstore.Hash
		if head != gitstore.ZeroHash {
			parents = []gitstore.Hash{head}
		}
		c, _, err := s.WritePack(p, parents...)
		if err != nil {
			return head, err
		}
		head = c
	}
	return head, nil
}

// BenchEnv names the repository benchmarks run on: one Git made, with
// cmd/boardgen (docs/guide/performance.md).
const BenchEnv = "GIT_BOARD_BENCH_REPO"

var bench struct {
	once sync.Once
	dir  string
	err  error
}

// BenchRepo is the repository benchmarks run on: $GIT_BOARD_BENCH_REPO,
// or else a board of 1,000 issues Git makes once per process, in a
// temporary directory it leaves behind.
func BenchRepo() (string, error) {
	if dir := os.Getenv(BenchEnv); dir != "" {
		return dir, nil
	}
	bench.once.Do(func() {
		if bench.dir, bench.err = os.MkdirTemp("", "boardgen-"); bench.err == nil {
			_, bench.err = Git(context.Background(), bench.dir, Options{Issues: 1000})
		}
	})
	return bench.dir, bench.err
}

// clockOf is a pack's clock.
func clockOf(pack []byte) uint64 {
	var hd struct {
		Clock uint64 `json:"clock"`
	}
	_ = json.Unmarshal(pack, &hd)
	return hd.Clock
}
