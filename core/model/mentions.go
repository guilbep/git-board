package model

import (
	"regexp"
	"strings"
)

// mentionRe is an @login (web/src/richtext.ts has it too) not inside an
// email, a path or code, ending on a letter, a digit or _. Its second
// group is the mention, @ included.
var mentionRe = regexp.MustCompile(`(^|[^\w.\-/@` + "`" + `])(@[A-Za-z0-9_](?:[A-Za-z0-9_.\-]*[A-Za-z0-9_])?)`)

// Mentions reports whether text mentions login (@login), ignoring case.
func Mentions(text, login string) bool {
	if login == "" {
		return false
	}
	for _, m := range mentionRe.FindAllStringSubmatch(text, -1) {
		if strings.EqualFold(m[2][1:], login) {
			return true
		}
	}
	return false
}
