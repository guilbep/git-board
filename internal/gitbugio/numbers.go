package gitbugio

import (
	"cmp"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// The project's own numbers (CO2-3075): refs/board/seq, a chain of commits
// whose tree holds one file, numbers:
//
//	prefix CO2
//	start 3052
//	3052 <git-bug id>
//	3053 <git-bug id>
//
// An issue's number is its line there; else its GitHub number when below
// start, the number it had before the project numbered its own; else none
// yet. Numbering pushes the next commit: a push git refuses (not a
// fast-forward) means someone else took the number, so fetch and take the
// next one.

// NumbersRemote is the one remote that hands numbers out. A mirror never
// does: two counters would give a number twice.
// ponytail: origin only, as internal/branches; a setting when a project's isn't
const NumbersRemote = "origin"

const (
	seqNamespace = "board"
	seqKey       = "seq"
	seqFile      = "numbers"
)

var prefixRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)

// Counter is the project's numbers, as refs/board/seq holds them.
type Counter struct {
	Prefix string         // CO2
	Start  int            // the first number the counter hands out
	ByID   map[string]int // the numbers it handed out, by git-bug id
}

// ParseCounter reads a numbers file.
func ParseCounter(data []byte) (Counter, error) {
	c := Counter{ByID: map[string]int{}}
	for i, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		key, value, _ := strings.Cut(strings.TrimSpace(line), " ")
		switch key {
		case "prefix":
			c.Prefix = value
		case "start":
			n, err := strconv.Atoi(value)
			if err != nil {
				return Counter{}, fmt.Errorf("numbers, line %d: start %q is not a number", i+1, value)
			}
			c.Start = n
		default:
			n, err := strconv.Atoi(key)
			if err != nil || value == "" {
				return Counter{}, fmt.Errorf("numbers, line %d: %q is not a number and an issue", i+1, line)
			}
			c.ByID[value] = n
		}
	}
	if !prefixRe.MatchString(c.Prefix) || c.Start < 1 {
		return Counter{}, fmt.Errorf("numbers: a prefix and a start, not %q and %d", c.Prefix, c.Start)
	}
	return c, nil
}

// Bytes is the numbers file, its lines by number.
func (c Counter) Bytes() []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "prefix %s\nstart %d\n", c.Prefix, c.Start)
	for _, id := range slices.SortedFunc(maps.Keys(c.ByID), func(x, y string) int { return cmp.Compare(c.ByID[x], c.ByID[y]) }) {
		fmt.Fprintf(&b, "%d %s\n", c.ByID[id], id)
	}
	return []byte(b.String())
}

// Next is the number the counter hands out next.
func (c Counter) Next() int {
	next := c.Start
	for _, n := range c.ByID {
		next = max(next, n+1)
	}
	return next
}

// NumberOf is an issue's number, 0 for none yet: its line, else its GitHub
// number below Start.
func (c Counter) NumberOf(id string, forges []Forge) int {
	if n, ok := c.ByID[id]; ok {
		return n
	}
	for _, f := range forges {
		if n, err := strconv.Atoi(f.Number); f.Name == "GitHub" && err == nil && n < c.Start {
			return n
		}
	}
	return 0
}

// Format is a number as people write it: CO2-3075.
func (c Counter) Format(n int) string { return fmt.Sprintf("%s-%d", c.Prefix, n) }
