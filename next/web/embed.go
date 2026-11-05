// Package web embeds git-board v2's web app, next/web/dist, built by
// `npm run build` (make next-web-build): next/surface/http serves its
// dist/spa (ADR 0012). Until a build, dist holds only a .gitkeep, so
// `go build ./...` works and the page is empty.
package web

import "embed"

//go:embed all:dist
var DistFS embed.FS
