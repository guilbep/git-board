package httpapi

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

// An issue's text as GitHub shows it: GitHub-flavored Markdown, line
// breaks kept, then cleaned to what GitHub allows. Issue text is
// untrusted (anyone can comment on a public repository): this is the one
// place it becomes HTML. v1's internal/markdown, copied and owned: a
// forge's pictures go through the server (route 42, app.Media).

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

// render turns Markdown into clean HTML; media, when set, is where a
// picture's address points the page (app.MediaURL).
func render(src string, media func(string) string) string {
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return xhtml.EscapeString(src)
	}
	clean := policy.Sanitize(buf.String())
	if media == nil || !strings.Contains(clean, "<img") {
		return clean
	}
	return rewriteImages(clean, media)
}

// rewriteImages points every <img> of clean HTML at media(src).
func rewriteImages(clean string, media func(string) string) string {
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
