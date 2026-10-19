package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/uploads"
)

// fakeS3 keeps the last PUT it got.
type fakeS3 struct {
	path, acl, contentType string
	body                   []byte
}

func newFakeS3(t *testing.T) (*httptest.Server, *fakeS3) {
	f := &fakeS3{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.path, f.acl, f.contentType = r.URL.Path, r.Header.Get("X-Amz-Acl"), r.Header.Get("Content-Type")
		f.body, _ = io.ReadAll(r.Body)
	}))
	t.Cleanup(srv.Close)
	return srv, f
}

func multipartBody(t *testing.T, name string, data []byte) (*bytes.Buffer, string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(data)
	_ = w.Close()
	return &buf, w.FormDataContentType()
}

func upload(t *testing.T, ts *httptest.Server, name string, data []byte, withHeader bool) (int, map[string]string) {
	t.Helper()
	body, ct := multipartBody(t, name, data)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/uploads", body)
	req.Header.Set("Content-Type", ct)
	if withHeader {
		req.Header.Set("X-Git-Board-Upload", "1")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestUploadPutsAPublicFileOnTheCDN(t *testing.T) {
	t.Parallel()
	s3, got := newFakeS3(t)
	ts, s := newIssueServer(t)
	s.store = &uploads.Store{Endpoint: s3.URL, Bucket: "cdn", Region: "us-east-1", AccessKey: "k", SecretKey: "s",
		PublicURL: "https://cdn.example", Prefix: "git-board/co2", ACL: "public-read", Client: http.DefaultClient}
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")

	if code, _ := upload(t, ts, "shot.png", png, false); code != http.StatusForbidden {
		t.Errorf("without the page's header: %d, want 403", code)
	}
	code, out := upload(t, ts, "Screen shot.png", png, true)
	if code != http.StatusOK {
		t.Fatalf("upload: %d %v", code, out)
	}
	if !strings.HasPrefix(got.path, "/cdn/git-board/co2/") || got.acl != "public-read" || got.contentType != "image/png" || !bytes.Equal(got.body, png) {
		t.Errorf("S3 got %s acl %q type %q", got.path, got.acl, got.contentType)
	}
	if !strings.HasPrefix(out["url"], "https://cdn.example/git-board/co2/") || out["markdown"] != "![Screen shot.png]("+out["url"]+")" {
		t.Errorf("answer %v", out)
	}
	if code, _ := upload(t, ts, "logo.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), true); code != http.StatusBadRequest {
		t.Errorf("an SVG: %d, want 400", code)
	}
	var st map[string]any
	call(t, "GET", ts.URL+"/api/uploads", nil, &st)
	if st["enabled"] != true || st["public_url"] != "https://cdn.example" {
		t.Errorf("status %v", st)
	}
}

func TestUploadWithoutABucketSaysHowToSetItUp(t *testing.T) {
	t.Setenv("ACCESS_KEY_ID", "")
	ts, _ := newIssueServer(t)
	if code, out := upload(t, ts, "a.txt", []byte("hi"), true); code != http.StatusNotImplemented || !strings.Contains(out["error"], "BUCKET_NAME") {
		t.Errorf("%d %v, want 501 saying how", code, out)
	}
}
