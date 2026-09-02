package main

import "net/http"

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
			w.Header().Set("Access-Control-Allow-Methods", "GET, PATCH, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
