package search

import (
	"slices"
	"testing"
)

func TestTokensAsUnicode61(t *testing.T) {
	t.Parallel()
	for text, want := range map[string][]string{
		"Crashes on save":     {"crashes", "on", "save"},
		"Café, déjà-vu!":      {"cafe", "deja", "vu"},
		"title:crash v2.1":    {"title", "crash", "v2", "1"},
		"  -- ":               nil,
		"ÉCOLE polytechnique": {"ecole", "polytechnique"},
	} {
		if got := Tokens(text); !slices.Equal(got, want) {
			t.Errorf("Tokens(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestMatch(t *testing.T) {
	t.Parallel()
	texts := []string{"Crash on save", "The export fails", "Seen on staging\ncafé-crème"}
	for text, want := range map[string]bool{
		"":                   true,
		"-":                  true,
		"crash":              true,
		"cras sav":           true,
		"export crash":       true,
		"on save":            true,
		"save:on":            false, // tokens in a row
		"cafe-cr":            true,  // the last a prefix
		"crash-on":           true,
		"staging crash nope": false,
	} {
		if got := Match(text, texts...); got != want {
			t.Errorf("Match(%q) = %v, want %v", text, got, want)
		}
	}
}
