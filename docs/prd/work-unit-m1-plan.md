# Work unit M1: project numbers, implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Issues get a project number, `CO2-3075`, handed out through a
counter ref in git, with GitHub's numbers kept for the issues from before the
switch.

**Architecture:** The counter is `refs/board/seq`, a chain of commits whose
tree holds one file, `numbers` (`prefix`, `start`, then `N <git-bug id>`
lines), read and written through git-bug's `repository.Repo` (go-git), fetched
and pushed on its own refspec `refs/board/*`. Numbering commits on the fetched
counter and pushes; a non-fast-forward rejection means someone else took the
number, so it fetches and tries again. `gitbugio.Issue` gains its project
number, and the one number lookup (`HasNumber`, `HasForgeNumber`,
`BareNumber`) decides what a bare number, `#N` and `CO2-N` mean.

**Tech Stack:** Go 1.x, git-bug fork (`bridge-sync`) `repository` and `cache`
packages, cobra, Vue 3 + Quasar.

**Spec:** `docs/prd/work-unit.md`, section M1.

## Global Constraints

- The prefix lives in the counter (`prefix CO2` in `numbers`), not in
  `.git-board.toml`: every clone reads the same one. This replaces the PRD's
  `[numbers] prefix` table; Task 6 updates the PRD.
- A prefix is a letter, then letters or digits: `^[A-Za-z][A-Za-z0-9]*$`.
- One remote hands numbers out: `origin` (`gitbugio.NumbersRemote`). Never a
  mirror.
- `#N` always means a forge's number. A bare `N`, and `CO2-N`, mean the
  project number once the project numbers its issues; before, a bare number
  means a forge's, as today.
- An issue's project number: its line in `numbers`; else its GitHub number
  when below `start`; else none yet.
- A project without a counter touches no remote when numbering: nothing
  changes for it.
- Live tests only on the scratch forges (guilbep/git-board-test); never push a
  counter to co2-calculator.
- Comments and docs in the repository's voice: short, plain sentences saying
  what a thing is; `ponytail:` comments for deliberate ceilings.

## Review Focus

1. **A clone that never fetched the counter** creates an issue: it stays
   unnumbered, nothing fails, and the next sync numbers it. (Task 3:
   `TestNumbersNotYetFetched`.)
2. **A remote with no `refs/board/*` at all** (every project before its
   switch): fetching the numbers is not an error. (Task 3:
   `TestNumbersBeforeTheSwitch`.)
3. **A push rejected halfway** leaves the local counter as fetched, not with a
   number nobody else will ever see. (Task 3: `TestNumbersPushIsTheLock`
   checks the counter after the rejection.)
4. **A bare number that is one issue's CO2 number and another's GitHub
   number** finds only the first, in the filter and in branch names. (Task 2:
   `TestMatchesProjectNumbers`, `TestNamesProjectNumbers`.)
5. **Initialising twice**, from either clone, refuses and names the existing
   counter. (Task 3: `TestInitNumbersOnce`.)

---

### Task 1: the counter file

**Files:**
- Create: `internal/gitbugio/numbers.go`
- Test: `internal/gitbugio/numbers_test.go`

**Interfaces:**
- Produces: `type Counter struct { Prefix string; Start int; ByID map[string]int }`;
  `func ParseCounter(data []byte) (Counter, error)`; `func (c Counter) Bytes() []byte`;
  `func (c Counter) Next() int`; `func (c Counter) NumberOf(id string, forges []Forge) int`;
  `func (c Counter) Format(n int) string`; `const NumbersRemote = "origin"`;
  `var prefixRe` (unexported, used by Task 3).

- [ ] **Step 1: Write the failing test**

`internal/gitbugio/numbers_test.go`:

```go
package gitbugio_test

import (
	"testing"

	"github.com/guilbep/git-board/internal/gitbugio"
)

// The numbers file: a prefix, where the counter starts, then one line per
// numbered issue. GitHub numbers below start are the issues' own.
func TestCounter(t *testing.T) {
	c, err := gitbugio.ParseCounter([]byte("prefix CO2\nstart 3052\n3053 bbb\n3052 aaa\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Prefix != "CO2" || c.Start != 3052 || c.ByID["aaa"] != 3052 || c.ByID["bbb"] != 3053 {
		t.Fatalf("parsed %+v", c)
	}
	if got, want := string(c.Bytes()), "prefix CO2\nstart 3052\n3052 aaa\n3053 bbb\n"; got != want {
		t.Errorf("Bytes() = %q, want %q", got, want)
	}
	if c.Next() != 3054 {
		t.Errorf("Next() = %d, want 3054", c.Next())
	}
	empty, _ := gitbugio.ParseCounter([]byte("prefix CO2\nstart 3052\n"))
	if empty.Next() != 3052 {
		t.Errorf("Next() of an empty counter = %d, want its start", empty.Next())
	}

	github := func(n string) []gitbugio.Forge { return []gitbugio.Forge{{Name: "GitHub", Number: n}} }
	for _, tc := range []struct {
		name   string
		id     string
		forges []gitbugio.Forge
		want   int
	}{
		{"its line", "aaa", nil, 3052},
		{"its line over its GitHub number", "bbb", github("2518"), 3053},
		{"GitHub, below start", "ccc", github("2518"), 2518},
		{"GitHub, at start: not yet", "ddd", github("3052"), 0},
		{"GitLab's never counts", "eee", []gitbugio.Forge{{Name: "GitLab", Number: "34"}}, 0},
		{"nowhere", "fff", nil, 0},
	} {
		if got := c.NumberOf(tc.id, tc.forges); got != tc.want {
			t.Errorf("%s: NumberOf = %d, want %d", tc.name, got, tc.want)
		}
	}
	if c.Format(2518) != "CO2-2518" {
		t.Errorf("Format(2518) = %q", c.Format(2518))
	}

	for _, bad := range []string{"", "start 3052\n", "prefix CO2\n", "prefix 2CO\nstart 1\n", "prefix CO2\nstart x\n", "prefix CO2\nstart 1\nnot-a-line\n"} {
		if _, err := gitbugio.ParseCounter([]byte(bad)); err == nil {
			t.Errorf("ParseCounter(%q): no error", bad)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/gitbugio/ -run TestCounter`
