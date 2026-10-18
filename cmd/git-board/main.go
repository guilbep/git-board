// Command git-board is fields, sprints and views over git-bug issues.
// Field writes go through git-bug's cache API in-process — no `git-bug`
// binary required on PATH, just a normal git-bug-initialized repo.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/git-bug/git-bug/repository"

	"github.com/guilbep/git-board/internal/server"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", explainLock(err))
		os.Exit(1)
	}
}

// explainLock tells, when git-bug's lock refuses the repository, that
// this repository's git-board server holds it, and where.
// ponytail: matches git-bug's message, which has no error type to test.
func explainLock(err error) error {
	gitDir, gerr := repository.FindGitDir(".")
	if gerr != nil {
		return err
	}
	inst, ok := server.ReadInstance(gitDir)
	if !ok || !strings.Contains(err.Error(), fmt.Sprintf("locked by the process pid %d", inst.PID)) {
		return err
	}
	return fmt.Errorf("%w\na git-board server is running for this repository at %s (pid %d): use the web UI there, or stop it", err, inst.URL, inst.PID)
}
