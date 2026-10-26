package web

import (
	_ "embed"
	"strings"
)

// The EPFL design system for pages built outside the Quasar app — the
// static site of `git-board html`, which inlines it so the site works from
// a plain rsync target. Same vendored copy (epfl-design/, see README.md)
// and logo (public/git-board.svg) the web UI uses: one source.

//go:embed epfl-design/tokens/colors_and_type.css
var epflTokens string

//go:embed epfl-design/tokens/fonts.css
var epflFonts string

//go:embed epfl-design/css/organisms/header.css
var epflHeader string

//go:embed epfl-design/css/templates/layout.css
var epflLayout string

// LogoSVG is git-board's logo, as served to the web UI.
//
//go:embed public/git-board.svg
var LogoSVG []byte

// EPFLCSS is the token layer (with Suisse Int'l's @font-face rules, which
// load from enac-it-fonts.epfl.ch) plus the header and layout classes
// (.epfl-header, .epfl-nav, .epfl-breadcrumb, .epfl-eyebrow). The tokens'
// relative @import "fonts.css" is replaced by that file's content, since a
// page-inlined <style> can't resolve a relative import.
var EPFLCSS = strings.Replace(epflTokens, `@import "fonts.css";`, epflFonts, 1) + epflHeader + epflLayout