Expected: FAIL, `undefined: gitbugio.ParseCounter`.

- [ ] **Step 3: Write the minimal implementation**

`internal/gitbugio/numbers.go`:

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/gitbugio/ -run TestCounter`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/gitbugio/numbers.go internal/gitbugio/numbers_test.go
git commit -m "feat(numbers): the counter's file, and which number an issue has"
```

---

### Task 2: the one number lookup knows its counters

**Files:**
- Modify: `internal/gitbugio/gitbugio.go:37-66` (Issue fields, `Numbers()` → `BareNumber()`, `HasNumber`, new `HasForgeNumber`, new `Headline`)
- Modify: `internal/branches/branches.go:56-61` (`number`), `:100-107` (`Names` comment)
- Modify: `internal/filter/filter.go:1-8` (package comment), `:122-132` (`itself`), `:220-221` (`KindNumber`)
- Test: `internal/filter/filter_test.go`, `internal/branches/branches_test.go`

**Interfaces:**
- Consumes: nothing from Task 1.
- Produces: `Issue.Number string` (json `number`, `CO2-3075`, "" for none);
  `Issue.Counted bool` (json `-`); `func (is Issue) BareNumber() string`;
  `func (is Issue) HasNumber(n string) bool`; `func (is Issue) HasForgeNumber(n string) bool`;
  `func (is Issue) Headline() string`. `Issue.Numbers()` is removed.

- [ ] **Step 1: Write the failing tests**

Append to `internal/filter/filter_test.go`:

```go
// Once the project numbers its issues, a bare number and CO2-3060 are the
// project's number, #3060 GitHub's: X is CO2-3060 (GitHub #3071), Y is
// GitHub #3060 imported after the switch (CO2-3075).
func TestMatchesProjectNumbers(t *testing.T) {
	cfg := mustLoad(t)
	x := gitbugio.Issue{ID: "aaaaaaaaaa", Title: "Bouncer pool", Status: "open", Number: "CO2-3060", Counted: true, Forges: []gitbugio.Forge{{Name: "GitHub", Number: "3071"}}}
	y := gitbugio.Issue{ID: "bbbbbbbbbb", Title: "CSV export", Status: "open", Number: "CO2-3075", Counted: true, Forges: []gitbugio.Forge{{Name: "GitHub", Number: "3060"}}}
	for expr, want := range map[string][2]bool{ // [x, y]
		"3060":     {true, false},
		"CO2-3060": {true, false},
		"co2-3060": {true, false},
		"#3060":    {false, true},
		"#3071":    {true, false},
		"3071":     {false, false}, // a bare number is the project's now
		"CO2-30":   {false, false},
	} {
		c := Parse(expr)
		if got := [2]bool{Matches(x, c, cfg, "", today), Matches(y, c, cfg, "", today)}; got != want {
			t.Errorf("%q: %v, want %v", expr, got, want)
		}
	}
}
```

Append to `internal/branches/branches_test.go`:

```go
// Once the project numbers its issues, a branch's number is the project's:
// fix/3060-… is CO2-3060's, not GitHub #3060's; an issue the counter hasn't
// numbered yet names its branch by its id.
func TestNamesProjectNumbers(t *testing.T) {
	x := gitbugio.Issue{ID: "aaaaaaaaaa", Title: "Bouncer pool", Number: "CO2-3060", Counted: true, Forges: []gitbugio.Forge{{Name: "GitHub", Number: "3071"}}}
	y := gitbugio.Issue{ID: "bbbbbbbbbb", Title: "CSV export", Number: "CO2-3075", Counted: true, Forges: []gitbugio.Forge{{Name: "GitHub", Number: "3060"}}}
	waiting := gitbugio.Issue{ID: "cccccccccc", Title: "New one", Counted: true, Forges: []gitbugio.Forge{{Name: "GitHub", Number: "3080"}}}
	for _, tc := range []struct {
		branch string
		is     gitbugio.Issue
		want   bool
	}{
		{"fix/3060-bouncer-pool", x, true},
		{"fix/3060-bouncer-pool", y, false},
		{"fix/3071-bouncer-pool", x, false},
		{"fix/3080-new-one", waiting, false},
		{"ccccccc-new-one", waiting, true},
	} {
		if got := Names(tc.branch, tc.is); got != tc.want {
			t.Errorf("Names(%q, %s) = %v, want %v", tc.branch, tc.is.Title, got, tc.want)
		}
	}
	if got := Name(config.Branches{}, x); got != "3060-bouncer-pool" {
		t.Errorf("Name(x) = %q", got)
	}
	if got := Name(config.Branches{}, waiting); got != "ccccccc-new-one" {
		t.Errorf("Name(waiting) = %q", got)
	}
}
```

