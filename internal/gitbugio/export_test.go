package gitbugio

import (
	"fmt"

	"github.com/guilbep/git-board/core/gitstore"
)

// EditRelease is editRelease for the tests: an operation a later
// git-board might add.
func EditRelease(b *Backend, id, kind string) error { return b.editRelease(id, kind, releaseData{}) }

// NumberOnce is numberOnce for the tests: one try, on the counter as fetched.
var NumberOnce = (*Backend).numberOnce

// WriteCounter makes refs/board/seq hold data, as a later or a broken
// git-board might have written it.
func WriteCounter(b *Backend, data []byte) error {
	blob, err := b.s.WriteBlob(data)
	if err != nil {
		return err
	}
	tree, err := b.s.WriteTree([]gitstore.TreeEntry{{Name: seqFile, Blob: blob}})
	if err != nil {
		return err
	}
	commit, err := b.s.WriteCommit(gitstore.Commit{Tree: tree})
	if err != nil {
		return err
	}
	return b.setSeq(commit)
}

// Compare is compare for the tests.
var Compare = (*Backend).compare

// StoreCommit stores a commit holding name on parents, for the tests.
func StoreCommit(b *Backend, name string, parents ...gitstore.Hash) (gitstore.Hash, error) {
	blob, err := b.s.WriteBlob([]byte(name))
	if err != nil {
		return gitstore.ZeroHash, err
	}
	tree, err := b.s.WriteTree([]gitstore.TreeEntry{{Name: name, Blob: blob}})
	if err != nil {
		return gitstore.ZeroHash, err
	}
	return b.s.WriteCommit(gitstore.Commit{Tree: tree, Parents: parents})
}

// WriteForkRelease writes a release record as the git-bug fork kept it:
// a chain of commits under refs/releases/<name>, one per ops file, with
// their edit clocks.
func WriteForkRelease(b *Backend, name string, ops ...string) error {
	var parents []gitstore.Hash
	for i, o := range ops {
		blob, err := b.s.WriteBlob([]byte(o))
		if err != nil {
			return err
		}
		empty, err := b.s.WriteBlob(nil)
		if err != nil {
			return err
		}
		tree, err := b.s.WriteTree([]gitstore.TreeEntry{{Name: fmt.Sprintf("edit-clock-%d", i+1), Blob: empty}, {Name: "ops", Blob: blob}, {Name: "version-1", Blob: empty}})
		if err != nil {
			return err
		}
		head, err := b.s.WriteCommit(gitstore.Commit{Tree: tree, Parents: parents})
		if err != nil {
			return err
		}
		parents = []gitstore.Hash{head}
	}
	return b.s.UpdateRef(ctx, "refs/releases/"+name, parents[0], gitstore.ZeroHash)
}

// CommitCounter commits c on refs/board/seq's head, unpushed, as switch
// --apply's re-key does.
func CommitCounter(b *Backend, c Counter) error {
	head, err := b.s.Ref(seqRef)
	if err != nil {
		return err
	}
	_, err = commitCounter(b.s, c, head)
	return err
}

// Places is ForkBug.places for the tests.
func Places(bug ForkBug, forges []NamedForge, paths map[string]string) ([]string, bool, string) {
	return bug.places(forges, paths)
}

// Scratch is scratch for the tests.
var Scratch = (*Backend).scratch
