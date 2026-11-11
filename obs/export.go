package obs

import (
	"bytes"
	"cmp"
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// batch and every are when spans are sent: 512 of them, or each second.
const (
	batch  = 512
	every  = time.Second
	buffer = 8192 // spans waiting; more are dropped, counted
)

type exporter struct {
	url     string
	service string
	client  *http.Client
	spans   chan *Span
	dropped atomic.Int64
	sent    atomic.Int64
	flush   chan chan struct{}
	stop    sync.Once
	done    chan struct{}
}

// Endpoint is where spans go, from the environment as the OpenTelemetry
// SDK reads it: OTEL_EXPORTER_OTLP_TRACES_ENDPOINT, whole, else
// OTEL_EXPORTER_OTLP_ENDPOINT with /v1/traces; "" for none.
func Endpoint() string {
	if u := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"); u != "" {
		return u
	}
	if u := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); u != "" {
		return strings.TrimSuffix(u, "/") + "/v1/traces"
	}
	return ""
}

// Init turns tracing on, to url (Endpoint's), spans named for service
// (OTEL_SERVICE_NAME's when set). Its answer sends what waits and turns
// it off. With url "", nothing: tracing stays off.
func Init(url, service string) (shutdown func()) {
	if url == "" {
		return func() {}
	}
	ex := &exporter{url: url, service: cmp.Or(os.Getenv("OTEL_SERVICE_NAME"), service), client: &http.Client{Timeout: 5 * time.Second},
		spans: make(chan *Span, buffer), flush: make(chan chan struct{}), done: make(chan struct{})}
	current.Store(ex)
	go ex.run()
	return func() {
		ex.stop.Do(func() {
			current.CompareAndSwap(ex, nil)
			close(ex.spans)
			<-ex.done
		})
	}
}

// Flush sends the spans waiting, and returns once they are sent.
func Flush() {
	if ex := current.Load(); ex != nil {
		ack := make(chan struct{})
		select {
		case ex.flush <- ack:
			<-ack
		case <-ex.done:
		}
	}
}

// Dropped and Sent count the spans lost to a full buffer or a collector
// that refused them, and those sent.
func Dropped() int64 {
	if ex := current.Load(); ex != nil {
		return ex.dropped.Load()
	}
	return 0
}

func (ex *exporter) add(s *Span) {
	defer func() { recover() }() // a span ended after shutdown: dropped
	select {
	case ex.spans <- s:
	default:
		ex.dropped.Add(1)
	}
}

func (ex *exporter) run() {
	defer close(ex.done)
	t := time.NewTicker(every)
	defer t.Stop()
	var b []*Span
	send := func() {
		if len(b) > 0 {
			ex.send(b)
			b = b[:0]
		}
	}
	for {
		select {
		case s, ok := <-ex.spans:
			if !ok {
				send()
				return
			}
			if b = append(b, s); len(b) >= batch {
				send()
			}
		case <-t.C:
			send()
		case ack := <-ex.flush:
			for len(ex.spans) > 0 {
				b = append(b, <-ex.spans)
			}
			send()
			close(ack)
		}
	}
}

func (ex *exporter) send(spans []*Span) {
	body, err := json.Marshal(ex.payload(spans))
	if err != nil {
		ex.dropped.Add(int64(len(spans)))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ex.url, bytes.NewReader(body))
	if err != nil {
		ex.dropped.Add(int64(len(spans)))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := ex.client.Do(req)
	if err != nil {
		ex.dropped.Add(int64(len(spans)))
		return
	}
	res.Body.Close()
	if res.StatusCode >= 300 {
		ex.dropped.Add(int64(len(spans)))
		return
	}
	ex.sent.Add(int64(len(spans)))
}

// The OTLP/JSON shapes (opentelemetry-proto, its JSON mapping): ids in
// hex, times and integers as strings.
type (
	payload struct {
		ResourceSpans []resourceSpans `json:"resourceSpans"`
	}
	resourceSpans struct {
		Resource   resource     `json:"resource"`
		ScopeSpans []scopeSpans `json:"scopeSpans"`
	}
	resource struct {
		Attributes []keyValue `json:"attributes"`
	}
	scopeSpans struct {
		Scope scope      `json:"scope"`
		Spans []spanJSON `json:"spans"`
	}
	scope struct {
		Name string `json:"name"`
	}
	spanJSON struct {
		TraceID      string      `json:"traceId"`
		SpanID       string      `json:"spanId"`
		ParentSpanID string      `json:"parentSpanId,omitempty"`
		Name         string      `json:"name"`
		Kind         int         `json:"kind"`
		Start        string      `json:"startTimeUnixNano"`
		End          string      `json:"endTimeUnixNano"`
		Attributes   []keyValue  `json:"attributes,omitempty"`
		Events       []eventJSON `json:"events,omitempty"`
		Status       *status     `json:"status,omitempty"`
	}
	eventJSON struct {
		Time       string     `json:"timeUnixNano"`
		Name       string     `json:"name"`
		Attributes []keyValue `json:"attributes,omitempty"`
	}
	status struct {
		Code    int    `json:"code"` // 2: error
		Message string `json:"message,omitempty"`
	}
	keyValue struct {
		Key   string   `json:"key"`
		Value anyValue `json:"value"`
	}
	anyValue struct {
		String *string  `json:"stringValue,omitempty"`
		Int    *string  `json:"intValue,omitempty"`
		Bool   *bool    `json:"boolValue,omitempty"`
		Double *float64 `json:"doubleValue,omitempty"`
	}
)

func (ex *exporter) payload(spans []*Span) payload {
	out := make([]spanJSON, 0, len(spans))
	for _, s := range spans {
		s.mu.Lock()
		j := spanJSON{TraceID: hex.EncodeToString(s.trace[:]), SpanID: hex.EncodeToString(s.id[:]), Name: s.name, Kind: s.kind,
			Start: nanos(s.start), End: nanos(s.end), Attributes: kvs(s.attrs)}
		if s.parent != ([8]byte{}) {
			j.ParentSpanID = hex.EncodeToString(s.parent[:])
		}
		for _, e := range s.events {
			j.Events = append(j.Events, eventJSON{Time: nanos(e.at), Name: e.name, Attributes: kvs(e.attrs)})
		}
		if s.err != "" {
			j.Status = &status{Code: 2, Message: s.err}
		}
		s.mu.Unlock()
		out = append(out, j)
	}
	return payload{[]resourceSpans{{
		Resource:   resource{kvs([]Attr{String("service.name", ex.service)})},
		ScopeSpans: []scopeSpans{{Scope: scope{"git-board"}, Spans: out}},
	}}}
}

func nanos(t time.Time) string { return strconv.FormatInt(t.UnixNano(), 10) }

func kvs(attrs []Attr) []keyValue {
	out := make([]keyValue, 0, len(attrs))
	for _, a := range attrs {
		var v anyValue
		switch x := a.Value.(type) {
		case string:
			v.String = &x
		case int64:
			s := strconv.FormatInt(x, 10)
			v.Int = &s
		case int:
			s := strconv.Itoa(x)
			v.Int = &s
		case bool:
			v.Bool = &x
		case float64:
			v.Double = &x
		default:
			continue
		}
		out = append(out, keyValue{a.Key, v})
	}
	return out
}
