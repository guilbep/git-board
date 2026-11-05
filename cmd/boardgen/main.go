// Command boardgen makes a synthetic board, for measuring git-board on
// a board of thousands of issues (docs/guide/performance.md):
//
//	go run ./cmd/boardgen -n 5000 /tmp/board5k
//
// It makes a git repository at the path, with a .git-board.toml and the
// board in git, or, with -pg, the board in a PostgreSQL database (a
// postgres:// URL, set as the repository's board.datastore).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/guilbep/git-board/core/datastore"
	"github.com/guilbep/git-board/core/datastore/memstore"
	"github.com/guilbep/git-board/core/issue/boardgen"
	"github.com/guilbep/git-board/core/pgstore"
)

func main() {
	n := flag.Int("n", 5000, "issues")
	persons := flag.Int("persons", 25, "persons")
	seed := flag.Uint64("seed", 1, "the random choices' seed")
	pg := flag.String("pg", "", "a postgres:// URL to keep the board in, instead of git")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: boardgen [-n issues] [-persons n] [-seed s] [-pg url] <new repository path>")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(flag.Arg(0), *pg, boardgen.Options{Issues: *n, Persons: *persons, Seed: *seed}); err != nil {
		fmt.Fprintln(os.Stderr, "boardgen:", err)
		os.Exit(1)
	}
}

func run(dir, pg string, o boardgen.Options) error {
	ctx := context.Background()
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("%s exists", dir)
	}
	start := time.Now()
	if pg == "" {
		b, err := boardgen.Git(ctx, dir, o)
		if err != nil {
			return err
		}
		fmt.Printf("%s: %d issues, %d persons in git, in %s\n", dir, len(b.Issues), len(b.Persons), time.Since(start).Round(time.Millisecond))
		return nil
	}
	if err := boardgen.Init(ctx, dir); err != nil {
		return err
	}
	folder := filepath.Join(dir, ".git", "board")
	mem := memstore.New(folder)
	b, err := boardgen.Make(ctx, mem, o)
	if err != nil {
		return err
	}
	p, err := pgstore.Open(ctx, pg, folder)
	if err != nil {
		return err
	}
	defer p.Close()
	if _, err := datastore.Copy(ctx, mem, p); err != nil {
		return err
	}
	for _, kv := range [][2]string{{"board.datastore", pg}, {"board.me", b.Persons[0]}} {
		if out, err := exec.Command("git", "-C", dir, "config", "--local", kv[0], kv[1]).CombinedOutput(); err != nil {
			return fmt.Errorf("git config: %w: %s", err, out)
		}
	}
	fmt.Printf("%s: %d issues, %d persons in PostgreSQL, in %s\n", dir, len(b.Issues), len(b.Persons), time.Since(start).Round(time.Millisecond))
	return nil
}