(`branches_test.go` already imports `config` and `gitbugio`; check with
`head -15 internal/branches/branches_test.go` and add the imports if not.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/filter/ ./internal/branches/`
Expected: FAIL to compile, `unknown field Number in struct literal of type gitbugio.Issue`.

- [ ] **Step 3: Implement**

In `internal/gitbugio/gitbugio.go`, add to `Issue` after `Forges`:

```go
	// Number is its project number, CO2-3075: "" until the counter hands it
	// one, and while the project doesn't number its issues (numbers.go).
	Number string `json:"number,omitempty"`
	// Counted tells the project numbers its issues: a bare number (3075,
	// fix/3075-…) then means a project number; #3075 is still a forge's.
	Counted bool `json:"-"`
```

Replace `Numbers` and `HasNumber` (lines 51-66) with:

```go
// BareNumber is the number a bare mention names it by (3075, fix/3075-…):
// its project number once the project numbers its issues, else its first
// forge number; "" for none. With HasNumber and HasForgeNumber, the one
// place a number is told to name an issue, for search, branch names and
// titles.
func (is Issue) BareNumber() string {
	if is.Counted {
		return is.Number[strings.LastIndexByte(is.Number, '-')+1:]
	}
	for _, f := range is.Forges {
		if f.Number != "" {
			return f.Number
		}
	}
	return ""
}

// HasNumber tells whether a bare number names the issue: its project
// number once the project numbers its issues, else any of its forge
// numbers.
func (is Issue) HasNumber(n string) bool {
	if is.Counted {
		return n != "" && is.BareNumber() == n
	}
	return is.HasForgeNumber(n)
}

// HasForgeNumber tells whether #n, a forge's own way to name an issue,
// names it.
func (is Issue) HasForgeNumber(n string) bool {
	return n != "" && slices.ContainsFunc(is.Forges, func(f Forge) bool { return f.Number == n })
}

// Headline is its title after its project number, as lists show it:
// "CO2-3075 Restarting Postgres empties the bouncer pool".
func (is Issue) Headline() string {
	if is.Number == "" {
		return is.Title
	}
	return is.Number + " " + is.Title
}
```

In `internal/branches/branches.go`, replace `number`:

```go
func number(is gitbugio.Issue) string {
	return cmp.Or(is.BareNumber(), is.ID[:min(7, len(is.ID))])
}
```

and in the `Name` comment change `{number} (its first forge number, else its
git-bug id's first 7 characters)` to `{number} (its project number, else its
first forge number before the project numbers its issues, else its git-bug
id's first 7 characters)`. In the `Names` comment change `its forge number as
the name's first number` to `its number (BareNumber) as the name's first
number`.

In `internal/filter/filter.go`:
- package comment: `#2528 (an issue's number on a forge)` becomes `#2528 (an
  issue's number on a forge), CO2-2528 (its project number)`;
- `KindNumber`: `ok = issue.HasNumber(value)` becomes `ok = issue.HasForgeNumber(value)`;
- `itself`:

```go
// itself: a word that is an issue's number (2528: its project number once
// the project numbers its issues, else a forge's), its project number as
// written (CO2-2528), or the start of its git-bug id, 7 characters at least
// as git-bug shows it, finds that issue too.
func itself(issue gitbugio.Issue, word string) bool {
	if digits(word) {
		return issue.HasNumber(word)
	}
	if issue.Number != "" && strings.EqualFold(word, issue.Number) {
		return true
	}
	word = strings.ToLower(word)
	return len(word) >= 7 && strings.Trim(word, "0123456789abcdef") == "" && strings.HasPrefix(issue.ID, word)
}
```

- [ ] **Step 4: Run the tests to verify they pass, and nothing else broke**

Run: `go build ./... && go test ./internal/...`
Expected: PASS. (`go build` catches any other caller of the removed
`Numbers()`; there are none on main besides `branches.number`.)

- [ ] **Step 5: Commit**

```bash
git add internal/gitbugio/gitbugio.go internal/branches/ internal/filter/
git commit -m "feat(numbers): a bare number is the project's once it numbers its issues, #N a forge's"
```

---

### Task 3: the counter in git: init, fetch, number, read

**Files:**
- Modify: `internal/gitbugio/numbers.go` (Backend methods)
- Modify: `internal/gitbugio/gitbugio.go:152-212` (`ListIssues`, `issueFrom`, `GetIssue` read the counter)
- Create: `internal/gitbugio/export_test.go`
- Test: `internal/gitbugio/numbers_test.go`

**Interfaces:**
- Consumes: `Counter`, `ParseCounter`, `NumberOf`, `Format`, `prefixRe`,
  `seqNamespace`, `seqKey`, `seqFile` (Task 1); `Issue.Number`, `Issue.Counted` (Task 2).
- Produces: `func (b *Backend) Counter() (Counter, bool, error)`;
  `func (b *Backend) FetchNumbers(remote string) error`;
  `func (b *Backend) InitNumbers(remote, prefix string, start int) error`;
  `func (b *Backend) NumberIssues(remote string) (int, error)`;
  test-only `gitbugio.NumberOnce(b, remote) (int, bool, error)`.

- [ ] **Step 1: Write the failing tests**

`internal/gitbugio/export_test.go`:

```go
package gitbugio

// NumberOnce is numberOnce for the tests: one try, on the counter as fetched.
var NumberOnce = (*Backend).numberOnce
```

Append to `internal/gitbugio/numbers_test.go` (add
`"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"` and
`"strings"` to its imports):

```go
// Before the switch nothing changes: no counter, no number, and fetching
// from a remote that has none is no error.
func TestNumbersBeforeTheSwitch(t *testing.T) {
	mine, _ := gitbugiotest.NewPair(t)
	id := gitbugiotest.NewIssue(t, mine, "Crash")
	if err := mine.FetchNumbers("origin"); err != nil {
		t.Fatalf("FetchNumbers: %v", err)
	}
	if n, err := mine.NumberIssues("origin"); n != 0 || err != nil {
		t.Fatalf("NumberIssues = %d, %v", n, err)
	}
	is, err := mine.GetIssue(id)
	if err != nil || is.Counted || is.Number != "" {
		t.Fatalf("GetIssue = %+v, %v", is, err)
	}
}

// GitHub numbers below start are the issues' own, whenever they are
// imported; an issue opened on GitHub after the switch gets the next one.
func TestNumbersGitHubBelowStart(t *testing.T) {
	mine, _ := gitbugiotest.NewPair(t)
	old := gitbugiotest.NewImportedIssue(t, mine, "Old", "https://github.com/o/r/issues/2518")
	late := gitbugiotest.NewImportedIssue(t, mine, "Late", "https://github.com/o/r/issues/3060")
	if err := mine.InitNumbers("origin", "CO2", 3052); err != nil {
		t.Fatalf("InitNumbers: %v", err)
	}
	if n, err := mine.NumberIssues("origin"); n != 1 || err != nil {
		t.Fatalf("NumberIssues = %d, %v; want 1, the late one", n, err)
	}
	for id, want := range map[string]string{old: "CO2-2518", late: "CO2-3052"} {
		if is, err := mine.GetIssue(id); err != nil || is.Number != want || !is.Counted {
			t.Errorf("GetIssue(%s) = %q counted %v, %v; want %s", id[:7], is.Number, is.Counted, err, want)
		}
	}
	c, _, _ := mine.Counter()
	if _, listed := c.ByID[old]; listed {
		t.Error("the counter lists an issue its GitHub number already numbers")
	}
}

// Two clones number at once: the push is the lock. Theirs, on a counter
// mine has moved, is refused and left as fetched; its next try takes the
// next number.
func TestNumbersPushIsTheLock(t *testing.T) {
	mine, theirs := gitbugiotest.NewPair(t)
	if err := mine.InitNumbers("origin", "CO2", 3052); err != nil {
		t.Fatalf("InitNumbers: %v", err)
	}
	if err := theirs.FetchNumbers("origin"); err != nil {
		t.Fatalf("FetchNumbers: %v", err)
	}
	a := gitbugiotest.NewIssue(t, mine, "Mine")
	b := gitbugiotest.NewIssue(t, theirs, "Theirs")
	if n, err := mine.NumberIssues("origin"); n != 1 || err != nil {
		t.Fatalf("mine: %d, %v", n, err)
	}
	if _, rejected, err := gitbugio.NumberOnce(theirs, "origin"); !rejected || err == nil {
		t.Fatalf("theirs on a stale counter: rejected %v, %v", rejected, err)
	}
	if c, _, _ := theirs.Counter(); len(c.ByID) != 0 {
		t.Fatalf("a refused push left %v in their counter", c.ByID)
	}
	if n, err := theirs.NumberIssues("origin"); n != 1 || err != nil {
		t.Fatalf("theirs: %d, %v", n, err)
	}
	for _, clone := range []*gitbugio.Backend{mine, theirs} {
		if err := clone.FetchNumbers("origin"); err != nil {
			t.Fatal(err)
		}
		if c, _, _ := clone.Counter(); c.ByID[a] != 3052 || c.ByID[b] != 3053 {
			t.Errorf("counter %v; want %s 3052, %s 3053", c.ByID, a[:7], b[:7])
		}
	}
}

// A clone that hasn't fetched the counter yet keeps its issue unnumbered
// and fails nothing; the next fetch and numbering give it its number.
func TestNumbersNotYetFetched(t *testing.T) {
	mine, theirs := gitbugiotest.NewPair(t)
	if err := mine.InitNumbers("origin", "CO2", 3052); err != nil {
		t.Fatal(err)
	}
	id := gitbugiotest.NewIssue(t, theirs, "Offline")
	if n, err := theirs.NumberIssues("origin"); n != 0 || err != nil {
		t.Fatalf("before fetching: %d, %v", n, err)
	}
	if err := theirs.FetchNumbers("origin"); err != nil {
		t.Fatal(err)
	}
	if is, _ := theirs.GetIssue(id); !is.Counted || is.Number != "" {
		t.Fatalf("fetched, not numbered yet: %+v", is)
	}
	if n, err := theirs.NumberIssues("origin"); n != 1 || err != nil {
		t.Fatalf("after fetching: %d, %v", n, err)
	}
	if is, _ := theirs.GetIssue(id); is.Number != "CO2-3052" {
		t.Fatalf("number %q", is.Number)
	}
}

// Init once: a second init, from either clone, names the counter there is;
// a prefix is a letter then letters or digits.
func TestInitNumbersOnce(t *testing.T) {
	mine, theirs := gitbugiotest.NewPair(t)
	if err := mine.InitNumbers("origin", "co-2", 3052); err == nil {
		t.Fatal("prefix co-2 taken")
	}
	if err := mine.InitNumbers("origin", "CO2", 3052); err != nil {
		t.Fatal(err)
	}
	for _, clone := range []*gitbugio.Backend{mine, theirs} {
		if err := clone.InitNumbers("origin", "XY", 1); err == nil || !strings.Contains(err.Error(), "CO2, from 3052") {
			t.Errorf("second init: %v", err)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/gitbugio/ -run 'Numbers|InitNumbers'`
Expected: FAIL to compile, `mine.FetchNumbers undefined`.

- [ ] **Step 3: Implement the Backend side**

Append to `internal/gitbugio/numbers.go` (imports become: `cmp`, `errors`,
`fmt`, `io`, `maps`, `regexp`, `slices`, `sort`, `strconv`, `strings`,
`github.com/git-bug/git-bug/cache`, `github.com/git-bug/git-bug/repository`,
`github.com/go-git/go-git/v5/plumbing/transport`):

```go
// Counter is the project's numbers as this clone last fetched them; ok is
// false while the project doesn't number its issues.
func (b *Backend) Counter() (c Counter, ok bool, err error) {
	h, err := b.repo.ResolveRef(seqNamespace, seqKey)
	if errors.Is(err, repository.ErrNotFound) {
		return Counter{}, false, nil
	}
	if err != nil {
		return Counter{}, false, err
	}
	commit, err := b.repo.ReadCommit(h)
	if err != nil {
		return Counter{}, false, fmt.Errorf("reading refs/board/seq: %w", err)
	}
	entries, err := b.repo.ReadTree(commit.TreeHash)
	if err != nil {
		return Counter{}, false, fmt.Errorf("reading refs/board/seq: %w", err)
	}
	for _, e := range entries {
		if e.Name != seqFile {
			continue
		}
		r, err := b.repo.ReadData(e.Hash)
		if err != nil {
			return Counter{}, false, fmt.Errorf("reading refs/board/seq: %w", err)
		}
		defer r.Close()
		data, err := io.ReadAll(r)
		if err != nil {
			return Counter{}, false, fmt.Errorf("reading refs/board/seq: %w", err)
		}
		c, err := ParseCounter(data)
		return c, err == nil, err
	}
	return Counter{}, false, fmt.Errorf("refs/board/seq holds no %s file", seqFile)
}

// counter is Counter for a listing: nil while the project doesn't number
// its issues.
func (b *Backend) counter() (*Counter, error) {
	c, ok, err := b.Counter()
	if !ok {
		return nil, err
	}
	return &c, nil
}

// FetchNumbers brings the remote's counter and makes it this clone's: a
// clone never holds numbers the remote hasn't, as numbering pushes at once.
// A remote without one is no error: the project doesn't number its issues.
func (b *Backend) FetchNumbers(remote string) error {
	if _, err := b.repo.FetchRefs(remote, seqNamespace); err != nil && !errors.Is(err, transport.ErrEmptyRemoteRepository) {
		return fmt.Errorf("fetching the numbers from %s: %w", remote, err)
	}
	theirs, err := b.repo.ResolveTrackingRef(remote, seqNamespace, seqKey)
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	ours, err := b.repo.ResolveRef(seqNamespace, seqKey)
	if errors.Is(err, repository.ErrNotFound) {
		ours = ""
	} else if err != nil {
		return err
	}
	if ours == theirs {
		return nil
	}
	return b.repo.UpdateRef(seqNamespace, seqKey, ours, theirs)
}

// InitNumbers makes the project number its issues, as prefix-N from start:
// the counter's first commit, pushed to remote. Pushing it is the switch.
func (b *Backend) InitNumbers(remote, prefix string, start int) error {
	if !prefixRe.MatchString(prefix) {
		return fmt.Errorf("a prefix is a letter, then letters or digits, like CO2; not %q", prefix)
	}
	if start < 1 {
		return fmt.Errorf("numbers start from 1 at least, not %d", start)
	}
	if err := b.FetchNumbers(remote); err != nil {
		return err
	}
	if c, ok, err := b.Counter(); err != nil || ok {
		if err == nil {
			err = fmt.Errorf("the project already numbers its issues: %s, from %d", c.Prefix, c.Start)
		}
		return err
	}
	_, err := b.pushCounter(remote, Counter{Prefix: prefix, Start: start, ByID: map[string]int{}}, "")
	return err
}

// NumberIssues gives the issues without a number the next ones, oldest
// first, and says how many it numbered. A project that doesn't number its
// issues, as this clone last fetched it, touches no remote.
func (b *Backend) NumberIssues(remote string) (int, error) {
	if _, ok, err := b.Counter(); !ok {
		return 0, err
	}
	// ponytail: five tries; more people numbering in the same second is
	// beyond a team of ten
	for range 5 {
		if err := b.FetchNumbers(remote); err != nil {
			return 0, err
		}
		n, rejected, err := b.numberOnce(remote)
		if !rejected {
			return n, err
		}
	}
	return 0, errors.New("the numbers kept moving: sync again")
}

// numberOnce numbers the issues without one on the counter as fetched, and
// pushes it; rejected: someone else pushed first.
func (b *Backend) numberOnce(remote string) (n int, rejected bool, err error) {
	c, _, err := b.Counter()
	if err != nil {
		return 0, false, err
	}
	head, err := b.repo.ResolveRef(seqNamespace, seqKey)
	if err != nil {
		return 0, false, err
	}
	var todo []*cache.BugExcerpt
	for _, id := range b.c.Bugs().AllIds() {
		e, err := b.c.Bugs().ResolveExcerpt(id)
		if err != nil {
			return 0, false, err
		}
		if forges, _ := forgesOf(e.CreateMetadata); c.NumberOf(id.String(), forges) == 0 {
			todo = append(todo, e)
		}
	}
	if len(todo) == 0 {
		return 0, false, nil
	}
	sort.Slice(todo, func(i, j int) bool {
		if todo[i].CreateUnixTime != todo[j].CreateUnixTime {
			return todo[i].CreateUnixTime < todo[j].CreateUnixTime
		}
		return todo[i].Id() < todo[j].Id()
	})
	next := c.Next()
	for i, e := range todo {
		c.ByID[e.Id().String()] = next + i
	}
	if rejected, err := b.pushCounter(remote, c, head); err != nil {
		return 0, rejected, err
	}
	return len(todo), false, nil
}

// pushCounter commits c on parent ("" for the first commit) and pushes it.
// A failed push leaves the counter as it was; rejected says git refused it,
// someone else having pushed first.
func (b *Backend) pushCounter(remote string, c Counter, parent repository.Hash) (rejected bool, err error) {
	data, err := b.repo.StoreData(c.Bytes())
	if err != nil {
		return false, err
	}
	tree, err := b.repo.StoreTree([]repository.TreeEntry{{ObjectType: repository.Blob, Hash: data, Name: seqFile}})
	if err != nil {
		return false, err
	}
	var parents []repository.Hash
	if parent != "" {
		parents = append(parents, parent)
	}
	commit, err := b.repo.StoreCommit(tree, parents...)
	if err != nil {
		return false, err
	}
	if err := b.repo.UpdateRef(seqNamespace, seqKey, parent, commit); err != nil {
		return false, err
	}
	if _, err := b.repo.PushRefs(remote, seqNamespace); err != nil {
		undo := b.repo.UpdateRef(seqNamespace, seqKey, commit, parent)
		if parent == "" {
			undo = b.repo.RemoveRef(seqNamespace, seqKey)
		}
		return strings.Contains(err.Error(), "non-fast-forward"), errors.Join(fmt.Errorf("pushing the numbers to %s: %w", remote, err), undo)
	}
	return false, nil
}
```

- [ ] **Step 4: Issues carry their number**

In `internal/gitbugio/gitbugio.go`, `ListIssues` reads the counter once:

```go
func (b *Backend) ListIssues() ([]Issue, error) {
	ids, err := b.c.Bugs().Query(query.NewQuery())
	if err != nil {
		return nil, fmt.Errorf("listing bugs: %w", err)
	}
	c, err := b.counter()
	if err != nil {
		return nil, err
	}
	issues := make([]Issue, len(ids))
	for i, id := range ids {
		excerpt, err := b.c.Bugs().ResolveExcerpt(id)
		if err != nil {
			return nil, fmt.Errorf("resolving %s: %w", id, err)
		}
		issues[i] = b.issueFrom(id.String(), excerpt, c)
	}
	return issues, nil
}
```

`issueFrom` takes it and sets the number:

```go
// issueFrom is excerptToIssue with who opened it, for author:, and its
// project number when c, the counter, numbers the project's issues.
func (b *Backend) issueFrom(id string, e *cache.BugExcerpt, c *Counter) Issue {
	issue := excerptToIssue(id, e)
	if c != nil {
		issue.Counted = true
		if n := c.NumberOf(id, issue.Forges); n > 0 {
			issue.Number = c.Format(n)
		}
	}
	// … the AuthorLogins loop, unchanged
	return issue
}
```

`GetIssue`:

```go
func (b *Backend) GetIssue(idPrefix string) (Issue, error) {
	excerpt, err := b.c.Bugs().ResolveExcerptPrefix(idPrefix)
	if err != nil {
		return Issue{}, fmt.Errorf("resolving %s: %w", idPrefix, err)
	}
	c, err := b.counter()
	if err != nil {
		return Issue{}, err
	}
	return b.issueFrom(excerpt.Id().String(), excerpt, c), nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/gitbugio/ -run 'Counter|Numbers|InitNumbers' -v`
Expected: PASS, the five tests. If `TestNumbersBeforeTheSwitch` fails on
the fetch with an error about no matching refs, make `FetchNumbers` treat
that error as "no counter" too (match the error go-git returns, in the same
`if`), and say so in its comment.

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/gitbugio/
git commit -m "feat(numbers): the counter in git, refs/board/seq: init, fetch, number with the push as the lock"
```

---

### Task 4: numbering from the CLI and the server, numbers in the CLI and TUI

**Files:**
- Create: `cmd/git-board/numbers.go`
- Modify: `cmd/git-board/cmd.go:67` (register `newNumbersCmd()`)
- Modify: `cmd/git-board/sync.go` (`syncOne`)
- Modify: `cmd/git-board/render.go:91,109`, `cmd/git-board/html.go:167`
- Modify: `internal/tui/tui.go:40`, `internal/tui/detail.go:74`
- Modify: `cmd/git-board-server/issues.go` (`handleNewIssue`), `cmd/git-board-server/remotes.go` (`handleRemote`), `cmd/git-board-server/forges.go` (`runForgeSync`'s `finish`)
- Test: `cmd/git-board/sync_test.go`

**Interfaces:**
- Consumes: `gitbugio.NumbersRemote`, `(*Backend).InitNumbers`, `FetchNumbers`,
  `NumberIssues` (Task 3); `Issue.Headline()` (Task 2).
- Produces: `git board numbers` and `git board numbers init --prefix CO2 --start 3052`.

- [ ] **Step 1: Write the failing test**

Append to `cmd/git-board/sync_test.go`:

```go
// sync fetches the counter and numbers what this clone made since.
func TestSyncNumbers(t *testing.T) {
	pathA, pathB := twoReposWithRemote(t)
	a, err := gitbugio.Open(pathA)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.InitNumbers("origin", "CO2", 3052); err != nil {
		t.Fatal(err)
	}
	a.Close()
	seedBug(t, pathB, "Made before B knew")
	var out strings.Builder
	if err := runSync(&out, []string{pathB}, "origin"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "numbered 1 issue") {
		t.Errorf("sync said:\n%s", out.String())
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./cmd/git-board/ -run TestSyncNumbers`
Expected: FAIL, the output has no "numbered 1 issue".

- [ ] **Step 3: Implement sync and the command**

In `cmd/git-board/sync.go`, `syncOne`: after the pull block,

```go
	if remote == gitbugio.NumbersRemote {
		if err := b.FetchNumbers(remote); err != nil {
			return fmt.Errorf("numbers: %w", err)
		}
	}
```

and after the push succeeds, before `synced`,

```go
	if remote == gitbugio.NumbersRemote {
		n, err := b.NumberIssues(remote)
		if err != nil {
			return fmt.Errorf("numbering: %w", err)
		}
		if n > 0 {
			fmt.Fprintf(out, "%s: numbered %d issue(s)\n", path, n)
		}
	}
```

`cmd/git-board/numbers.go`:

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/gitbugio"
)

func newNumbersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "numbers",
		Short: "Number the issues that have no project number yet",
		Long: `Number the issues that have no project number yet, through the
counter on origin (refs/board/seq). sync and the web UI do it too; this
is for when neither ran. A project that doesn't number its issues: nothing.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := gitbugio.OpenCwd()
			if err != nil {
				return err
			}
			defer b.Close()
			if err := b.FetchNumbers(gitbugio.NumbersRemote); err != nil {
				return err
			}
			n, err := b.NumberIssues(gitbugio.NumbersRemote)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "numbered %d issue(s)\n", n)
			return nil
		},
	}
	cmd.AddCommand(newNumbersInitCmd())
	return cmd
}

