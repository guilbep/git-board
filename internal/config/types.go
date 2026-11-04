package config

import (
	"fmt"
	"slices"
	"strings"

	"github.com/guilbep/git-board/core/fields"
)

// checkTypes refuses a type git-board doesn't know.
func checkTypes(defs map[string]fields.Field) error {
	for name, f := range defs {
		if f.Type != "" && !slices.Contains(fields.Types, f.Type) {
			return fmt.Errorf("fields.%s: type %q isn't one of %s", name, f.Type, strings.Join(fields.Types, ", "))
		}
	}
	return nil
}
