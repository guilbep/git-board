package httpapi

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

// Route 61: every issue as NNN-slug.md, in a zip the browser downloads.
func TestExportMarkdown(t *testing.T) {
	t.Parallel()
	dir, ids := newBoard(t)
	a := newAPI(t, dir)
	res := a.do("GET", "/api/export/markdown", nil)
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "application/zip" ||
		!strings.Contains(res.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("status %d, headers %v", res.StatusCode, res.Header)
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, f := range z.File {
		if !strings.HasSuffix(f.Name, ".md") || strings.Contains(f.Name, "/") {
			t.Errorf("file %q, want NNN-slug.md", f.Name)
		}
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(r)
		r.Close()
		all.Write(b)
	}
	// the third issue is deleted (newBoard), and is no file
	got := all.String()
	if len(z.File) != len(ids)-1 || !strings.Contains(got, "Seen on staging") || strings.Contains(got, "title: Gone") {
		t.Fatalf("%d files, want %d with the comment and without the deleted issue:\n%s", len(z.File), len(ids)-1, got)
	}
}
