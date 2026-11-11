// Package obs traces what git-board does: spans in a context, sent as
// OTLP/JSON to a collector or to Grafana Tempo (docs/guide/performance.md,
// Traces). It is off unless OTEL_EXPORTER_OTLP_ENDPOINT (or
// OTEL_EXPORTER_OTLP_TRACES_ENDPOINT) is set: then Start answers its
// context and a no-op, and nothing is kept or sent.
//
// It is the OpenTelemetry wire format and environment, without the SDK
// (ADR 0014: few dependencies): spans, their attributes and events, W3C
// traceparent in and out. No sampling, metrics or logs: every span of a
// traced process is sent, and a full buffer drops spans, counted.
// ponytail: one exporter, in-process batches; the SDK if sampling or
// metrics are ever needed (the API is the same shape: Start, End).
package obs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Attr is a span's attribute: a string, an int64, a bool or a float64.
type Attr struct {
	Key   string
	Value any
}

// String, Int and Bool are attributes.
func String(k, v string) Attr        { return Attr{k, v} }
func Int(k string, v int64) Attr     { return Attr{k, v} }
func Bool(k string, v bool) Attr     { return Attr{k, v} }
func Float(k string, v float64) Attr { return Attr{k, v} }

// Span is one timed piece of work.
type Span struct {
	trace  [16]byte
	id     [8]byte
	parent [8]byte
	name   string
	kind   int // OTLP: 1 internal, 2 server, 3 client
	start  time.Time

	mu     sync.Mutex
	end    time.Time
	attrs  []Attr
	events []spanEvent
	err    string
}

type spanEvent struct {
	at    time.Time
	name  string
	attrs []Attr
}

type spanKey struct{}

// remote is a parent from another process: a traceparent's ids.
type remote struct {
	trace [16]byte
	span  [8]byte
}

type remoteKey struct{}

var current atomic.Pointer[exporter]

// On says whether spans are kept and sent.
func On() bool { return current.Load() != nil }

func noop(error) {}

// Start begins a span named name under ctx's span, or its traceparent, or
// as a trace's root, and answers the context that carries it and its end.
// end(err) ends it, err its status. Off, it answers ctx and a no-op.
func Start(ctx context.Context, name string, attrs ...Attr) (context.Context, func(error)) {
	return start(ctx, name, 1, attrs)
}

// StartServer is Start for a request this process answers.
func StartServer(ctx context.Context, name string, attrs ...Attr) (context.Context, func(error)) {
	return start(ctx, name, 2, attrs)
}

// StartClient is Start for a request this process makes.
func StartClient(ctx context.Context, name string, attrs ...Attr) (context.Context, func(error)) {
	return start(ctx, name, 3, attrs)
}

func start(ctx context.Context, name string, kind int, attrs []Attr) (context.Context, func(error)) {
	ex := current.Load()
	if ex == nil {
		return ctx, noop
	}
	s := newSpan(ctx, name, kind, attrs)
	return context.WithValue(ctx, spanKey{}, s), func(err error) {
		s.mu.Lock()
		s.end = time.Now()
		if err != nil {
			s.err = err.Error()
		}
		s.mu.Unlock()
		ex.add(s)
	}
}

// newSpan is a span under ctx's span, or its traceparent, or a root.
func newSpan(ctx context.Context, name string, kind int, attrs []Attr) *Span {
	s := &Span{name: name, kind: kind, start: time.Now(), attrs: append([]Attr(nil), attrs...)}
	switch p, r := spanOf(ctx), remoteOf(ctx); {
	case p != nil:
		s.trace, s.parent = p.trace, p.id
	case r != nil:
		s.trace, s.parent = r.trace, r.span
	default:
		rand.Read(s.trace[:])
	}
	rand.Read(s.id[:])
	return s
}

func spanOf(ctx context.Context) *Span {
	s, _ := ctx.Value(spanKey{}).(*Span)
	return s
}

