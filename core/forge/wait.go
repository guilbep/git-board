package forge

import (
	"context"
	"time"
)

// Outage is how long a read waits out a dropped connection or a server
// error, in waits doubling from a second to OutageStep: a Wi-Fi drop, a
// laptop asleep. A write is never tried again: the forge may have made it.
const (
	Outage     = 5 * time.Minute
	OutageStep = 30 * time.Second
)

// Backoff is the wait before a read's next try after fails failures, and
// whether the outage, with waited already waited, has room for it.
func Backoff(fails int, waited time.Duration) (time.Duration, bool) {
	d := min(time.Second<<min(fails, 5), OutageStep)
	return d, waited+d <= Outage
}

// Wait is an adapter waiting before it asks again: its project's key, why
// ("GitHub doesn't answer"), and for how long.
type Wait struct {
	Forge, Why string
	For        time.Duration
}

type waitsKey struct{}

// WithWaits is ctx, through which the adapters tell hear each wait.
func WithWaits(ctx context.Context, hear func(Wait)) context.Context {
	return context.WithValue(ctx, waitsKey{}, hear)
}

// Waiting tells ctx's hearer, if any, of a wait.
func Waiting(ctx context.Context, w Wait) {
	if hear, ok := ctx.Value(waitsKey{}).(func(Wait)); ok {
		hear(w)
	}
}
