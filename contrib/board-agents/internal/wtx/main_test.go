package wtx_test

import (
	"testing"

	"github.com/guilbep/board-agents/internal/hermetic"
)

// TestMain keeps the tests off the network and the user's tokens.
func TestMain(m *testing.M) { hermetic.Run(m) }
