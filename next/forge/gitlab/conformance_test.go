package gitlab

import (
	"testing"

	"github.com/guilbep/git-board/next/forge"
	"github.com/guilbep/git-board/next/forge/fake"
	"github.com/guilbep/git-board/next/forge/forgetest"
)

// The suite, over the fake served as GitLab, two items a page.
func TestConformance(t *testing.T) {
	t.Parallel()
	forgetest.Run(t, func(t *testing.T, f *fake.Forge) forge.Forge {
		_, a := serve(t, f, "p")
		return a
	})
}
