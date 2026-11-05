package proof

import (
	"fmt"
	"regexp"
	"sync"
	"testing"
)

// The ADR's lost-line case: twenty writes to one address, each a separate
// process, every line kept.
func TestAppendParallel(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	addr := run(t, dir, "hash-object", "-w", "--stdin")
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- Append(ctx(t), dir, "alice-host-0001", addr, fmt.Sprintf("line %02d", i))
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	lines, err := Lines(ctx(t), dir, addr)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(lines["alice-host-0001"]); got != 20 {
		t.Errorf("%d lines kept of 20: %v", got, lines)
	}
	if left := run(t, dir, "for-each-ref", "refs/notes/ci-tmp/"); left != "" {
		t.Errorf("tmp refs left: %s", left)
	}
}

func TestLinesEveryWriter(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	addr := run(t, dir, "hash-object", "-w", "--stdin")
	for _, w := range []string{"alice-a-0001", "bob-b-0002"} {
		if err := Append(ctx(t), dir, w, addr, "from "+w); err != nil {
			t.Fatal(err)
		}
	}
	// a fetched copy of carol's ref
	if err := Append(ctx(t), dir, "carol-c-0003", addr, "from carol"); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "update-ref", "refs/notes/remotes/origin/ci/carol-c-0003", "refs/notes/ci/carol-c-0003")
	run(t, dir, "update-ref", "-d", "refs/notes/ci/carol-c-0003")
	lines, err := Lines(ctx(t), dir, addr)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 || lines["bob-b-0002"][0] != "from bob-b-0002" || lines["carol-c-0003"][0] != "from carol" {
		t.Errorf("lines: %v", lines)
	}
	if other, _ := Lines(ctx(t), dir, run(t, dir, "hash-object", "-t", "tree", "/dev/null")); len(other) != 0 {
		t.Errorf("an address with no note: %v", other)
	}
}

func TestWriterName(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	run(t, dir, "config", "user.name", "Ana María O'Neil")
	w, err := WriterName(ctx(t), dir)
	if err != nil || !regexp.MustCompile(`^ana-mar-a-o-neil-[a-z0-9-]+-[0-9a-f]{4}$`).MatchString(w) {
		t.Errorf("writer %q %v", w, err)
	}
	if again, _ := WriterName(ctx(t), dir); again != w {
		t.Errorf("second call %q, first %q", again, w)
	}
}
