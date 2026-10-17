// Package docs embeds the texts the settings page shows: the reference
// next to its .git-board.toml editor, and how forges work. The same text
// readers find on GitHub.
package docs

import _ "embed"

//go:embed configuration.md
var Configuration string

//go:embed forges.md
var Forges string
