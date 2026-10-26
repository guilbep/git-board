// Package docs embeds the texts the settings page shows: the reference
// next to its .git-board.toml editor, and how forges work. The same text
// readers find on GitHub.
package docs

import _ "embed"

//go:embed guide/configuration.md
var Configuration string

//go:embed guide/forges.md
var Forges string
