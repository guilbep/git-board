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
	text   []byte
	tables map[string]toml.Primitive
	md     toml.MetaData
	names  []string // declared
}

// Load reads the file at path once; a missing one is an empty file. A
// file that isn't TOML is an error: no table of it can be read.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &File{path: path, tables: map[string]toml.Primitive{}}, nil
	}
	if err != nil {
		return nil, err
	}
	f, err := Parse(path, data)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return f, nil
}

// Parse reads text as the file at path would be: what a write checks
// before it lands.
func Parse(path string, text []byte) (*File, error) {
	f := &File{path: path, text: text, tables: map[string]toml.Primitive{}}
	var err error
	if f.md, err = toml.Decode(string(text), &f.tables); err != nil {
		return nil, err
	}
	return f, nil
}

// Path is where the file is, or goes.
func (f *File) Path() string { return f.path }

// Text is the file's bytes, nil when there is none.
func (f *File) Text() []byte { return f.text }

// Exists says there is a file.
func (f *File) Exists() bool { return f.text != nil }

// Unknown are the file's keys no declared table read, in the file's
// order: a misspelled key above all. Ask once every table was read.
func (f *File) Unknown() []string {
	undecoded := map[string]bool{}
	for _, k := range f.md.Undecoded() {
		undecoded[k.String()] = true
	}
	var res []string
	for _, k := range f.md.Keys() {
		// a table no domain declares is unknown itself, not its keys
		if undecoded[k.String()] || len(k) == 1 && !slices.Contains(f.names, k[0]) {
			res = append(res, k.String())
		}
	}
	return res
}

// Type is the TOML type of a key's value (md.Type): "String", "Hash"…
func (f *File) Type(key ...string) string { return f.md.Type(key...) }

// Problem is what is wrong with a table.
type Problem struct {
	Table string
	Err   error
}

// Error is the domain's message: it names its table and key, as v1's.
func (p Problem) Error() string { return p.Err.Error() }

func (p Problem) Unwrap() error { return p.Err }

// Table is one table of the file, declared by the domain that reads it.
type Table[T any] struct {
	Name    string // "fields": [fields]
	Default func() T
	Check   func(*T) error // the domain's own rules; it may fill defaults in
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
	if h.t.Check != nil {
		if err := h.t.Check(&v); err != nil {
			return v, []Problem{{h.t.Name, err}}
		}
	}
	return v, nil
}

// Primitive is the table undecoded, for a domain that reads its parts
// by their type (a [cycle.kinds] string or table); ok false without one.
func (h *Handle[T]) Primitive() (p toml.Primitive, ok bool) {
	p, ok = h.f.tables[h.t.Name]
	return p, ok
}

// Decode decodes a part of the table Primitive gave.
func (h *Handle[T]) Decode(p toml.Primitive, v any) error { return h.f.md.PrimitiveDecode(p, v) }

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

// Problems are Get's problems alone.
func (h *Handle[T]) Problems() []Problem {
	_, p := h.Get()
	return p
}
