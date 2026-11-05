package httpapi

import (
	"errors"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

// The checks of ADR 0013, ported from v1's internal/server/cors.go: the
// server has no login, so only this machine's own pages may reach it.
// Every request must name this machine in its Host (DNS rebinding); a
// write must come from this server's own origin, and be JSON, which a
// page of another site can't send without a preflight only the dev
// server passes. Clients without an Origin (curl, scripts) are the
// user's own.

// devOrigins are the web app's dev server (web/quasar.config.ts: Vite's
// default port, 9000).
var devOrigins = map[string]bool{
	"http://localhost:9000": true,
	"http://127.0.0.1:9000": true,
}

var (
	errForeignHost   = errors.New("this server answers its own machine only: the request's Host names another")
	errForeignOrigin = errors.New("a write from another site's page is refused")
	errNotJSON       = errors.New("a write's body is JSON: Content-Type: application/json")
)

func forbidden(err error) error {
	return &coded{code: "forbidden", status: http.StatusForbidden, err: err}
}

// withCORS lets the dev web app call this server from its own port, and
// answers its preflights: http.ServeMux would answer one with a bare 405.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if devOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Expose-Headers", "ETag, Location")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, If-Match, If-None-Match, X-Git-Board-Upload, X-Git-Board-Client")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// withGuard refuses what another site could make the user's browser
// send, and logs it as a warning. exposed are the names --addr exposes
// beyond the loopback ones; "*" is every interface: an IP address and
// this machine's own name, never another name.
func withGuard(log *slog.Logger, exposed []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refuse := func(err error, attrs ...slog.Attr) {
			log.LogAttrs(r.Context(), slog.LevelWarn, "refused", append([]slog.Attr{slog.String("method", r.Method),
				slog.String("path", r.URL.Path), slog.String("why", err.Error())}, attrs...)...)
			writeError(w, r, nil, err)
		}
		if !servedHost(r.Host, exposed) {
			refuse(forbidden(errForeignHost), slog.String("host", r.Host))
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !devOrigins[origin] && !sameHost(origin, r.Host) {
			refuse(forbidden(errForeignOrigin), slog.String("origin", origin))
			return
		}
		mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		switch {
		case r.URL.Path == "/api/uploads" && mt == "multipart/form-data":
			// a file can't be JSON: the page's header instead, which another
			// site can't send without a preflight
			if r.Header.Get("X-Git-Board-Upload") != "1" {
				refuse(forbidden(errForeignOrigin))
				return
			}
		case r.ContentLength == 0 && r.Header.Get("Content-Type") == "" && r.Header.Get("Origin") == "":
			// no body, no origin: a script's POST, which a page can't send
			// without an Origin
		case err != nil || mt != "application/json":
			writeError(w, r, nil, &coded{code: "not-json", status: http.StatusUnsupportedMediaType, err: errNotJSON})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// loopbackOnly answers next on a loopback address alone: what reaches
// another datastore, or this machine's files (ADR 0013, Consequences).
func loopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if ip := net.ParseIP(host); err != nil || ip == nil || !ip.IsLoopback() {
			writeError(w, r, logOf(r.Context()), forbidden(errors.New("this answers on localhost only")))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func sameHost(origin, host string) bool {
	u, err := url.Parse(origin)
	return err == nil && u.Host == host
}

// servedHost tells whether a request's Host names this server.
func servedHost(hostport string, exposed []string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	for _, h := range exposed {
		if h == host || h == "*" && (net.ParseIP(host) != nil || machineName(host)) {
			return true
		}
	}
	return false
}

var hostname = sync.OnceValue(func() string {
	h, _ := os.Hostname()
	return strings.ToLower(h)
})

// machineName says whether host is this machine's name: the system's,
// its first label, or that with .local (mDNS).
func machineName(host string) bool {
	name := hostname()
	if name == "" {
		return false
	}
	short, _, _ := strings.Cut(name, ".")
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	return host == name || host == short || host == short+".local"
}
