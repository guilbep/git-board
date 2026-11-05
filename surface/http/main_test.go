package httpapi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/guilbep/git-board/hermetic"
)

func TestMain(m *testing.M) {
	hermetic.Run(m, func() {
		if guard.dir != "" {
			os.RemoveAll(filepath.Dir(guard.dir))
		}
	})
}
