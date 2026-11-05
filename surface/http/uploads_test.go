package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/guilbep/git-board/app"
)

// post sends a file as the page does: multipart, X-Git-Board-Upload.
func (a *api) post(name string, data []byte, header bool) *http.Response {
	a.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", name)
	_, _ = fw.Write(data)
	_ = mw.Close()
	req, _ := http.NewRequest("POST", a.srv.URL+"/api/uploads", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if header {
		req.Header.Set("X-Git-Board-Upload", "1")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	a.t.Cleanup(func() { res.Body.Close() })
	return res
}

// Uploads: off without a bucket; with one (a fake S3), a picture put on
// it and its Markdown answered; a kind no issue carries refused. Serial:
// it swaps app.UploadStore.
func TestUploads(t *testing.T) {
	dir, _ := newBoard(t)
	a := newAPI(t, dir)
	if st := want[app.UploadStatus](t, a.do("GET", "/api/uploads", nil), 200); st.Enabled {
		t.Fatalf("GET /api/uploads without keys = %+v", st)
	}
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	if e := want[apiError](t, a.post("shot.png", png, true), 409); e.Code != "missing" {
		t.Fatalf("an upload without a bucket = %+v", e)
	}

	var mu sync.Mutex
	put := map[string]int{}
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method != http.MethodPut || r.Header.Get("Authorization") == "" {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		put[r.URL.Path] = int(r.ContentLength)
	}))
	defer s3.Close()
	old := app.UploadStore
	app.UploadStore = &app.UploadBucket{Endpoint: s3.URL, Region: "us-east-1", Bucket: "b", AccessKey: "k", SecretKey: "s",
		PublicURL: "https://cdn.test", Prefix: "git-board/r", ACL: "public-read", Client: s3.Client()}
	t.Cleanup(func() { app.UploadStore = old })

	if st := want[app.UploadStatus](t, a.do("GET", "/api/uploads", nil), 200); !st.Enabled || st.Where != "https://cdn.test/git-board/r" {
		t.Fatalf("GET /api/uploads with a bucket = %+v", st)
	}
	up := want[app.Uploaded](t, a.post("my [shot].png", png, true), 200)
	if !strings.HasPrefix(up.URL, "https://cdn.test/git-board/r/") || up.Type != "image/png" || !strings.HasPrefix(up.Markdown, "![my (shot).png](https://cdn.test/") {
		t.Fatalf("POST /api/uploads = %+v", up)
	}
	if len(put) != 1 {
		t.Fatalf("the bucket got %v, want one PUT", put)
	}
	if e := want[apiError](t, a.post("run.exe", []byte("MZ\x90\x00binary"), true), 400); e.Code != "invalid" {
		t.Fatalf("an .exe = %+v", e)
	}
	// without the page's header, a form from another site: refused
	if res := a.post("shot.png", png, false); res.StatusCode < 400 {
		t.Fatalf("an upload without X-Git-Board-Upload: %d", res.StatusCode)
	}
}

// A forge's picture in issue text goes through the server: the thread's
// HTML points at /api/media, which answers a kept picture without the
// network, and refuses an address that isn't a forge's.
func TestMedia(t *testing.T) {
	t.Parallel()
	dir, ids := newBoard(t)
	a := newAPI(t, dir)
	pic := "https://github.com/user-attachments/assets/1234"
	th := want[threadBody](t, a.do("PATCH", "/api/issues/"+string(ids[1]), map[string]any{"comment": "![shot](" + pic + ") and ![elsewhere](https://example.com/a.png)", "status": "closed"}), 200)
	html := ""
	for _, e := range th.Events {
		html += e.HTML
	}
	proxied := "/api/media?url=" + url.QueryEscape(pic)
	if !strings.Contains(html, `src="`+strings.ReplaceAll(proxied, "&", "&amp;")+`"`) || !strings.Contains(html, `src="https://example.com/a.png"`) {
		t.Fatalf("the thread's HTML: %s", html)
	}
	// kept beside the board: answered from there
	gitDir := strings.TrimSpace(gitRun(t, dir, "rev-parse", "--git-common-dir"))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	sum := sha256.Sum256([]byte(pic))
	kept := filepath.Join(gitDir, "board", "v2", "media", hex.EncodeToString(sum[:]))
	if err := os.MkdirAll(filepath.Dir(kept), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kept, []byte("image/png\n\x89PNG"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := a.do("GET", proxied, nil)
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/png" || string(body) != "\x89PNG" || res.Header.Get("Content-Security-Policy") == "" {
		t.Fatalf("GET %s: %d %s %q", proxied, res.StatusCode, res.Header.Get("Content-Type"), body)
	}
	if e := want[apiError](t, a.do("GET", "/api/media?url="+url.QueryEscape("https://example.com/a.png"), nil), 400); e.Field != "url" {
		t.Fatalf("a picture not a forge's = %+v", e)
	}
}
