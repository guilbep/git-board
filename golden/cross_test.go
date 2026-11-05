package golden

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// writes are the commands the cross-write test runs on the basic board,
// one writer at a time: each of M2's writes, with the cases a write
// refuses.
var writes = []string{
	`new "Cross write one" prio=p1 -b "a body, with ünicode"`,
	`new "Cross write two" status=ready kind:bug assignee=bob`,
	`comment 26a4ba8 -b "a comment from the cross test"`,
	`comment 22d74e7 -b "another"`,
	`comment 22d74e7 --edit-last -b "another, edited"`,
	`edit 398523d --title "Retitled by the cross test"`,
	`edit 398523d --title "Retitled by the cross test"`,
	`edit 398523d -b "a new body"`,
	`set 26a4ba8 prio=p0 est=3`,
	`set 26a4ba8 prio=p9`,
	`set 26a4ba8 nosuch=x`,
	`set 26a4ba8 assignee=+chloe`,
	`set 26a4ba8 assignee=-chloe`,
	`move 26a4ba8 in-review`,
	`move 22d74e7 --next`,
	`move 22d74e7 --prev`,
	`move 398523d --prev`,
	`close 999eab2 -c "closing it" --reason not-planned`,
	`close 22d74e7 --duplicate-of 9062b9a`,
	`close 22d74e7 --duplicate-of 22d74e7`,
	`reopen 999eab2 -c "back again"`,
	`set 398523d assignee=+@me status=done`,
	`set 398523d status=ready`,
	`set 22c49b4 est=`,
	`set 22c49b4 kind=bug`,
	`new`,
	`new "Bad field" prio=p9`,
	`comment 9062b9a -b ""`,
	`comment 999eab2 --delete nosuch --yes`,
	`delete 427cb1b --yes`,
	`comment 427cb1b -b "on a deleted issue"`,
	`set 427cb1b prio=p1`,
	`comment 427cb1b --edit-last -b "on a deleted issue"`,
	`delete 0000000 --yes`,
	`undo 26a4ba8`,
	`undo 26a4ba8`,
	`undo 26a4ba8`,
	`undo 999eab2`,
	`undo 999eab2 --force`,
	`undo 22c49b4`,
	`undo --json`,
	`undo`,
	`undo e958491`,
	`follow 9062b9a`,
	`unfollow 26a4ba8`,
	`follow 22c49b4 --auto`,
	`follow 0000000`,
	`follow 398523d --json`,
	`labels`,
	`me`,
	`config set views.ready.filter "status:ready is:open prio:p0"`,
	`config set views.mine.filter "assignee:@me"`,
	`config set views.mine.sort "updated desc"`,
	`config set sprints.length_days 21`,
	`config set sprints.length_days two`,
	`config set fields.prio.options '["p0", "p1", "p2", "p3", "p4"]'`,
	`config set fields.nope.colors 1`,
	`config set roles.reviewer @bob`,
	`config set github-project.owner guilbep`,
	`config unset sprints.start_weekday`,
	`config get views.mine.filter`,
	`config get github-project.number`,
	`config get sprints.length_days`,
	`view mine`,
	`numbers`,
	`numbers init --prefix GB --start 1`,
	`set --filter "prio:p0" prio=p2`,
	`set --filter "prio:p0 is:open,closed" prio=p2 assignee=+@me --apply`,
	`set --filter "is:open" nosuch=x`,
	`set --filter "status:nope" prio=p1`,
	`set 26a4ba8 prio=p1 --apply`,
	`set --filter "is:open" status=done --apply`,
	`labels rename prio:p2 prio:p3`,
	`labels rename nosuch other`,
	`labels rename same same`,
	`labels rename prio:p3 " spaced"`,
	`comment 9062b9a --delete 6a8541a --yes --json id,deleted`,
	`comment 9062b9a --delete d3ad24b --yes --template "{{.ref}} {{.deleted}}"`,
	`get 26a4ba8`,
	`get 0000000`,
}

