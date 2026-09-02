package main

import (
	"fmt"
	"os"

	"github.com/guilbep/git-board/internal/gitbugio"
)

func main() {
	b, err := gitbugio.Open("/tmp/smoke-repo")
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		os.Exit(1)
	}
	defer b.Close()

	c := b.TestCache()
	id, err := c.Identities().NewFromGitUser()
	if err != nil {
		fmt.Fprintln(os.Stderr, "identity:", err)
		os.Exit(1)
	}
	if err := c.SetUserIdentity(id); err != nil {
		fmt.Fprintln(os.Stderr, "setuser:", err)
		os.Exit(1)
	}

	titles := []string{
		"Fix login bug on Safari",
		"Add dark mode toggle",
		"Improve TUI scrolling performance",
		"Search does not match labels",
		"Refactor filter grammar parser",
		"Write docs for sprint aliases",
		"Investigate flaky CI test",
		"Support numeric sort for est field",
		"Add keyboard shortcut for quick-set",
		"Bump git-bug dependency to v0.11",
		"Long title to check wrapping behavior in the list item description area",
		"Closed issue example",
	}
	for i, title := range titles {
		bug, _, err := c.Bugs().New(title, "")
		if err != nil {
			fmt.Fprintln(os.Stderr, "new bug:", err)
			os.Exit(1)
		}
		if _, _, err := bug.ChangeLabels([]string{"prio:p1", "status:ready"}, nil); err != nil {
			fmt.Fprintln(os.Stderr, "labels:", err)
			os.Exit(1)
		}
		if err := bug.Commit(); err != nil {
			fmt.Fprintln(os.Stderr, "commit:", err)
			os.Exit(1)
		}
		if i == len(titles)-1 {
			if _, err := bug.Close(); err != nil {
				fmt.Fprintln(os.Stderr, "close:", err)
				os.Exit(1)
			}
			if err := bug.Commit(); err != nil {
				fmt.Fprintln(os.Stderr, "commit close:", err)
				os.Exit(1)
			}
		}
	}
	fmt.Println("seeded", len(titles), "issues")
}