func newNumbersInitCmd() *cobra.Command {
	var prefix string
	var start int
	cmd := &cobra.Command{
		Use:   "init --prefix CO2 --start 3052",
		Short: "Start the project's own numbers: the switch, for everyone",
		Long: `Start the project's own numbers, CO2-3052 and on, by pushing the
counter's first commit to origin. That push is the switch, for every clone.

GitHub numbers below start stay the issues' numbers (#2518 is CO2-2518);
the counter numbers the rest. start is one more than GitHub's highest
issue or pull request number:

  gh api 'repos/OWNER/REPO/issues?state=all&per_page=1' -q '.[0].number'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := gitbugio.OpenCwd()
			if err != nil {
				return err
			}
			defer b.Close()
			if err := b.InitNumbers(gitbugio.NumbersRemote, prefix, start); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "numbering as %s-N from %d, on %s\n", prefix, start, gitbugio.NumbersRemote)
			return nil
		},
	}
	cmd.Flags().StringVar(&prefix, "prefix", "", "the numbers' prefix, like CO2")
	cmd.Flags().IntVar(&start, "start", 0, "the first number the counter hands out")
	_ = cmd.MarkFlagRequired("prefix")
	_ = cmd.MarkFlagRequired("start")
	return cmd
}
```

Register it in `cmd/git-board/cmd.go:67`'s `root.AddCommand(...)` list:
add `newNumbersCmd()`.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./cmd/git-board/ -run TestSyncNumbers`
Expected: PASS.

- [ ] **Step 5: The server numbers on create, push, pull and forge sync**

`cmd/git-board-server/issues.go`, `handleNewIssue`, right after
`s.b.NewIssue(…)` succeeds:

```go
	// numbered at once when the project numbers its issues; offline, at
	// the next sync
	if _, err := s.b.NumberIssues(gitbugio.NumbersRemote); err != nil {
		log.Printf("numbering %s: %v", id, err)
	}
```

`cmd/git-board-server/remotes.go`, `handleRemote`'s map:

```go
		"pull": func(remote string) error {
			err := s.b.Pull(remote)
			if err == nil && remote == gitbugio.NumbersRemote {
				err = s.b.FetchNumbers(remote)
			}
			return err
		},
		"push": func(remote string) error {
			_, err := s.b.Push(remote)
			if err == nil && remote == gitbugio.NumbersRemote {
				_, err = s.b.NumberIssues(remote)
			}
			return err
		},
```

`cmd/git-board-server/forges.go`, in `runForgeSync`'s `finish`, before
`s.seeRefs()`:

```go
		// issues a forge brought in get their numbers
		if !job.DryRun {
			if _, err := s.b.NumberIssues(gitbugio.NumbersRemote); err != nil {
				log.Printf("numbering after the forge sync: %v", err)
			}
		}
```

Add the `log` and `gitbugio` imports where missing.

- [ ] **Step 6: The CLI and TUI show the number before the title**

- `cmd/git-board/render.go:91` and `:109`: `issue.Title` becomes `issue.Headline()`.
- `cmd/git-board/html.go:167`: `Title: issue.Title,` becomes `Title: issue.Headline(),`.
- `internal/tui/tui.go:40`: `func (i item) Title() string { return i.issue.Headline() }`.
- `internal/tui/detail.go:74`: the first `issue.Title` becomes `issue.Headline()`.

- [ ] **Step 7: Run everything**

Run: `go build ./... && go test ./...`
Expected: PASS. Golden-output tests of `ls`/`view`/`html` stay unchanged:
their issues have no counter, so `Headline()` is the title.

- [ ] **Step 8: Commit**

```bash
git add cmd/ internal/tui/
git commit -m "feat(numbers): git board numbers [init], sync and the server number new issues"
```

---

### Task 5: the web shows the number

**Files:**
- Modify: `web/src/api/board.ts:15-23` (`Issue.number`)
- Modify: `web/src/components/IssueBoard.vue:61` (card)
- Modify: `web/src/components/IssuePanel.vue` (title `h2`)
- Modify: `web/src/components/IssueTable.vue:87` (title cell)
- Modify: `web/src/css/gitboard.css` (`.gb-num`)

**Interfaces:**
- Consumes: the API's issue JSON `number` (Task 2's `json:"number,omitempty"`).

- [ ] **Step 1: The type**

In `web/src/api/board.ts`'s `Issue`, after `forges`:

```ts
  number?: string; // its project number, CO2-3075; absent until it has one
```

- [ ] **Step 2: Card, title, row**

- `IssueBoard.vue:61`: `{{ issue.id.slice(0, 8) }}` becomes `{{ issue.number ?? issue.id.slice(0, 8) }}`.
- `IssuePanel.vue`, the title: `<h2 v-if="!renaming" :id="`title-${uid}`">{{ thread.title }}</h2>` becomes
  `<h2 v-if="!renaming" :id="`title-${uid}`"><span v-if="thread.number" class="gb-num">{{ thread.number }}</span>{{ thread.title }}</h2>`.
- `IssueTable.vue:87`, inside the title button: `{{ issue.title }}` becomes
  `<span v-if="issue.number" class="gb-num">{{ issue.number }}</span>{{ issue.title }}`.
- `web/src/css/gitboard.css`, after `.gb-mono`:

```css
.gb-num{font-family:var(--font-mono);font-weight:500;opacity:.6;margin-right:.4em}
```

- [ ] **Step 3: Check it builds and the tests pass**

Run: `cd web && npm run typecheck && npm test`
Expected: no type errors; tests pass.

- [ ] **Step 4: See it**

Run the app against a scratch clone with a counter (the `run` skill, or
`make` + `git-board-server` in a clone of guilbep/git-board-test after
`git board numbers init --prefix GBT --start <its next>`), and check a card,
the issue page and a table row show `GBT-N`.

- [ ] **Step 5: Commit**

```bash
git add web/src/
git commit -m "feat(web): an issue's project number on its card, its page and its row"
```

---

### Task 6: the docs

**Files:**
- Modify: `docs/configuration.md` (a `## Numbers` section after `## Branches`)
- Modify: `docs/prd/work-unit.md` (M1: the prefix in the counter)
- Modify: `README.md` (the command list, if it lists commands)

- [ ] **Step 1: `docs/configuration.md`**

After the Branches section:

````markdown
## Numbers

A project can number its issues itself, `CO2-3075`, instead of leaning on
a forge's numbers. The counter is in git, `refs/board/seq` on `origin`: no
key here. Start it once, for everyone:

```bash
git board numbers init --prefix CO2 --start 3052
```

`start` is one more than GitHub's highest issue or pull request number.
GitHub numbers below it stay the issues' own: `#2518` is `CO2-2518`, even
for an issue imported later. Every other issue, made here or imported,
gets the next number when it is made, or at the next sync when offline.

After the switch, a bare number (`3075`, `fix/3075-…`) and `CO2-3075` are
the project's number; `#3075` is always GitHub's.
````

and in the Branches section's table or text, where `{number}` is described,
say: `{number}` is the project number once the project numbers its issues.

- [ ] **Step 2: the PRD**

In `docs/prd/work-unit.md`, M1: replace the toml block

```toml
[numbers]
prefix = "CO2"
```

with one sentence: "The prefix is in the counter itself (`prefix CO2`), so
every clone reads the same one; `.git-board.toml` has no key for it." Change
`It holds one file, numbers: a first line start 3052` to `It holds one file,
numbers: prefix CO2, start 3052`, and `git board numbers init` to
`git board numbers init --prefix CO2 --start 3052`. In the Forges bullet,
replace `; to confirm with the main session, which expected it to need an
entry` with `: the fork's refIndex only rewrites #n, a forge's own
reference syntax (confirmed with the main session)`. In the "One lookup"
bullet, replace `return each number with where it comes from (project,
GitHub, GitLab), not a flat list` with `tell a bare number (HasNumber,
BareNumber) from a forge's #N (HasForgeNumber)`.

- [ ] **Step 3: README**

Run: `grep -n "git board sync\|git-board sync" README.md | head`. Where the
README lists commands, add one line: `git board numbers [init]`: the
project's own numbers, see docs/configuration.md § Numbers.

- [ ] **Step 4: Commit**

```bash
git add docs/ README.md
git commit -m "docs: numbers, how a project numbers its issues"
```
