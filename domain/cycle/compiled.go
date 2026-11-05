package cycle

import (
	"regexp"
	"sync"
)

// compiledRes are the regexps compiled from the config's words, by
// their text: the Work page computes every open issue's cycle with the
// same few.
var compiledRes sync.Map

// compiled is regexp.MustCompile(re), compiled once.
func compiled(re string) *regexp.Regexp {
	if r, ok := compiledRes.Load(re); ok {
		return r.(*regexp.Regexp)
	}
	r, _ := compiledRes.LoadOrStore(re, regexp.MustCompile(re))
	return r.(*regexp.Regexp)
}
