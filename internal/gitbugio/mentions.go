package gitbugio

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

// Mentioning is the issues whose description or a comment mentions login,
// by id: the full-text index narrows (it finds the login anywhere, in an
// email too), the mention pattern confirms on their text.
func (b *Backend) Mentioning(login string) (map[string]bool, error) {
	res := map[string]bool{}
	if login == "" {
		return res, nil
	}
	hits, err := b.SearchText(login)
	if err != nil {
		return nil, err
	}
	for id := range hits {
		is, _, err := b.loadIssue(id)
		if err != nil {
			return nil, err
		}
		mentions := Mentions(is.Body, login)
		for _, c := range is.Comments {
			mentions = mentions || Mentions(c.Body, login)
		}
		if mentions {
			res[id] = true
		}
	}
	return res, nil
}
