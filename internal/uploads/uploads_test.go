package uploads

import (
	"bytes"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// AWS's own example of a signed S3 request ("Example: GET Object",
// Signature Version 4 docs): the signature must come out the same.
func TestSignMatchesAWSExample(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	req.Header.Set("Range", "bytes=0-9")
	emptyHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	sign(req, emptyHash, "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "us-east-1",
		time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))
	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request," +
		"SignedHeaders=host;range;x-amz-content-sha256;x-amz-date," +
		"Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := req.Header.Get("Authorization"); got != want {
		t.Errorf("Authorization\n got %s\nwant %s", got, want)
	}
}

func TestCheckTakesWhatForgesShow(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	for name, c := range map[string]struct {
		data []byte
		ok   bool
		typ  string
	}{
		"shot.png":   {png, true, "image/png"},
		"notes.txt":  {[]byte("plain text"), true, "text/plain; charset=utf-8"},
		"data.csv":   {[]byte("a,b\n1,2\n"), true, "text/csv"},
		"logo.svg":   {[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>x</script></svg>`), false, ""},
		"page.html":  {[]byte("<html><script>x</script></html>"), false, ""},
		"fake.png":   {[]byte("<html><script>x</script></html>"), false, ""}, // the bytes decide
		"run.sh":     {[]byte("#!/bin/sh\nrm -rf /"), false, ""},
	} {
		typ, err := Check(name, c.data)
		if (err == nil) != c.ok || (c.ok && typ != c.typ) {
			t.Errorf("%s: %q, %v; want ok=%v %q", name, typ, err, c.ok, c.typ)
		}
	}
	if _, err := Check("big.png", append(png, make([]byte, MaxBytes)...)); err == nil {
		t.Error("a file over the limit was taken")
	}
}

func TestKeyIsUnguessableAndReadable(t *testing.T) {
	a := key("git-board/co2", "Screen Shot 2026-10-15 à 12.00.png", time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC))
	b := key("git-board/co2", "Screen Shot 2026-10-15 à 12.00.png", time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC))
	if a == b || !strings.HasPrefix(a, "git-board/co2/2026-10/") || !strings.HasSuffix(a, "/Screen-Shot-2026-10-15-a-12.00.png") {
		t.Errorf("keys %q %q", a, b)
	}
}

// Against a real S3 (MinIO in Docker): GIT_BOARD_S3_TEST=http://127.0.0.1:19000
// with minioadmin/minioadmin and a bucket "test".
func TestPutOnARealS3(t *testing.T) {
	endpoint := os.Getenv("GIT_BOARD_S3_TEST")
	if endpoint == "" {
		t.Skip("GIT_BOARD_S3_TEST not set")
	}
	s := &Store{Endpoint: endpoint, Region: "us-east-1", Bucket: "test", AccessKey: "minioadmin", SecretKey: "minioadmin",
		PublicURL: endpoint + "/test", Prefix: "git-board/it", ACL: "public-read", Client: http.DefaultClient}
	data := []byte("hello from git-board")
	url, err := s.Put(t.Context(), "hello.txt", data, "text/plain; charset=utf-8")
	if err != nil {
		t.Fatal(err)
	}
	// read it back, signed (MinIO's bucket isn't public)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	sign(req, emptySHA256, "minioadmin", "minioadmin", "us-east-1", time.Now())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got bytes.Buffer
	_, _ = got.ReadFrom(resp.Body)
	if resp.StatusCode != http.StatusOK || got.String() != string(data) {
		t.Errorf("GET %s: %d %q", url, resp.StatusCode, got.String())
	}
}
