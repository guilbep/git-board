package httpapi

import (
	"bytes"
	"regexp"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
	xhtml "golang.org/x/net/html"
)

// An issue's text as GitHub shows it: GitHub-flavored Markdown, line
// breaks kept, then cleaned to what GitHub allows. Issue text is
// untrusted (anyone can comment on a public repository): this is the one
// place it becomes HTML. v1's internal/markdown, copied and owned; its
// media proxy comes with [uploads] (M6), and images keep their URL.

var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM), // tables, task lists, strikethrough, autolinks
	goldmark.WithRendererOptions(
		html.WithHardWraps(), // a newline in an issue comment is a line break, as on GitHub
		html.WithUnsafe(),    // raw HTML (<details>, <img width>) is kept here, cleaned below
	),
)

var policy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowElements("details", "summary")
	p.AllowAttrs("open").OnElements("details")
	p.AllowAttrs("type").Matching(regexp.MustCompile(`^checkbox$`)).OnElements("input")
	p.AllowAttrs("checked", "disabled").OnElements("input")
	p.AllowAttrs("class").Matching(regexp.MustCompile(`^language-[\w+#-]+$`)).OnElements("code")
	p.AddTargetBlankToFullyQualifiedLinks(true)
	return p
}()

// render turns Markdown into clean HTML.
func render(src string) string {
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return xhtml.EscapeString(src)
	}
	return policy.Sanitize(buf.String())
}
