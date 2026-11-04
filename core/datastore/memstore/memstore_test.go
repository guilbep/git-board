package memstore_test

import (
	"testing"

	"github.com/guilbep/git-board/core/datastore"
	"github.com/guilbep/git-board/core/datastore/dstest"
	"github.com/guilbep/git-board/core/datastore/memstore"
)

func TestDatastore(t *testing.T) {
	t.Parallel()
	dstest.Run(t, func(t *testing.T) datastore.Datastore { return memstore.New(t.TempDir()) })
}
