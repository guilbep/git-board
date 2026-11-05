// Package search is how a free text finds an entity: its words, cut into
// tokens as sqlite's FTS5 unicode61 tokenizer cuts them, each matching
// in a row, the last as a prefix (docs/spec/queries.md, Text). The view
// searches so in SQL; Match searches texts held in memory the same way,
// without it.
package search

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Texts are an issue's texts a search can look in: its title, its body,
// its comments (the index's columns, in order).
var Texts = []string{"title", "body", "comments"}

// Match reports whether texts hold text as the index finds it: each
// word's tokens in a row in one of them, the last as a prefix, every word
// somewhere. A word with no token is left out; a text with no word left
// matches. The comments are one text, joined by a line feed, as the index
// keeps them. Each text is cut into tokens once.
func Match(text string, texts ...string) bool {
	var docs [][]string
	for _, w := range strings.Fields(text) {
		toks := Tokens(w)
		if len(toks) == 0 {
			continue
		}
		if docs == nil {
			docs = make([][]string, len(texts))
			for i, t := range texts {
				docs[i] = Tokens(t)
			}
		}
		if !slices.ContainsFunc(docs, func(doc []string) bool { return holds(doc, toks) }) {
			return false
		}
	}
	return true
}

// holds tells doc has toks in a row, the last as a prefix.
func holds(doc, toks []string) bool {
	last := len(toks) - 1
	for i := 0; i+last < len(doc); i++ {
		if strings.HasPrefix(doc[i+last], toks[last]) && slices.Equal(doc[i:i+last], toks[:last]) {
			return true
		}
	}
	return false
}

// Tokens are text's tokens as unicode61 cuts them: lower case, without
// diacritics, each run of letters, numbers and private use characters.
func Tokens(text string) []string {
	if !ascii(text) {
		if folded, _, err := transform.String(fold(), text); err == nil {
			text = folded
		}
	}
	text = strings.ToLower(text)
	var toks []string
	start := -1
	for i, r := range text {
		switch {
		case IsToken(r):
			if start < 0 {
				start = i
			}
		case start >= 0:
			toks = append(toks, text[start:i])
			start = -1
		}
	}
	if start >= 0 {
		toks = append(toks, text[start:])
	}
	return toks
}

// IsToken says r belongs in a token: a letter, a number, a private use
// character.
func IsToken(r rune) bool {
	if r < utf8.RuneSelf {
		return 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9'
	}
	return unicode.In(r, unicode.L, unicode.N, unicode.Co)
}

// ascii says text has no diacritic to take off.
func ascii(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// fold takes the diacritics off, as unicode61's remove_diacritics does. A
// chain keeps state between calls: one per call, never shared between
// goroutines (two refreshes at once tore a shared one).
func fold() transform.Transformer {
	return transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
}
