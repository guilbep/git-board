package kernel

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// ErrStale is a write that expected an entity at a version it has moved
// from: someone wrote it since it was read (an HTTP If-Match). Nothing is
// written.
var ErrStale = errors.New("changed since it was read")

type expectKey struct{}

// expectation is the version a request expects an entity at. A write
// that lands moves it on, so a request of several writes checks each
// against the one before.
type expectation struct {
	mu sync.Mutex
	id ID
	v  Version
}

// Expect makes ctx's writes on the entity id compare-and-swap from v: a
// Do finding it at another version answers ErrStale and writes nothing,
// checked on the head it would write on, so nothing slips in between.
// Each write that lands expects its own version next.
func Expect(ctx context.Context, id ID, v Version) context.Context {
	return context.WithValue(ctx, expectKey{}, &expectation{id: id, v: v})
}

// check is nil when ctx expects nothing of the entity id, or it is at
// the version expected.
func check(ctx context.Context, kind string, id ID, at Version) error {
	x, _ := ctx.Value(expectKey{}).(*expectation)
	if x == nil || x.id != id {
		return nil
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.v != at {
		return fmt.Errorf("%s %s: %w", kind, id, ErrStale)
	}
	return nil
}

// moved tells ctx's expectation of id that a write moved it to v.
func moved(ctx context.Context, id ID, v Version) {
	if x, _ := ctx.Value(expectKey{}).(*expectation); x != nil && x.id == id {
		x.mu.Lock()
		x.v = v
		x.mu.Unlock()
	}
}
