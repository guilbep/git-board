package watch

import (
	"testing"

	"github.com/guilbep/git-board/core/internal/hermetic"
)

// TestMain keeps the tests off the network and the user's tokens.
func TestMain(m *testing.M) { hermetic.Run(m) }
