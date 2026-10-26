package fake_test

import (
	"testing"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/conformance"
	"github.com/guilbep/git-board/core/forge/fake"
)

// The fake is its own adapter.
func TestConformance(t *testing.T) {
	t.Parallel()
	conformance.Run(t, func(_ *testing.T, f *fake.Forge) forge.Forge { return f })
}
