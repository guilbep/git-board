// Package web embeds the built Quasar frontend (web/dist) so
// internal/server can serve it as static assets from a single binary
// in production. Run `npm run build` in web/ first to populate dist/ with
// the real app — until then it only contains a .gitkeep placeholder, so the
// embed (and `go build ./...`) still succeeds, just with an empty UI.
package web

import "embed"

//go:embed all:dist
var DistFS embed.FS
