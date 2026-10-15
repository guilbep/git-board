// Package docs embeds the reference the settings page shows next to its
// .git-board.toml editor: the same text readers find on GitHub.
package docs

import _ "embed"

//go:embed configuration.md
var Configuration string
