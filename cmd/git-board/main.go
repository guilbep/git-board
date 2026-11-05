// Command git-board is git-board: `git board`, as git runs it, its issues
// kept in git (README.md). The CLI is surface/cli; the layers below it are
// in doc.go.
package main

import (
	"context"
	"os"
	"runtime/pprof"

	"github.com/guilbep/git-board/surface/cli"
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
