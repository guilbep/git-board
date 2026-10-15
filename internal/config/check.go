package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
)

// SyntaxError is a .git-board.toml that isn't TOML, where it breaks.
type SyntaxError struct {
	Line    int    `json:"line"`
	Col     int    `json:"col"`
	Message string `json:"message"`
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("line %d, column %d: %s", e.Line, e.Col, e.Message)
}

// Check parses a .git-board.toml as the settings page edits it: a
// *SyntaxError when it isn't TOML, else the config and warnings about
// what git-board won't use — an unknown key (a typo), a view's layout
// other than board or table, a sprint slug given twice.
func Check(data []byte) (*Config, []string, error) {
	cfg, md, err := parse(data)
	if err != nil {
		var pe toml.ParseError
		if errors.As(err, &pe) {
			// from the byte offset: the parser's line is the next one when
			// the error is the newline ending a line ("[views.x" unclosed)
			at := min(pe.Position.Start, len(data))
			before := data[:at]
			line := bytes.Count(before, []byte("\n")) + 1
			col := at - bytes.LastIndexByte(before, '\n')
			return nil, nil, &SyntaxError{Line: line, Col: col, Message: pe.Message}
		}
		return nil, nil, err
	}
	var warnings []string
	for _, k := range md.Undecoded() {
		warnings = append(warnings, fmt.Sprintf("%s: git-board doesn't know this key", k))
	}
	names := make([]string, 0, len(cfg.Views))
	for name := range cfg.Views {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if l := cfg.Views[name].Layout; l != "" && l != "board" && l != "table" {
			warnings = append(warnings, fmt.Sprintf("views.%s: layout %q isn't board or table (shown as a table)", name, l))
		}
	}
	seen := map[string]bool{}
	for _, sp := range cfg.Sprints.List {
		if seen[sp.Slug] {
			warnings = append(warnings, fmt.Sprintf("sprints: %q is listed twice", sp.Slug))
		}
		seen[sp.Slug] = true
	}
	return cfg, warnings, nil
}

// Save writes a .git-board.toml whole, through a file next to it renamed
// over it: a crash leaves the old file or the new one, never half.
func Save(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".git-board.toml.*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
