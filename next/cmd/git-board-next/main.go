// Command git-board-next is git-board v2 (docs/rewrite/design.md), grown
// in next/ beside v1 until it replaces it: `git board-next ls` reads the
// same board as `git board ls`.
package main

import (
	"context"
	"os"
	"runtime/pprof"

	"github.com/guilbep/git-board/next/surface/cli"
)

func main() {
	if p := os.Getenv("GIT_BOARD_CPUPROFILE"); p != "" { // a profile of one command, for docs/guide/performance.md
		f, err := os.Create(p)
		if err == nil && pprof.StartCPUProfile(f) == nil {
			code := cli.Main(context.Background(), os.Args[1:], os.Stdout, os.Stderr)
			pprof.StopCPUProfile()
			f.Close()
			os.Exit(code)
		}
	}
	os.Exit(cli.Main(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}
