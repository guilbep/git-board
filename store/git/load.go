package git

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strconv"
	"sync"

	"github.com/guilbep/git-board/kernel/pack"
	"github.com/guilbep/git-board/store"
)

// Load reads the refs' entities through `git cat-file`: one process for a
// few, one per CPU for many, each walking its share. each is called from
// those processes' goroutines, so it may run on several at once.
func (s *Store) Load(ctx context.Context, refs []store.Ref, each func(store.Ref, [][]byte, error) error) error {
	if c := store.CountsOf(ctx); c != nil {
		c.Loads.Add(1)
		c.Entities.Add(int64(len(refs)))
	}
	n := min(runtime.GOMAXPROCS(0), 1+len(refs)/256)
	if n <= 1 {
		return s.loadShare(ctx, refs, each)
	}
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	var wg sync.WaitGroup
	for i := range n {
		var share []store.Ref
		for j := i; j < len(refs); j += n { // interleaved: long and short entities spread
			share = append(share, refs[j])
		}
		wg.Go(func() {
			if err := s.loadShare(ctx, share, each); err != nil {
				cancel(err)
			}
		})
	}
	wg.Wait()
	if err := context.Cause(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return ctx.Err()
}

func (s *Store) loadShare(ctx context.Context, refs []store.Ref, each func(store.Ref, [][]byte, error) error) error {
	w, err := s.walker(ctx)
	if err != nil {
		return err
	}
	w.counts = store.CountsOf(ctx)
	err = w.walk(refs, func(r store.Ref, nodes []node, err error) error {
		var packs [][]byte
		if err == nil {
			packs = make([][]byte, len(nodes))
			for i, n := range nodes {
				packs[i] = n.pack
			}
		}
		return each(r, packs, err)
	})
	w.counts = nil // before another load takes w (done)
	if err != nil {
		w.close() // its answers may be half read
		return err
	}
	s.done(w)
	return nil
}

// idleWalkers is how many cat-files a Store keeps between loads.
const idleWalkers = 4

// done keeps a walker whose answers are all read for the next load, or
// lets it go.
func (s *Store) done(w *walker) {
	select {
	case s.idle <- w:
	default:
		w.close()
	}
}

// node is one entity commit: its parents, its pack and the pack's clock.
type node struct {
	commit  string
	parents []string
	pack    []byte
	clock   uint64
	empty   bool
}

// walker is one `git cat-file --batch-command --buffer`: requests go in
// rounds, each ended by a flush, its answers read in order.
type walker struct {
	in     *bufio.Writer
	out    *bufio.Reader
	wait   func() error
	counts *store.Counts // the load's, while it walks
}

func (s *Store) startWalker(ctx context.Context) (*walker, error) {
	cmd := s.command(ctx, "cat-file", "--batch-command", "--buffer")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &walker{
		in:  bufio.NewWriterSize(stdin, 64<<10),
		out: bufio.NewReaderSize(stdout, 256<<10),
		wait: func() error {
			stdin.Close()
			if err := cmd.Wait(); err != nil {
				return fmt.Errorf("git cat-file: %w: %s", err, bytes.TrimSpace(stderr.Bytes()))
			}
			return nil
		},
	}, nil
}

func (w *walker) close() error { return w.wait() }

// Warm starts a cat-file for the next load, which then waits for no
// process to start: show's, while it checks the view meanwhile.
func (s *Store) Warm(ctx context.Context) {
	s.warm.Do(func() {
		w, err := s.startWalker(context.WithoutCancel(ctx))
		s.warmed <- warmed{w, err}
	})
}

type warmed struct {
	w   *walker
	err error
}

// walker is the warm one when there is one, else an idle one, else a
// new one. It outlives the load's context: Close ends it.
func (s *Store) walker(ctx context.Context) (*walker, error) {
	select {
	case w := <-s.warmed:
		return w.w, w.err
	default:
	}
	select {
	case w := <-s.idle:
		return w, nil
	default:
		return s.startWalker(context.WithoutCancel(ctx))
	}
}

// window is how many entities a walker reads at once: enough for rounds
// of a few thousand objects, few enough to keep their packs small.
const window = 512

// entity is one ref being walked.
type entity struct {
	ref     store.Ref
	nodes   []*node
	seen    map[string]bool // commits asked for
	pending int
	err     error
}

type reqKind uint8

const (
	reqCommit reqKind = iota
	reqTree
	reqBlob
)

type request struct {
	e    *entity
	kind reqKind
	oid  string // or the ref's name, for a head not pinned
	n    *node
}

// walk reads every commit reachable from each ref, its tree and its pack,
// a window of entities at a time, round by round, and checks their clocks
// (storage.md, What a reader rejects). each gets an entity once it is
// whole, its head first; an entity of no ref gets store.ErrNotFound.
func (w *walker) walk(refs []store.Ref, each func(store.Ref, []node, error) error) error {
	var todo []request
	active := 0
	next := 0
	admit := func() error {
		for active < window && next < len(refs) {
			r := refs[next]
			next++
			e := &entity{ref: r, seen: map[string]bool{}}
			oid := r.Version
			if oid == "" {
				oid = refName(r.Kind, r.ID)
			} else if !isHash(oid) {
				e.err = fmt.Errorf("%w: %s at %q", store.ErrNotFound, refName(r.Kind, r.ID), oid)
			}
			if e.err != nil {
				if err := each(r, nil, e.err); err != nil {
					return err
				}
				continue
			}
			e.seen[oid] = true
			e.pending++
			todo = append(todo, request{e: e, kind: reqCommit, oid: oid})
			active++
		}
		return nil
	}
	if err := admit(); err != nil {
		return err
	}
	for len(todo) > 0 {
		round := todo
		todo = nil
		written := make(chan error, 1)
		go func() { written <- w.send(round) }()
		var done []*entity
		for _, rq := range round {
			oid, typ, body, err := w.read()
			if err != nil {
				<-written
				return err
			}
			e := rq.e
			e.pending--
			if e.err == nil {
				todo = w.answer(todo, rq, oid, typ, body)
			}
			if e.pending == 0 {
				done = append(done, e)
			}
		}
		if err := <-written; err != nil {
			return err
		}
		for _, e := range done {
			active--
			var nodes []node
			if e.err == nil {
				nodes, e.err = check(e)
			}
			if err := each(e.ref, nodes, e.err); err != nil {
				return err
			}
		}
		if err := admit(); err != nil {
			return err
		}
	}
	return nil
}

func (w *walker) send(round []request) error {
	for _, rq := range round {
		w.in.WriteString("contents ")
		w.in.WriteString(rq.oid)
		w.in.WriteByte('\n')
	}
	w.in.WriteString("flush\n")
	return w.in.Flush()
}

// read is one answer: an object, or its absence (typ "missing").
func (w *walker) read() (oid, typ string, body []byte, err error) {
	line, err := w.out.ReadSlice('\n')
	if err != nil {
		return "", "", nil, fmt.Errorf("git cat-file: %w", err)
	}
	f := bytes.Fields(line)
	if len(f) == 2 { // "<object> missing", "<object> ambiguous"
		return string(f[0]), string(f[1]), nil, nil
	}
	if len(f) != 3 {
		return "", "", nil, fmt.Errorf("git cat-file said %q", line)
	}
	oid, typ = string(f[0]), string(f[1])
	if w.counts != nil {
		w.counts.Objects.Add(1)
	}
	size, err := strconv.Atoi(string(f[2]))
	if err != nil {
		return "", "", nil, fmt.Errorf("git cat-file said %q", line)
	}
	body = make([]byte, size+1) // and its line feed
	if _, err := io.ReadFull(w.out, body); err != nil {
		return "", "", nil, fmt.Errorf("git cat-file: %w", err)
	}
	return oid, typ, body[:size], nil
}

// answer reads one object into its entity, and asks for what it names.
func (w *walker) answer(todo []request, rq request, oid, typ string, body []byte) []request {
	e := rq.e
	fail := func(format string, args ...any) []request {
		// as v1 words it: broken entities' lines are a contract
		e.err = fmt.Errorf("oplog: %s: "+format, append([]any{refName(e.ref.Kind, e.ref.ID)}, args...)...)
		return todo
	}
	ask := func(kind reqKind, oid string, n *node) {
		e.pending++
		todo = append(todo, request{e: e, kind: kind, oid: oid, n: n})
	}
	switch rq.kind {
	case reqCommit:
		if typ == "missing" && len(e.nodes) == 0 && e.ref.Version == "" {
			e.err = fmt.Errorf("%w: %s", store.ErrNotFound, refName(e.ref.Kind, e.ref.ID))
			return todo
		}
		if typ != "commit" {
			return fail("commit %s: %s", rq.oid, typ)
		}
		if len(e.nodes) == 0 {
			e.ref.Version = oid
		}
		n := &node{commit: oid}
		tree := ""
		for line := range bytes.SplitSeq(body, []byte{'\n'}) {
			if len(line) == 0 {
				break // the headers end
			}
			k, v, _ := bytes.Cut(line, []byte{' '})
			switch string(k) {
			case "tree":
				tree = string(v)
			case "parent":
				n.parents = append(n.parents, string(v))
			}
		}
		if !isHash(tree) {
			return fail("commit %s: no tree", oid)
		}
		e.nodes = append(e.nodes, n)
		ask(reqTree, tree, n)
		for _, p := range n.parents {
			if !e.seen[p] {
				e.seen[p] = true
				ask(reqCommit, p, nil)
			}
		}
	case reqTree:
		if typ != "tree" {
			return fail("commit %s: tree %s: %s", rq.n.commit, rq.oid, typ)
		}
		blob := ""
		for b := body; len(b) > 0; {
			sp := bytes.IndexByte(b, ' ')
			nul := bytes.IndexByte(b, 0)
			if sp < 0 || nul < sp || nul+21 > len(b) {
				return fail("commit %s: tree %s: corrupt", rq.n.commit, rq.oid)
			}
			if string(b[sp+1:nul]) == "pack" {
				blob = fmt.Sprintf("%x", b[nul+1:nul+21])
			}
			b = b[nul+21:]
		}
		if blob == "" {
			return fail("commit %s: no pack", rq.n.commit)
		}
		ask(reqBlob, blob, rq.n)
	case reqBlob:
		if typ != "blob" {
			return fail("commit %s: pack: %s", rq.n.commit, typ)
		}
		clock, empty, err := pack.Header(body)
		if err != nil {
			return fail("commit %s: pack: %v", rq.n.commit, err)
		}
		rq.n.pack, rq.n.clock, rq.n.empty = body, clock, empty
	}
	return todo
}

// check is an entity's commits, its head first, once their clocks hold:
// each one more than its parents' highest, 1 with none; and no pack
// without an operation but a merge's.
func check(e *entity) ([]node, error) {
	clocks := make(map[string]uint64, len(e.nodes))
	for _, n := range e.nodes {
		clocks[n.commit] = n.clock
	}
	out := make([]node, len(e.nodes))
	for i, n := range e.nodes {
		want := uint64(1)
		for _, p := range n.parents {
			want = max(want, clocks[p]+1)
		}
		if n.clock != want {
			return nil, fmt.Errorf("oplog: %s: commit %s: clock %d, want %d", refName(e.ref.Kind, e.ref.ID), n.commit, n.clock, want)
		}
		if n.empty && len(n.parents) < 2 {
			return nil, fmt.Errorf("oplog: %s: commit %s: %w", refName(e.ref.Kind, e.ref.ID), n.commit, pack.ErrEmpty)
		}
		out[i] = *n
	}
	return out, nil
}
