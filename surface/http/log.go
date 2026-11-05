package httpapi

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The server's logs (docs/rewrite/api.md, Logs): log/slog to stderr, a
// line a request at info, never a header, a cookie, a token or a body.

// LogEnv and LogFormatEnv choose the logs when no flag does.
const (
	LogEnv       = "GIT_BOARD_LOG_LEVEL"
	LogFormatEnv = "GIT_BOARD_LOG_FORMAT"
)

// NewLogger is a logger writing to w at level (debug, info, warn, error;
// "" is info) as format (text, json; "" is text).
func NewLogger(w io.Writer, level, format string) (*slog.Logger, error) {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "", "info":
		l = slog.LevelInfo
	case "warn", "warning":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		return nil, fmt.Errorf("log level %q: debug, info, warn or error", level)
	}
	o := &slog.HandlerOptions{Level: l}
	switch strings.ToLower(format) {
	case "", "text":
		return slog.New(slog.NewTextHandler(w, o)), nil
	case "json":
		return slog.New(slog.NewJSONHandler(w, o)), nil
	}
	return nil, fmt.Errorf("log format %q: text or json", format)
}

// Slow is how long a request may take before it is logged as a warning.
var Slow = time.Second

type logKey struct{}

// logOf is the request's logger: the server's.
func logOf(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(logKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.New(slog.DiscardHandler)
}

type clientKey struct{}

// clientOf is the page that sent the request (X-Git-Board-Client): its
// change events carry it.
func clientOf(ctx context.Context) string {
	s, _ := ctx.Value(clientKey{}).(string)
	return s
}

// secretKeys are the query parameters whose values a log never holds.
var secretKeys = []string{"token", "key", "secret", "password", "pass", "auth", "credential"}

// redactQuery is a query string with every secret's value replaced.
func redactQuery(raw string) string {
	q, err := url.ParseQuery(raw)
	if err != nil {
		return "REDACTED"
	}
	for k := range q {
		lk := strings.ToLower(k)
		for _, s := range secretKeys {
			if strings.Contains(lk, s) {
				q[k] = []string{"REDACTED"}
			}
		}
	}
	return q.Encode()
}

// recorder keeps what a response was: its status, its size, the error
// code it answered.
type recorder struct {
	http.ResponseWriter
	status int
	bytes  int
	code   string
}

func (r *recorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// Flush lets the change events stream through.
func (r *recorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *recorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := r.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("not a hijacker")
}

func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// withLog logs each request once it is answered: at info, its method,
// path, status, duration and size; at warn when it took longer than
// Slow; the query string at debug, redacted.
func withLog(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &recorder{ResponseWriter: w}
		ctx := context.WithValue(r.Context(), logKey{}, log)
		if c := r.Header.Get("X-Git-Board-Client"); c != "" {
			ctx = context.WithValue(ctx, clientKey{}, c)
		}
		next.ServeHTTP(rec, r.WithContext(ctx))
		took := time.Since(start)
		attrs := []slog.Attr{slog.String("method", r.Method), slog.String("path", r.URL.Path),
			slog.Int("status", cmpOr(rec.status, http.StatusOK)), slog.Duration("duration", took), slog.Int("bytes", rec.bytes)}
		if rec.code != "" {
			attrs = append(attrs, slog.String("code", rec.code))
		}
		if r.URL.RawQuery != "" && log.Enabled(ctx, slog.LevelDebug) {
			attrs = append(attrs, slog.String("query", redactQuery(r.URL.RawQuery)))
		}
		level, msg := slog.LevelInfo, "request"
		if took > Slow && r.URL.Path != "/api/events" { // a stream lasts
			level, msg = slog.LevelWarn, "slow request"
		}
		log.LogAttrs(ctx, level, msg, attrs...)
	})
}
