package github

import (
	"testing"

	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/fake"
	"github.com/guilbep/git-board/forge/forgetest"
)

// The suite, over the fake served as GitHub.
func TestConformance(t *testing.T) {
	t.Parallel()
	forgetest.Run(t, func(t *testing.T, f *fake.Forge) forge.Forge { return newHub(t, f).adapter(t) })
}
