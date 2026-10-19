package config

import _ "embed"

// Default is the project file a repository starts from: what `git board
// init` writes, and what Settings › Project file offers when there is none.
//
//go:embed default.git-board.toml
var Default string
