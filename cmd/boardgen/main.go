// Command boardgen makes a synthetic board, for measuring git-board on a
// board of thousands of issues (docs/guide/performance.md; make perf):
//
//	go run ./cmd/boardgen -n 5000 /tmp/board5k
//
// It makes a git repository at the path, with a .git-board.toml (the
// cycle on), board.me and the board in refs/board/ (tools/boardgen). For
// a board in PostgreSQL, copy it there: set board.datastore and run git
// board datastore copy.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/guilbep/git-board/tools/boardgen"
)

func main() {
	n := flag.Int("n", 5000, "issues")
	people := flag.Int("people", 25, "people")
	seed := flag.Uint64("seed", 1, "the random choices' seed: the same seed, the same board")
	withForge := flag.Bool("forge", false, "a forge, GitHub's acme/board, as a [forges] table: its token looked for, nothing asked of GitHub")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: boardgen [-n issues] [-people n] [-seed s] <new repository path>")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	dir := flag.Arg(0)
	if _, err := os.Stat(dir); err == nil {
		fmt.Fprintf(os.Stderr, "boardgen: %s exists\n", dir)
		os.Exit(1)
	}
	start := time.Now()
	b, err := boardgen.Git(context.Background(), dir, boardgen.Options{Issues: *n, People: *people, Seed: *seed, Forge: *withForge})
	if err != nil {
		fmt.Fprintln(os.Stderr, "boardgen:", err)
		os.Exit(1)
	}
	fmt.Printf("%s: %d issues, %d people in git, in %s\n", dir, len(b.Issues), len(b.People), time.Since(start).Round(time.Millisecond))
}
