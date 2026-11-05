package fake_test

import (
	"testing"

	"github.com/guilbep/git-board/next/forge"
	"github.com/guilbep/git-board/next/forge/fake"
	"github.com/guilbep/git-board/next/forge/forgetest"
)

// The fake is its own adapter.
func TestConformance(t *testing.T) {
	t.Parallel()
	forgetest.Run(t, func(_ *testing.T, f *fake.Forge) forge.Forge { return f })
}
