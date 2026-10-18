package gitbugio

import "github.com/git-bug/git-bug/repository"

// NumberOnce is numberOnce for the tests: one try, on the counter as fetched.
var NumberOnce = (*Backend).numberOnce

// WriteCounter makes refs/board/seq hold data, as a later or a broken
// git-board might have written it.
func WriteCounter(b *Backend, data []byte) error {
	blob, err := b.repo.StoreData(data)
	if err != nil {
		return err
	}
	tree, err := b.repo.StoreTree([]repository.TreeEntry{{ObjectType: repository.Blob, Hash: blob, Name: seqFile}})
	if err != nil {
		return err
	}
	commit, err := b.repo.StoreCommit(tree)
	if err != nil {
		return err
	}
	return b.setSeq(commit)
}

// Compare is compare for the tests.
var Compare = (*Backend).compare

// StoreCommit stores a commit holding name on parents, for the tests.
func StoreCommit(b *Backend, name string, parents ...repository.Hash) (repository.Hash, error) {
	blob, err := b.repo.StoreData([]byte(name))
	if err != nil {
		return "", err
	}
	tree, err := b.repo.StoreTree([]repository.TreeEntry{{ObjectType: repository.Blob, Hash: blob, Name: name}})
	if err != nil {
		return "", err
	}
	return b.repo.StoreCommit(tree, parents...)
}
