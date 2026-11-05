package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/guilbep/git-board/store"
)

// seq is the counter's ref (docs/spec/datastores.md, The counter, In
// git): a chain of commits, each holding the counter's bytes as numbers.
const seq = "refs/board/seq"

// Count is the counter: its bytes (nil when the project doesn't number
// its issues) and its version, the commit.
func (s *Store) Count(ctx context.Context) ([]byte, string, error) {
	if !s.hasRef(seq) {
		return nil, "", nil
	}
	w, err := s.walker(ctx)
	if err != nil {
		return nil, "", err
	}
	defer w.close()
	w.in.WriteString("info " + seq + "\ncontents " + seq + ":numbers\nflush\n")
	if err := w.in.Flush(); err != nil {
		return nil, "", err
	}
	line, err := w.out.ReadString('\n')
	if err != nil {
		return nil, "", fmt.Errorf("git cat-file: %w", err)
	}
	f := strings.Fields(line)
	if len(f) != 3 || f[1] != "commit" {
		w.read()            // the numbers' answer
		return nil, "", nil // moved away meanwhile, or not a commit: no counter
	}
	_, typ, body, err := w.read()
	if err != nil {
		return nil, "", err
	}
	if typ != "blob" {
		return nil, f[0], fmt.Errorf("%s holds no numbers file", seq)
	}
	return body, f[0], nil
}

// SetCount writes the counter in a commit on prev ("" for the first),
// and moves refs/board/seq there if it is still at prev; ErrMoved
// otherwise. Pushing it to origin is numbering's, not the store's.
func (s *Store) SetCount(ctx context.Context, value []byte, prev string) (string, error) {
	blob, err := s.writeObject("blob", value)
	if err != nil {
		return "", err
	}
	raw, _ := hexDecode(blob)
	tree, err := s.writeObject("tree", append([]byte("100644 numbers\x00"), raw...))
	if err != nil {
		return "", err
	}
	var parents []string
	old := zero
	if prev != "" {
		parents, old = []string{prev}, prev
	}
	commit, err := s.writeCommit(tree, parents)
	if err != nil {
		return "", err
	}
	return commit, s.updateRef(ctx, seq, commit, old)
}

// hasRef says a ref exists, loose or packed, without starting git.
func (s *Store) hasRef(name string) bool {
	if _, err := os.Stat(filepath.Join(s.commonDir, filepath.FromSlash(name))); err == nil {
		return true
	}
	packed, err := os.ReadFile(filepath.Join(s.commonDir, "packed-refs"))
	return err == nil && strings.Contains(string(packed), " "+name+"\n")
}

var _ store.Counter = (*Store)(nil)
