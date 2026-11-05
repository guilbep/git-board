package mem

import (
	"testing"

	"github.com/guilbep/git-board/next/store"
	"github.com/guilbep/git-board/next/store/storetest"
)

func TestConformance(t *testing.T) {
	t.Parallel()
	storetest.Run(t, storetest.Options{Open: func(t *testing.T) store.Datastore { return New(t.TempDir()) }})
}
