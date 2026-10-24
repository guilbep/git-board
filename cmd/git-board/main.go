// Command git-board is fields, sprints and views over git-bug issues.
// Field writes go through git-bug's cache API in-process — no `git-bug`
// binary required on PATH, just a normal git-bug-initialized repo.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
