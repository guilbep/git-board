package fake_test

import (
	"testing"

	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/fake"
	"github.com/guilbep/git-board/forge/forgetest"
)

// The fake is its own adapter.
func TestConformance(t *testing.T) {
	t.Parallel()
	forgetest.Run(t, func(_ *testing.T, f *fake.Forge) forge.Forge { return f })
}
