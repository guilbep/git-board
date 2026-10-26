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

	if _, err := b.NewIdentity("smoke", "smoke@example.com"); err != nil {
		fmt.Fprintln(os.Stderr, "identity:", err)
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
		id, err := b.NewIssue(title, "", []string{"prio:p1", "status:ready"})
		if err != nil {
			fmt.Fprintln(os.Stderr, "new issue:", err)
			os.Exit(1)
		}
		if i == len(titles)-1 {
			if err := b.CloseBug(id); err != nil {
				fmt.Fprintln(os.Stderr, "close:", err)
				os.Exit(1)
			}
		}
	}
	fmt.Println("seeded", len(titles), "issues")
}
