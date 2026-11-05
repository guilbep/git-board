// Package conf is .git-board.toml: read once, each table declared by the
// domain that reads it (docs/rewrite/design.md, Config). A table is its
// domain's: one that doesn't decode is that domain's problem, and the
// others still answer. M1 reads; writing a table back comes with M2.
package conf

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"

	"github.com/BurntSushi/toml"
)

// FileName is the config's name, at the root of the repository.
const FileName = ".git-board.toml"

// File is one .git-board.toml, read.
type File struct {
	path   string
	tables map[string]toml.Primitive
	md     toml.MetaData
	names  []string // declared
}

// Load reads the file at path once; a missing one is an empty file. A
// file that isn't TOML is an error: no table of it can be read.
func Load(path string) (*File, error) {
	f := &File{path: path, tables: map[string]toml.Primitive{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return nil, err
	}
	if f.md, err = toml.Decode(string(data), &f.tables); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return f, nil
}

// Problem is what is wrong with a table.
type Problem struct {
	Table string
	Err   error
}

func (p Problem) Error() string { return fmt.Sprintf("[%s]: %v", p.Table, p.Err) }

// Table is one table of the file, declared by the domain that reads it.
type Table[T any] struct {
	Name    string // "fields": [fields]
	Default func() T
	Check   func(T) []error // the domain's own rules
}

// Handle is a declared table.
type Handle[T any] struct {
	f *File
	t Table[T]
}

// Declare registers a table on the file. Two domains can't declare one
// name.
func Declare[T any](f *File, t Table[T]) *Handle[T] {
	if slices.Contains(f.names, t.Name) {
		panic("conf: [" + t.Name + "] declared twice")
	}
	f.names = append(f.names, t.Name)
	return &Handle[T]{f, t}
}

// Get is the table, checked; its default when the file has none. A table
// that doesn't decode gives its default and the problem.
func (h *Handle[T]) Get() (T, []Problem) {
	var v T
	if h.t.Default != nil {
		v = h.t.Default()
	}
	p, ok := h.f.tables[h.t.Name]
	if !ok {
		return v, nil
	}
	if err := h.f.md.PrimitiveDecode(p, &v); err != nil {
		return v, []Problem{{h.t.Name, err}}
	}
	var probs []Problem
	if h.t.Check != nil {
		for _, err := range h.t.Check(v) {
			probs = append(probs, Problem{h.t.Name, err})
		}
	}
	return v, probs
}

// Defined says the file has the table.
func (h *Handle[T]) Defined() bool {
	_, ok := h.f.tables[h.t.Name]
	return ok
}

// Keys are the keys right under table (a dotted path: "branches",
// "kinds"), in the order the file writes them.
func (h *Handle[T]) Keys(path ...string) []string {
	table := append([]string{h.t.Name}, path...)
	var res []string
	for _, k := range h.f.md.Keys() {
		if len(k) == len(table)+1 && slices.Equal(k[:len(table)], table) {
			res = append(res, k[len(table)])
		}
	}
	return res
}
