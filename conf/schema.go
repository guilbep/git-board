package conf

import (
	"fmt"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// The file's schema has a version (docs/guide/config-schema.md): a
// top-level schema = N, and a file without one is version 1, what v0.11
// writes. Migrations go from each version to the next and back, as
// Alembic's do for a database: edits of the text, every other line and
// comment kept as it was.

// SchemaKey is the top-level key holding the file's schema version.
const SchemaKey = "schema"

// Current is the schema version this git-board writes: Migrations' last.
const Current = 2

// Migration is one step of the schema, From to From+1.
type Migration struct {
	From int    `json:"from"`
	To   int    `json:"to"`
	Desc string `json:"description"` // what it changes, a line
	// Lossy is what Down can't give back; "" when up then down gives the
	// file back byte for byte.
	Lossy string `json:"lossy,omitempty"`
	Up    Edit   `json:"-"`
	Down  Edit   `json:"-"`
}

// unread is the comment beside a key schema 2 sets aside.
const unread = "schema 2: git-board doesn't read this key"

// Migrations are the schema's steps, in order: the i-th goes from i+1 to
// i+2. A new one goes last, with Current, its tests both ways and its
// line in the guide's table (docs/guide/config-schema.md).
var Migrations = []Migration{
	{
		From: 1, To: 2,
		Desc: "sets aside [sprints] start_weekday and [repos] assignees, which git-board doesn't read, in a comment each",
		Up:   Chain(CommentKey("sprints", "start_weekday", unread), CommentKey("repos", "assignees", unread)),
		Down: Chain(UncommentKey("sprints", "start_weekday", unread), UncommentKey("repos", "assignees", unread)),
	},
}

// NewerError is a file whose schema is newer than this git-board's: it
// reads what it knows, and writes nothing.
type NewerError struct{ File int }

func (e *NewerError) Error() string {
	return fmt.Sprintf("%s is schema %d, and this git-board knows schema %d at most: it writes nothing to it. Upgrade git-board, or have a git-board that knows schema %d run git board config downgrade --to %d",
		FileName, e.File, Current, e.File, Current)
}

// Version is text's schema version: its schema key, 1 without one.
func Version(text string) (int, error) {
	var m map[string]any
	if _, err := toml.Decode(text, &m); err != nil {
		return 0, err
	}
	return version(m[SchemaKey])
}

func version(v any) (int, error) {
	switch n := v.(type) {
	case nil:
		return 1, nil
	case int64:
		if n >= 1 {
			return int(n), nil
		}
	}
	return 0, fmt.Errorf("%s = %s: the file's schema version is a whole number from 1", SchemaKey, Literal(v))
}

// Steps are the migrations from one version to another, in the order
// they run: Up's from < to, Down's else.
func Steps(from, to int) ([]Migration, error) {
	for _, v := range []int{from, to} {
		if v < 1 || v > Current {
			return nil, fmt.Errorf("schema %d: this git-board knows schema 1 to %d", v, Current)
		}
	}
	if from <= to {
		return Migrations[from-1 : to-1], nil
	}
	steps := slices.Clone(Migrations[to-1 : from-1])
	slices.Reverse(steps)
	return steps, nil
}

// Migrate is text at schema to, and the steps it took; the text itself
// when it is there already.
func Migrate(text string, to int) (string, []Migration, error) {
	from, err := Version(text)
	if err != nil {
		return "", nil, err
	}
	if from > Current {
		return "", nil, &NewerError{from}
	}
	steps, err := Steps(from, to)
	if err != nil || from == to {
		return text, nil, err
	}
	if text, err = run(text, steps, from < to); err != nil {
		return "", nil, err
	}
	text = setVersion(text, to)
	if _, err := Version(text); err != nil { // what it writes is TOML
		return "", nil, fmt.Errorf("schema %d to %d: %w", from, to, err)
	}
	return text, steps, nil
}

// run applies steps, up or down.
func run(text string, steps []Migration, up bool) (string, error) {
	for _, m := range steps {
		e, from, to := m.Up, m.From, m.To
		if !up {
			e, from, to = m.Down, m.To, m.From
		}
		var err error
		if text, err = e(text); err != nil {
			return "", fmt.Errorf("schema %d to %d: %w", from, to, err)
		}
	}
	return text, nil
}

// ForWrite is text as a write leaves it: at Current, and the version it
// was. A file newer than this git-board is refused (*NewerError).
func ForWrite(text string) (string, int, error) {
	from, err := Version(text)
	if err != nil {
		return "", 0, err
	}
	if from > Current {
		return "", from, &NewerError{from}
	}
	out, _, err := Migrate(text, Current)
	return out, from, err
}

// setVersion writes the schema key, or removes it at 1. A new one goes
// after the file's opening comment and its blank line, else first, a
// blank line under it; removing it takes that blank line too, so a
// version written then removed gives the file back.
func setVersion(text string, v int) string {
	lines := strings.SplitAfter(text, "\n") // each with its newline
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "[") {
			break
		}
		body, nl := strings.CutSuffix(l, "\n")
		m := keyLine.FindStringSubmatch(body)
		if m == nil || m[2] != SchemaKey {
			continue
		}
		if v == 1 {
			n := 1
			if i+1 < len(lines) && lines[i+1] == "\n" {
				n = 2
			}
			return strings.Join(slices.Delete(lines, i, i+n), "")
		}
		lines[i] = fmt.Sprintf("%s%d%s", m[1], v, m[4])
		if nl {
			lines[i] += "\n"
		}
		return strings.Join(lines, "")
	}
	if v == 1 {
		return text
	}
	at := 0
	for at < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[at]), "#") {
		at++
	}
	if at == 0 || at == len(lines) || strings.TrimSpace(lines[at]) != "" {
		at = 0 // no opening comment, or one that is a table's
	} else {
		at++
	}
	line := fmt.Sprintf("%s = %d\n", SchemaKey, v)
	if text == "" {
		return line
	}
	return strings.Join(slices.Insert(lines, at, line, "\n"), "")
}

// readText is what the domains read of a file at version v: the file
// migrated to Current in memory, the file itself when it can't be (a
// write then says why). A message names a line of it: an edit keeps the
// lines where they are when it can (CommentKey does).
func readText(text string, v int) (string, error) {
	if v >= Current {
		return text, nil
	}
	steps, err := Steps(v, Current)
	if err != nil {
		return text, err
	}
	out, err := run(text, steps, true)
	if err != nil {
		return text, err
	}
	return out, nil
}
