package httpapi

import (
	"cmp"
	"context"
	"html/template"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/app"
	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/kernel"
)

// The read-only site (#102): every issue as a static HTML page, and an
// index of them, for GitHub Pages or any web server. The text is rendered
// and cleaned as the web app does it (render): issue text is untrusted.
// Nothing on the pages writes, and no script runs.

var sitePages = template.Must(template.New("site").Parse(`{{define "head"}}<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title><style>
:root { color-scheme: light dark; --fg: #212121; --muted: #636363; --line: #d5d5d5; --bg: #fff; --open: #1a7f37; --closed: #8250df; }
@media (prefers-color-scheme: dark) { :root { --fg: #e6e6e6; --muted: #a0a0a0; --line: #3a3a3a; --bg: #161616; --open: #3fb950; --closed: #a371f7; } }
body { margin: 0 auto; max-width: 60rem; padding: 1rem; font: 16px/1.5 system-ui, sans-serif; color: var(--fg); background: var(--bg); }
a { color: inherit; } table { width: 100%; border-collapse: collapse; } td, th { padding: .375rem .5rem; border-bottom: 1px solid var(--line); text-align: left; vertical-align: top; }
.muted, .when { color: var(--muted); font-size: .875rem; } .open { color: var(--open); } .closed { color: var(--closed); }
.label { display: inline-block; margin: 0 .25rem .25rem 0; padding: 0 .375rem; border: 1px solid var(--line); border-radius: 1rem; font-size: .8125rem; }
.event { margin: 1rem 0; border: 1px solid var(--line); } .event header { padding: .375rem .75rem; border-bottom: 1px solid var(--line); }
.event .text { padding: 0 .75rem; overflow-x: auto; } img { max-width: 100%; } pre { overflow-x: auto; }
</style></head><body>{{end}}
{{define "index"}}{{template "head" .}}
<h1>{{.Title}}</h1>
<p class="muted">{{.Open}} open, {{.Closed}} closed. Written by git-board on {{.At}}: a copy to read, which the board does not read back.</p>
<table><thead><tr><th>#</th><th>Title</th><th>State</th><th>Labels</th><th>Updated</th></tr></thead><tbody>
{{range .Rows}}<tr><td class="muted">{{.Ref}}</td><td><a href="{{.Page}}">{{.Title}}</a></td><td class="{{.Status}}">{{.Status}}</td>
<td>{{range .Labels}}<span class="label">{{.}}</span>{{end}}</td><td class="when">{{.Updated}}</td></tr>
{{end}}</tbody></table></body></html>
{{end}}
{{define "issue"}}{{template "head" .}}
<p><a href="index.html">All issues</a></p>
<h1>{{.Title}} <span class="muted">{{.Ref}}</span></h1>
<p><span class="{{.Status}}">{{.Status}}</span> · opened by {{.Author}} on {{.Created}}{{range .Forges}} · <a href="{{.}}">on the forge</a>{{end}}</p>
<p>{{range .Labels}}<span class="label">{{.}}</span>{{end}}</p>
{{range .Events}}<section class="event"><header><b>{{.Author}}</b> <span class="when">{{.At}}</span></header><div class="text">{{.HTML}}</div></section>
{{end}}</body></html>
{{end}}`))

type siteRow struct {
	Ref, Title, Status, Page, Updated string
	Labels                            []string
}

type siteEvent struct {
	Author, At string
	HTML       template.HTML // render's output: cleaned
}

// WriteSite writes the board's issues as a static site in dir: index.html
// and one page a visible issue, NNN.html by its number, else by its id's
// first 7 digits. It answers the files it wrote. title names the site.
func WriteSite(ctx context.Context, b *app.Board, dir, title string) ([]string, error) {
	svc := b.Issues
	ctx = svc.Board.Snapshot(ctx)
	heads, _, err := svc.Heads(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]kernel.ID, len(heads))
	for i, h := range heads {
		ids[i] = kernel.ID(h.ID)
	}
	ths, err := svc.Threads(ctx, ids)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	// open first, then the last changed
	slices.SortStableFunc(ths, func(x, y issues.Thread) int {
		if x.Status != y.Status {
			return strings.Compare(y.Status, x.Status) // "open" before "closed"
		}
		return y.UpdatedAt.Compare(x.UpdatedAt)
	})
	day := func(t time.Time) string { return t.UTC().Format("2006-01-02") }
	write := func(name, page string, data any) error {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		if err := sitePages.ExecuteTemplate(f, page, data); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	}
	var written []string
	var rows []siteRow
	open := 0
	for _, th := range ths {
		ref := cmp.Or(th.Number, th.ID[:min(7, len(th.ID))])
		page := filepath.Base(ref) + ".html"
		row := siteRow{Ref: ref, Title: cmp.Or(th.Shown, th.Title), Status: th.Status, Page: page, Updated: day(th.UpdatedAt), Labels: th.Labels}
		rows = append(rows, row)
		if th.Status == "open" {
			open++
		}
		var evs []siteEvent
		for _, e := range th.Events {
			if e.Kind != "created" && e.Kind != "comment" {
				continue
			}
			evs = append(evs, siteEvent{Author: cmp.Or(e.Author.Name, e.Author.Login), At: e.At.UTC().Format("2006-01-02 15:04 UTC"),
				HTML: template.HTML(render(e.Body, nil))}) // #nosec: render cleans it
		}
		var forges []string
		for _, f := range th.Forges {
			if strings.HasPrefix(f.URL, "https://") {
				forges = append(forges, f.URL)
			}
		}
		err := write(page, "issue", map[string]any{
			"Title": row.Title, "Ref": ref, "Status": th.Status, "Labels": th.Labels,
			"Author": cmp.Or(th.Author.Name, th.Author.Login), "Created": day(th.CreatedAt), "Events": evs, "Forges": forges,
		})
		if err != nil {
			return written, err
		}
		written = append(written, page)
	}
	err = write("index.html", "index", map[string]any{
		"Title": title, "Rows": rows, "Open": open, "Closed": len(rows) - open, "At": day(time.Now()),
	})
	if err != nil {
		return written, err
	}
	return append(written, "index.html"), nil
}
