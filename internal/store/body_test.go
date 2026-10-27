package store

import (
	"reflect"
	"testing"
)

// The vectors of web/src/sections.test.ts: one rule on both sides.

func slugsOf(ss []section) []string {
	var out []string
	for _, s := range ss {
		out = append(out, s.slug)
	}
	return out
}

func TestSectionsLikeTheWeb(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, body string
		want       []string
	}{
		{"no heading is one section", "a\n\nb", []string{"intro"}},
		{"each heading opens a section", "Seen.\n\n## Releases page\n\n- a\n\n## Expected\n\n- 1\n\n## Workaround\n\nNot yet.", []string{"intro", "releases-page", "expected", "workaround"}},
		{"a repeated heading gets an index", "## Expected\n\na\n\n## Expected\n\nb", []string{"expected", "expected-2"}},
		{"a heading's slug is its rendered text", "## [Releases](https://x) page\n\n## A & B", []string{"releases-page", "a-b"}},
		{"a fence is not a heading", "## One\n\n```\n# not one\n```", []string{"one"}},
		{"a setext heading opens a section", "intro\n\nExpected\n---\n\n- 1", []string{"intro", "expected"}},
		{"a quoted heading opens none", "## One\n\n> # Note\n\ntext", []string{"one"}},
	} {
		if got := slugsOf(sectionsOfText(c.body)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestItem(t *testing.T) {
	t.Parallel()
	s := sectionsOfText("## Expected\n\n- [ ] One.\n- [x] Two &amp; *more*.\n  - nested\n- Three")
	if got := []string{s[0].item(1), s[0].item(2), s[0].item(4), s[0].item(5)}; !reflect.DeepEqual(got, []string{"One.", "Two & more.", "Three", ""}) {
		t.Errorf("items %q", got)
	}
}

func TestATickIsNotAVersion(t *testing.T) {
	t.Parallel()
	h := []Version{{ID: "a", Body: "- [ ] x\n- [ ] y"}, {ID: "b", Body: "- [x] x\n- [ ] y"}, {ID: "c", Body: "- [x] x\n- [ ] z"}, {ID: "d", Body: "- [x] x\n- [x] z"}}
	vs := versions(h)
	if len(vs) != 2 || vs[0].ID != "a" || vs[0].Body != h[1].Body || vs[1].ID != "c" || vs[1].Body != h[3].Body {
		t.Errorf("versions %+v", vs)
	}
}
