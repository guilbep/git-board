// Package boardgen makes a board of many issues in a new git repository,
// in a few seconds and the same for a seed: the board the complexity
// guards count on (docs/guide/performance.md, v2). It writes the packs as
// a command does, each change its own, straight into one `git
// fast-import`, which packs them as a clone has them; no kernel, no view.
// cmd/boardgen runs it for the timed budgets' boards (make perf).
package boardgen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/guilbep/git-board/kernel"
	"github.com/guilbep/git-board/kernel/pack"
	bsync "github.com/guilbep/git-board/sync"
)

// Options size the board.
type Options struct {
	Issues int
	People int    // 25 when 0
	Seed   uint64 // the same seed, the same board
	// Forge gives the board a forge, GitHub's acme/board, as a
	// [forges.github] table (ForgeConfig): each read of the board's state
	// looks for its token, as on a team's clone. A table, not a remote: a
	// forge found in the remotes is resolved on the forge, a call out; a
	// table's project is the file's, and nothing is asked of GitHub until
	// a sync. Without it, no forge path runs.
	Forge bool
	// Synced links every issue to an issue of acme/board on GitHub, in
	// the sync entity a sync leaves (SyncedKey): each read of an issue's
	// forges reads its records. Written after the issues, so the issues
	// are the seed's either way.
	Synced bool
}

// SyncedKey is the forge project Options.Synced links the issues to.
const SyncedKey = "github.com/acme/board"

// ForgeConfig is the forge Options.Forge adds to Config.
const ForgeConfig = `
[forges.github]
kind = "github"
project = "acme/board"
sync = true
`

// Config is the board's .git-board.toml, the cycle on.
const Config = `[fields.status]
single = true
options = ["backlog", "ready", "in-progress", "in-review", "qa", "validated"]

[fields.prio]
single = true
options = ["p0", "p1", "p2", "p3"]

[fields.assignee]
single = false

[experimental]
features = ["cycle"]
`

// Board is what Git made: the people's ids, the first one who writes
// (board.me), and the issues' ids, in the order made.
type Board struct {
	Dir    string
	People []kernel.ID
	Issues []kernel.ID
}

var words = strings.Fields(`export crash login table report save sync page filter label sprint
	board issue comment status field user admin file upload chart lane cache view list
	slow fast broken missing wrong empty double`)

