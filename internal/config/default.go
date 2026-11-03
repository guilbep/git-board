package config

import (
	"embed"
	_ "embed"
	"fmt"
	"strings"
)

// Default is the project file a repository starts from: what `git board
// init` writes, and what Settings › Project file offers when there is none.
//
//go:embed default.git-board.toml
var Default string

//go:embed templates/*.git-board.toml
var templates embed.FS

// Template is a built-in template by name (enac): templates/<name>.git-board.toml.
func Template(name string) (string, error) {
	b, err := templates.ReadFile("templates/" + name + ".git-board.toml")
	if err != nil || strings.ContainsAny(name, "/.") {
		return "", fmt.Errorf("no built-in template %q (there is enac)", name)
	}
	return string(b), nil
}
