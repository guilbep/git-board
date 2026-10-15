package main

import (
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// devOrigins are the Quasar/Vite dev server's default origin (see
// web/quasar.config.ts — no devServer.port override, so @quasar/app-vite's
// default of 9000 applies). Once the frontend is embedded and served
// same-origin in production, no browser ever sends a cross-origin request
// here, so this list only matters in dev.
var devOrigins = map[string]bool{
	"http://localhost:9000": true,
	"http://127.0.0.1:9000": true,
}

// withCORS allows the dev frontend to call this server from a different
// origin/port. Must run before mux dispatch: Go's http.ServeMux answers an
// OPTIONS preflight for a registered "PATCH /api/issues/{id}" pattern with a
// bare 405, so the preflight is answered here instead.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if devOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Git-Board-Upload")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// withGuard refuses the writes another site could make the user's browser
// send: the server writes to git, and through the forge sync to GitHub and
// GitLab with the user's tokens. A browser sends the Origin of the page
// behind a request, and a cross-site request can only be JSON after a CORS
// preflight, which only the dev server passes. Clients without an Origin
// (curl, scripts) are the user's own.
//
// Every request, a read too, must name this machine: a page can point its
// own domain at 127.0.0.1 ("DNS rebinding"), and its requests then carry
// Host and Origin of that domain, which agree. extraHosts are the names
// --addr exposes beyond the loopback ones; "*" serves any.
func withGuard(next http.Handler, extraHosts []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !servedHost(r.Host, extraHosts) {
			writeError(w, http.StatusForbidden, errForeignHost)
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !devOrigins[origin] && !sameHost(origin, r.Host) {
			writeError(w, http.StatusForbidden, errForbiddenOrigin)
			return
		}
		mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		switch {
		case r.URL.Path == "/api/uploads" && mt == "multipart/form-data":
			// a file can't be JSON: the page's header instead, which another
			// site can't send without a preflight only the dev server passes
			if r.Header.Get("X-Git-Board-Upload") != "1" {
				writeError(w, http.StatusForbidden, errForbiddenOrigin)
				return
			}
		case err != nil || mt != "application/json":
			writeError(w, http.StatusUnsupportedMediaType, errNotJSON)
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
func servedHost(hostport string, extra []string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	for _, h := range extra {
		if h == "*" || h == host {
			return true
		}
	}
	return false
}