// mdWrites are the writes the cross-write test runs on the Markdown
// board, whose issues are its own.
var mdWrites = []string{
	`new "Cross write on a folder" prio=p1 -b "a body"`,
	`comment c82cfa3 -b "a comment from the cross test"`,
	`comment fe82974 -b "another, ünicode"`,
	`comment fe82974 --edit-last -b "another, edited"`,
	`edit c82cfa3 --title "Retitled by the cross test"`,
	`edit fe82974 -b "a new body"`,
	`set c82cfa3 prio=p0 est=3`,
	`set c82cfa3 prio=p9`,
	`move c82cfa3 in-review`,
	`close fe82974 -c "closing it"`,
	`reopen fe82974`,
	`set fe82974 assignee=+@me`,
	`comment 703c174 -b "on a deleted issue"`,
	`undo c82cfa3`,
	`delete fe82974 --yes`,
	`labels`,
	`numbers`,
	`numbers init --prefix GB --start 1`,
	`numbers init --prefix 1x --start 1`,
}

var shortIDHex = regexp.MustCompile(`\b[0-9a-f]{7,64}\b`)

// upgraded is the line v2 says on the write that upgrades the file's
// schema (docs/guide/config-schema.md), which v1 didn't: taken out of
// what a write says.
var upgraded = regexp.MustCompile(`(?m)^\.git-board\.toml upgraded from schema \d+ to \d+ .*\n`)

// tomlLine is the line a TOML error names: in the file v2 upgraded, the
// schema key's lines over the tables put it lower than in v1's.
var tomlLine = regexp.MustCompile(`toml: line \d+`)

// The cross tests ran v1 and v2 side by side until the switch (v0.12,
// ADR 0021). v1's side is recorded since, as make compat's outputs are:
// each test's transcript is in testdata/golden/cross/<test>/, in Write's
// form, every step's output normalised as the test says, and the binary
// is held to it. make golden records them again from the binary it
// builds: review the diff, as it changes the contract.
const crossRoot = root + "/cross"

// CrossBinEnv names the binary the cross tests run instead of the one
// they build (make golden sets it).
const CrossBinEnv = "GIT_BOARD_CROSS_BIN"

// moduleRoot is the module's root, from this package.
const moduleRoot = ".."

