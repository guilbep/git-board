// Package fields applies field=value assignments as namespaced git-bug
// labels, via internal/gitbugio. Single-cardinality fields (the default)
// swap: any existing "field:*" label is removed before the new one is
// added. Multi-cardinality fields (assignee) add by default; a leading "-"
// removes instead (e.g. assignee=-guilbep).
package fields

import (
	"fmt"
	"strings"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// Backend is the subset of gitbugio.Backend that field application needs.
type Backend interface {
	GetLabels(idPrefix string) ([]string, error)
	ApplyLabelChange(idPrefix string, added, removed []string) ([]string, error)
}

var _ Backend = (*gitbugio.Backend)(nil)

// Apply sets fieldName to value on the issue identified by idPrefix.
// Returns the resulting label (e.g. "status:ready").
func Apply(b Backend, cfg *config.Config, idPrefix, fieldName, value, me string, today time.Time) (string, error) {
	fieldCfg, known := cfg.Fields[fieldName]
	single := !known || fieldCfg.Single

	resolved, err := resolveAssignedValue(cfg, value, me, today)
	if err != nil {
		return "", err
	}

	current, err := b.GetLabels(idPrefix)
	if err != nil {
		return "", err
	}
	currentSet := make(map[string]bool, len(current))
	for _, l := range current {
		currentSet[l] = true
	}

	if single {
		newLabel := fieldName + ":" + resolved
		var removed []string
		for _, l := range current {
			if strings.HasPrefix(l, fieldName+":") && l != newLabel {
				removed = append(removed, l)
			}
		}
		var added []string
		if !currentSet[newLabel] {
			added = []string{newLabel}
		}
		if _, err := b.ApplyLabelChange(idPrefix, added, removed); err != nil {
			return "", err
		}
		return newLabel, nil
	}

	remove := strings.HasPrefix(resolved, "-")
	target := strings.TrimLeft(resolved, "+-")
	newLabel := fieldName + ":" + target

	var added, removed []string
	switch {
	case remove && currentSet[newLabel]:
		removed = []string{newLabel}
	case !remove && !currentSet[newLabel]:
		added = []string{newLabel}
	}
	if _, err := b.ApplyLabelChange(idPrefix, added, removed); err != nil {
		return "", err
	}
	return newLabel, nil
}

func resolveAssignedValue(cfg *config.Config, value, me string, today time.Time) (string, error) {
	switch {
	case value == "@current" || value == "@next" || value == "@previous":
		sprint, err := config.ResolveSprint(cfg.Sprints, value[1:], today)
		if err != nil {
			return "", err
		}
		if sprint == nil {
			return "", fmt.Errorf("no sprint resolves to %s", value)
		}
		return sprint.Slug, nil
	case value == "@me":
		if me == "" {
			return "", fmt.Errorf("@me used but no current user configured")
		}
		return me, nil
	default:
		return value, nil
	}
}
