package git

import (
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
)

// writeObject stores an object as git does a loose one, and answers its
// id: zlib("<type> <size>\x00<data>") at objects/<2>/<38>, written aside
// then renamed, so a reader never sees half of it.
func (s *Store) writeObject(typ string, data []byte) (string, error) {
	head := append(append([]byte(typ+" "), strconv.Itoa(len(data))...), 0)
	h := sha1.New()
	h.Write(head)
	h.Write(data)
	id := hex.EncodeToString(h.Sum(nil))
	dir := filepath.Join(s.commonDir, "objects", id[:2])
	path := filepath.Join(dir, id[2:])
	if _, err := os.Stat(path); err == nil {
		return id, nil // there already: objects never change
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, "tmp_obj_")
	if err != nil {
		return "", err
	}
	z := zlib.NewWriter(f)
	_, err = z.Write(head)
	if err == nil {
		_, err = z.Write(data)
	}
	err = errors.Join(err, z.Close(), f.Close())
	if err == nil {
		err = os.Chmod(f.Name(), 0o444)
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		_ = os.Remove(f.Name())
		if _, statErr := os.Stat(path); !errors.Is(statErr, fs.ErrNotExist) && statErr == nil {
			return id, nil // another writer put it there first
		}
		return "", err
	}
	return id, nil
}

func hexDecode(h string) ([]byte, error) { return hex.DecodeString(h) }
