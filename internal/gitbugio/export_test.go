package gitbugio

import "github.com/guilbep/git-board/core/gitstore"

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