func remoteOf(ctx context.Context) *remote {
	r, _ := ctx.Value(remoteKey{}).(*remote)
	return r
}

// Set adds attributes to ctx's span.
func Set(ctx context.Context, attrs ...Attr) {
	if s := spanOf(ctx); s != nil {
		s.mu.Lock()
		s.attrs = append(s.attrs, attrs...)
		s.mu.Unlock()
	}
}

// Name renames ctx's span: a request's, once its route is known.
func Name(ctx context.Context, name string) {
	if s := spanOf(ctx); s != nil {
		s.mu.Lock()
		s.name = name
		s.mu.Unlock()
	}
}

// Event marks a moment in ctx's span: a wait, a retry.
func Event(ctx context.Context, name string, attrs ...Attr) {
	if s := spanOf(ctx); s != nil {
		s.mu.Lock()
		s.events = append(s.events, spanEvent{time.Now(), name, append([]Attr(nil), attrs...)})
		s.mu.Unlock()
	}
}

// Carry is dst with src's span as its parent: work that outlives a
// request (a sync run, on the server's own context) stays in its trace.
func Carry(dst, src context.Context) context.Context {
	if s := spanOf(src); s != nil {
		return context.WithValue(dst, spanKey{}, s)
	}
	if r := remoteOf(src); r != nil {
		return context.WithValue(dst, remoteKey{}, r)
	}
	return dst
}

// TraceID is ctx's trace, in hex; "" with none.
func TraceID(ctx context.Context) string {
	if s := spanOf(ctx); s != nil {
		return hex.EncodeToString(s.trace[:])
	}
	if r := remoteOf(ctx); r != nil {
		return hex.EncodeToString(r.trace[:])
	}
	return ""
}

// Traceparent is ctx's span as a W3C traceparent header; "" with none.
func Traceparent(ctx context.Context) string {
	if s := spanOf(ctx); s != nil {
		return "00-" + hex.EncodeToString(s.trace[:]) + "-" + hex.EncodeToString(s.id[:]) + "-01"
	}
	return ""
}

// FromTraceparent is ctx with a parent from another process: a page's
// interaction, a caller's span. A header that isn't one is ignored.
func FromTraceparent(ctx context.Context, h string) context.Context {
	parts := strings.Split(strings.TrimSpace(h), "-")
	if len(parts) != 4 || parts[0] != "00" || len(parts[1]) != 32 || len(parts[2]) != 16 {
		return ctx
	}
	var r remote
	if _, err := hex.Decode(r.trace[:], []byte(parts[1])); err != nil {
		return ctx
	}
	if _, err := hex.Decode(r.span[:], []byte(parts[2])); err != nil {
		return ctx
	}
	if r.trace == ([16]byte{}) {
		return ctx
	}
	return context.WithValue(ctx, remoteKey{}, &r)
}

// RecordRoot sends a span measured in another process, with the ids it
// gave it there: a page's interaction, the parent its requests named in
// their traceparent. trace and span are hex; an id that isn't is ignored.
func RecordRoot(trace, span, name string, start time.Time, took time.Duration, attrs ...Attr) bool {
	ex := current.Load()
	if ex == nil {
		return false
	}
	s := &Span{name: name, kind: 1, start: start, end: start.Add(took), attrs: append([]Attr(nil), attrs...)}
	if len(trace) != 32 || len(span) != 16 {
		return false
	}
	if _, err := hex.Decode(s.trace[:], []byte(trace)); err != nil {
		return false
	}
	if _, err := hex.Decode(s.id[:], []byte(span)); err != nil {
		return false
	}
	ex.add(s)
	return true
}

// Record sends a span measured elsewhere (a page's slow interaction), as
// a child of ctx's span or traceparent, from start for took.
func Record(ctx context.Context, name string, start time.Time, took time.Duration, attrs ...Attr) {
	ex := current.Load()
	if ex == nil {
		return
	}
	s := newSpan(ctx, name, 1, attrs)
	s.start, s.end = start, start.Add(took)
	ex.add(s)
}
