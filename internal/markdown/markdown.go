// Package markdown renders issue text as GitHub shows it — GitHub-flavored
// Markdown, line breaks kept — then cleans the HTML to what GitHub allows.
// Issue text is untrusted (anyone can comment on a public repository): this
// is the one place it becomes HTML.
package markdown

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
	xhtml "golang.org/x/net/html"
)

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

// Render turns Markdown into clean HTML. media maps an image's URL to the
// one the page loads it from (the server's image proxy).
func Render(src string, media func(url string) string) string {
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return xhtml.EscapeString(src)
	}
	return rewriteImages(policy.Sanitize(buf.String()), media)
}

// rewriteImages points every <img> of clean HTML at media(src).
func rewriteImages(clean string, media func(string) string) string {
	if !strings.Contains(clean, "<img") {
		return clean
	}
	var out strings.Builder
	z := xhtml.NewTokenizer(strings.NewReader(clean))
	for {
		switch z.Next() {
		case xhtml.ErrorToken:
			return out.String()
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			tok := z.Token()
			if tok.Data == "img" {
				for i, a := range tok.Attr {
					if a.Key == "src" {
						tok.Attr[i].Val = media(a.Val)
					}
				}
			}
			out.WriteString(tok.String())
		default:
			out.Write(z.Raw())
		}
	}
}