// Git makes the board in a new repository at dir.
func Git(ctx context.Context, dir string, o Options) (*Board, error) {
	if o.People == 0 {
		o.People = 25
	}
	rnd := rand.New(rand.NewPCG(o.Seed, o.Seed^0x9e3779b97f4a7c15))
	at := int64(1_700_000_000)
	tick := func() int64 { at += 60 + int64(rnd.IntN(3600)); return at }
	op := func(kind string, author kernel.ID, d any) kernel.Op {
		raw, _ := json.Marshal(d)
		return kernel.Op{Kind: kind, Author: string(author), Time: tick(), Data: raw}
	}
	sentence := func(n int) string {
		w := make([]string, n)
		for i := range w {
			w[i] = words[rnd.IntN(len(words))]
		}
		return strings.Join(w, " ")
	}

	var stream bytes.Buffer
	mark := 0
	b := &Board{Dir: dir}
	// entity writes one entity, a pack per change, and answers its id:
	// given, or its first pack's hash
	entity := func(kind string, id kernel.ID, changes [][]kernel.Op) (kernel.ID, error) {
		prev := 0
		for i, ops := range changes {
			data, err := pack.Encode(pack.Pack{Clock: uint64(i + 1), Ops: ops})
			if err != nil {
				return "", err
			}
			if i == 0 && id == "" {
				id = kernel.ID(pack.Hash(data))
			}
			mark++
			fmt.Fprintf(&stream, "blob\nmark :%d\ndata %d\n%s\n", mark, len(data), data)
			blob := mark
			mark++
			fmt.Fprintf(&stream, "commit refs/board/%s/%s\nmark :%d\ncommitter <> %d +0000\ndata 0\n", kind, id, mark, ops[0].Time)
			if prev > 0 {
				fmt.Fprintf(&stream, "from :%d\n", prev)
			}
			fmt.Fprintf(&stream, "M 100644 :%d pack\n\n", blob)
			prev = mark
		}
		return id, nil
	}

	for i := range o.People {
		login := fmt.Sprintf("dev%d", i)
		id, err := entity("person", "", [][]kernel.Op{{
			op("create", "", map[string]string{"name": "Dev " + login, "email": login + "@example.com"}),
			op("add-account", "", map[string]string{"server": "github.com", "id": fmt.Sprint(1000 + i), "login": login}),
		}})
		if err != nil {
			return nil, err
		}
		b.People = append(b.People, id)
	}
	statuses := []string{"backlog", "ready", "in-progress", "in-review", "qa", "validated"}
	var seen [][2]string // each issue's title and status, for Synced
	for range o.Issues {
		who := func() kernel.ID { return b.People[rnd.IntN(len(b.People))] }
		author := who()
		body := sentence(20) + ".\n\n" + sentence(30) + ".\n"
		if rnd.IntN(3) == 0 {
			body += "\n## Acceptance criteria\n\n- [ ] " + sentence(5) + "\n- [ ] " + sentence(6) + "\n"
		}
		title := sentence(3 + rnd.IntN(5))
		status := "open"
		changes := [][]kernel.Op{{op("create", author, map[string]string{"title": title, "body": body})}}
		changes = append(changes, []kernel.Op{op("change-labels", author, map[string]any{"add": []string{
			"status:" + statuses[rnd.IntN(len(statuses))], fmt.Sprintf("prio:p%d", rnd.IntN(4)),
			fmt.Sprintf("assignee:dev%d", rnd.IntN(o.People))}, "remove": []string{}})})
		for range rnd.IntN(5) {
			text := sentence(10 + rnd.IntN(20))
			if rnd.IntN(4) == 0 {
				text += "?"
			}
			changes = append(changes, []kernel.Op{op("comment", who(), map[string]any{"body": text})})
		}
		if rnd.IntN(3) == 0 {
			changes = append(changes, []kernel.Op{op("set-status", author, map[string]string{"status": "closed"})})
			status = "closed"
		}
		id, err := entity("issue", "", changes)
		if err != nil {
			return nil, err
		}
		b.Issues = append(b.Issues, id)
		seen = append(seen, [2]string{title, status})
	}
	if o.Synced { // after the issues: the seed's random choices stay theirs
		var ops []kernel.Op
		me := b.People[0]
		for i, id := range b.Issues {
			u := fmt.Sprintf("https://github.com/acme/board/issues/%d", i+1)
			ops = append(ops,
				op("link", me, map[string]string{"issue": string(id), "forge_id": u, "number": fmt.Sprint(i + 1), "url": u}),
				op("seen", me, map[string]string{"issue": string(id), "field": "title", "value": seen[i][0]}),
				op("seen", me, map[string]string{"issue": string(id), "field": "status", "value": seen[i][1]}))
		}
		if _, err := entity("sync", bsync.StateID(SyncedKey), [][]kernel.Op{ops}); err != nil {
			return nil, err
		}
	}
	stream.WriteString("done\n")

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	config := Config
	if o.Forge {
		config += ForgeConfig
	}
	if err := os.WriteFile(filepath.Join(dir, ".git-board.toml"), []byte(config), 0o644); err != nil {
		return nil, err
	}
	setup := [][]string{
		{"init", "-q"},
		{"config", "board.me", string(b.People[0])},
	}
	for _, args := range setup {
		if err := run(ctx, dir, nil, args...); err != nil {
			return nil, err
		}
	}
	if err := run(ctx, dir, &stream, "fast-import", "--quiet", "--done"); err != nil {
		return nil, err
	}
	if err := run(ctx, dir, nil, "pack-refs", "--all"); err != nil {
		return nil, err
	}
	return b, nil
}

func run(ctx context.Context, dir string, stdin *bytes.Buffer, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	if stdin != nil {
		cmd.Stdin = stdin
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %w: %s", args[0], err, bytes.TrimSpace(out))
	}
	return nil
}
