package httpapi

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

// exportMarkdown (route 61) answers every issue as Markdown, one
// NNN-slug.md each, in a zip: git board export --md, downloaded from the
// web app. The files are written in a folder of their own first, removed
// once sent.
func exportMarkdown(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dir, err := os.MkdirTemp("", "git-board-export-")
	if err != nil {
		writeError(w, r, logOf(ctx), err)
		return
	}
	defer os.RemoveAll(dir)
	if _, err := board(ctx).ExportMarkdown(ctx, dir); err != nil {
		writeError(w, r, logOf(ctx), err)
		return
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		writeError(w, r, logOf(ctx), err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="issues.zip"`)
	z := zip.NewWriter(w)
	for _, f := range files {
		if err := addFile(z, filepath.Join(dir, f.Name())); err != nil {
			logOf(ctx).Warn("exporting Markdown", "file", f.Name(), "error", err)
			return // the zip is cut short: the download fails
		}
	}
	if err := z.Close(); err != nil {
		logOf(ctx).Warn("exporting Markdown", "error", err)
	}
}

// addFile puts the file at path in z, under its base name.
func addFile(z *zip.Writer, path string) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := z.Create(filepath.Base(path))
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	return err
}