// binary is the git-board the cross tests run: CrossBinEnv's, else
// ./cmd/git-board built now.
func binary(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds the binary: not with -short")
	}
	if bin := os.Getenv(CrossBinEnv); bin != "" {
		abs, err := filepath.Abs(bin)
		if err != nil {
			t.Fatal(err)
		}
		return abs
	}
	// go test caches a result by the files the test reads: stat every
	// source the binary builds from, so an edit runs the test again.
	err := filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "testdata" || d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".")) && path != moduleRoot {
			return filepath.SkipDir
		}
		if !d.IsDir() && (strings.HasSuffix(path, ".go") || d.Name() == "go.mod" || d.Name() == "go.sum") {
			_, err = os.Stat(path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "git-board")
	if out, err := exec.Command("go", "build", "-o", bin, "../cmd/git-board").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// transcript is what a cross test saw, step by step, to hold against
// its recording.
type transcript struct {
	cases   []Case
	results map[string]Result
}

// add records a step: its command line and its result, normalised.
func (tr *transcript) add(line string, r Result) {
	if tr.results == nil {
		tr.results = map[string]Result{}
	}
	name := fmt.Sprintf("%02d-%s", len(tr.cases)+1, slug(line))
	tr.cases = append(tr.cases, Case{name, line})
	tr.results[name] = r
}

// file records a file's bytes, as a step whose stdout they are.
func (tr *transcript) file(name string, data []byte) {
	tr.add("file "+name, Result{Stdout: data})
}

// check holds the transcript to the recording at testdata/golden/cross/
// <name>, or records it there with GIT_BOARD_GOLDEN_UPDATE=1.
func (tr *transcript) check(t *testing.T, name string) {
	t.Helper()
	dir := filepath.Join(crossRoot, name)
	if os.Getenv(UpdateEnv) == "1" {
		if err := Write(dir, tr.cases, tr.results); err != nil {
			t.Fatal(err)
		}
		return
	}
	rec, err := Read(dir)
	if err != nil {
		t.Fatalf("%v: make golden", err)
	}
	if !slices.Equal(rec.Cases, tr.cases) {
		t.Fatalf("the steps recorded aren't the test's: make golden\nrecorded:\n%v\nnow:\n%v", rec.Cases, tr.cases)
	}
	for _, c := range tr.cases {
		want, got := rec.Results[c.Name], tr.results[c.Name]
		if want.Exit != got.Exit || !bytes.Equal(want.Stdout, got.Stdout) || !bytes.Equal(want.Stderr, got.Stderr) {
			t.Errorf("%s (%s): differs from the recording\nexit %d, want %d\nstderr:\n%s\nstdout:\n%s",
				c.Name, c.Line, got.Exit, want.Exit, Diff(want.Stderr, got.Stderr), Diff(want.Stdout, got.Stdout))
		}
	}
}

// today is the date of today, as a write stamps it in a line.
var today = regexp.MustCompile(time.Now().UTC().Format("2006-01-02"))

// dsn is a PostgreSQL board's database, which each checkout has its own
// of, on the server the machine has.
var dsn = regexp.MustCompile(`postgres(ql)?://[^"\s]*`)

// readNorm is a read's output after writes, as recorded: JSON as canon
// has it, the times of now, new ids and a database's URL out.
func readNorm(r Result) Result {
	if json.Valid(r.Stdout) && len(bytes.TrimSpace(r.Stdout)) > 0 {
		r.Stdout = append(canon(decode(r.Stdout)), '\n')
	}
	for _, p := range []*[]byte{&r.Stdout, &r.Stderr} {
		*p = dsn.ReplaceAll(today.ReplaceAll(unstamped(*p), []byte("TODAY")), []byte("$POSTGRES"))
	}
	return r
}

// noSchema is config list --json without the file's schema key, which
// v2 writes on its first write (docs/guide/config-schema.md) and v1
// never did.
func noSchema(out []byte) []byte {
	var keys []map[string]any
	if json.Unmarshal(out, &keys) != nil {
		return out
	}
	keys = slices.DeleteFunc(keys, func(k map[string]any) bool { return k["key"] == "schema" })
	b, _ := json.Marshal(keys)
	return b
}

// TestCrossWrites: the writes print what v1 printed, on each datastore:
// git (basic), a Markdown folder (markdown) and PostgreSQL (postgres,
// basic copied in; skipped without a server); and the board they leave
// reads as v1 read the board its own writes left.
func TestCrossWrites(t *testing.T) {
	t.Parallel()
	bin := binary(t)
	byName := map[string]Board{}
	for _, b := range boards(t) {
		byName[b.Name] = b
	}
	for _, c := range []struct {
		board  string
		writes []string
	}{{"basic", writes}, {"markdown", mdWrites}, {"postgres", writes}} {
		b, ok := byName[c.board]
		if !ok {
			t.Fatalf("no %s golden board", c.board)
		}
		t.Run(c.board, func(t *testing.T) {
			t.Parallel()
			crossWrites(t, bin, b, c.writes)
		})
	}
}

func crossWrites(t *testing.T, bin string, board Board, writes []string) {
	if board.Datastore == postgres && os.Getenv(PostgresEnv) == "" {
		t.Skipf("%s: %v", board.Name, ErrNoPostgres)
	}
	ctx := context.Background()
	dir, home, _ := checkout(t, board)
	var tr transcript
	for _, line := range writes {
		r, err := Run(ctx, bin, dir, home, Case{Line: line})
		if err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		r.Stdout = shortIDHex.ReplaceAll(r.Stdout, []byte("ID"))
		r.Stderr = tomlLine.ReplaceAll(upgraded.ReplaceAll(r.Stderr, nil), []byte("toml: line N"))
		tr.add(line, r)
	}
	for _, line := range []string{`ls "is:open,closed" --json`, "labels --json", `ls "follows:@me is:open,closed" --json`, "config list --json", "view ready --json"} {
		r, err := Run(ctx, bin, dir, home, Case{Line: line})
		if err != nil {
			t.Fatal(err)
		}
		if line == "config list --json" {
			r.Stdout = noSchema(r.Stdout)
		}
		tr.add(line, readNorm(r))
	}
	tr.check(t, "writes-"+board.Name)
}

// forged are the keys v1 lacked until M4 (sync records): left out of
// what readNorm records.
var forged = []string{"forges", "github_url", "pending", "shown_title"}

// canon is v as JSON with the forged keys out, to show a difference.
func canon(v any) []byte {
	var strip func(any)
	strip = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			for _, k := range forged {
				delete(v, k)
			}
			for _, e := range v {
				strip(e)
			}
		case []any:
			for _, e := range v {
				strip(e)
			}
		}
	}
	strip(v)
	out, _ := json.MarshalIndent(v, "", "  ") // maps marshal with sorted keys
	return out
}

func decode(b []byte) any {
	var v any
	if json.Unmarshal(b, &v) != nil {
		return string(b)
	}
	return v
}
