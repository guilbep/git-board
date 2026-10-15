// Package uploads puts the files attached to issue text on S3, where a CDN
// serves them to everyone who reads the issue — in git-board, on GitHub,
// on GitLab. EPFL ENAC's CDN by default: enacit4r-cdn.epfl.ch serves the
// bucket on s3.epfl.ch, public but not browsable, so a file's address
// carries a random part. Requests are signed with AWS Signature V4, in a
// few lines of the standard library rather than an SDK.
package uploads

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/guilbep/git-board/internal/config"
)

// MaxBytes caps a file: GitHub's limit for attachments.
const MaxBytes = 25 << 20

const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// Store is an S3 bucket a CDN serves.
type Store struct {
	Endpoint, Region, Bucket string
	AccessKey, SecretKey     string
	PublicURL, Prefix        string
	ACL                      string // public-read: the CDN serves public objects
	Client                   *http.Client
}

// FromEnv is the store of ACCESS_KEY_ID, ACCESS_KEY_SECRET and BUCKET_NAME,
// with s's addresses or EPFL's; false without the keys.
func FromEnv(s config.Uploads, repo string) (*Store, bool) {
	st := &Store{
		AccessKey: os.Getenv("ACCESS_KEY_ID"),
		SecretKey: os.Getenv("ACCESS_KEY_SECRET"),
		Bucket:    os.Getenv("BUCKET_NAME"),
		Endpoint:  or(s.Endpoint, "https://s3.epfl.ch"),
		PublicURL: strings.TrimSuffix(or(s.PublicURL, "https://enacit4r-cdn.epfl.ch"), "/"),
		Prefix:    strings.Trim(or(s.Prefix, "git-board/"+safeName(repo)), "/"),
		Region:    or(s.Region, "us-east-1"),
		ACL:       "public-read",
		Client:    &http.Client{Timeout: 2 * time.Minute},
	}
	return st, st.AccessKey != "" && st.SecretKey != "" && st.Bucket != ""
}

func or(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// Put stores a file and returns its public address.
func (s *Store) Put(ctx context.Context, name string, data []byte, contentType string) (string, error) {
	k := key(s.Prefix, name, time.Now())
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, strings.TrimSuffix(s.Endpoint, "/")+"/"+s.Bucket+"/"+k, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", contentType)
	if s.ACL != "" {
		req.Header.Set("X-Amz-Acl", s.ACL)
	}
	sum := sha256.Sum256(data)
	sign(req, hex.EncodeToString(sum[:]), s.AccessKey, s.SecretKey, s.Region, time.Now())
	resp, err := s.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("S3 answered %s: %s", resp.Status, s3Error(body))
	}
	return s.PublicURL + "/" + k, nil
}

var s3Code = regexp.MustCompile(`<(Code|Message)>([^<]*)</`)

// s3Error is the code and message of S3's XML error, not the whole body.
func s3Error(body []byte) string {
	var parts []string
	for _, m := range s3Code.FindAllSubmatch(body, 2) {
		parts = append(parts, string(m[2]))
	}
	if len(parts) == 0 {
		return strings.TrimSpace(string(body))
	}
	return strings.Join(parts, ": ")
}

// Check returns a file's type when it's one a forge shows or links: images
// (not SVG, which can carry scripts), PDF, video, text, zip and office
// files. The bytes decide, then the name for text and zip kinds.
func Check(name string, data []byte) (string, error) {
	if len(data) > MaxBytes {
		return "", fmt.Errorf("over %d MB", MaxBytes>>20)
	}
	detected := http.DetectContentType(data)
	ext := strings.ToLower(path.Ext(name))
	switch {
	case detected == "image/png", detected == "image/jpeg", detected == "image/gif", detected == "image/webp",
		detected == "application/pdf", detected == "video/mp4", detected == "video/webm":
		return detected, nil
	case strings.HasPrefix(detected, "text/plain"):
		if t, ok := textTypes[ext]; ok {
			return t, nil
		}
	case detected == "application/zip":
		if t, ok := zipTypes[ext]; ok {
			return t, nil
		}
	}
	return "", fmt.Errorf("%s files can't be attached (images, PDF, video, text, CSV, zip and office files can)", strings.TrimPrefix(or(ext, "these"), "."))
}

var textTypes = map[string]string{
	".txt": "text/plain; charset=utf-8", ".log": "text/plain; charset=utf-8", ".md": "text/markdown; charset=utf-8",
	".csv": "text/csv", ".json": "application/json", ".yaml": "text/yaml", ".yml": "text/yaml",
}

var zipTypes = map[string]string{
	".zip":  "application/zip",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
}

// key is where a file goes: prefix/2026-10/<random>/<name>. The random
// part keeps a file of a not-browsable CDN from being guessed; the name
// stays readable.
func key(prefix, name string, now time.Time) string {
	r := make([]byte, 8)
	_, _ = rand.Read(r)
	return path.Join(prefix, now.Format("2006-01"), hex.EncodeToString(r), safeName(name))
}

var unsafeRun = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// safeName keeps a file name to letters, digits, dots, dashes: accents
// dropped from their letter, the rest a dash.
func safeName(name string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(path.Base(name)) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	s := strings.Trim(unsafeRun.ReplaceAllString(b.String(), "-"), "-.")
	if s == "" {
		return "file"
	}
	return s
}

// sign adds AWS Signature V4 headers to req for S3: payloadHash is the
// body's SHA-256 in hex. Signed: host, range, content-type and x-amz-*.
func sign(req *http.Request, payloadHash, accessKey, secretKey, region string, now time.Time) {
	amzDate := now.UTC().Format("20060102T150405Z")
	day := amzDate[:8]
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	headers := map[string]string{"host": host}
	for k, v := range req.Header {
		lk := strings.ToLower(k)
		if lk == "range" || lk == "content-type" || strings.HasPrefix(lk, "x-amz-") {
			headers[lk] = strings.TrimSpace(strings.Join(v, ","))
		}
	}
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	var canonHeaders strings.Builder
	for _, k := range names {
		canonHeaders.WriteString(k + ":" + headers[k] + "\n")
	}
	signed := strings.Join(names, ";")

	uri := req.URL.EscapedPath()
	if uri == "" {
		uri = "/"
	}
	canonical := strings.Join([]string{req.Method, uri, req.URL.RawQuery, canonHeaders.String(), signed, payloadHash}, "\n")
	scope := day + "/" + region + "/s3/aws4_request"
	sum := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(sum[:])

	k := hmacSHA256([]byte("AWS4"+secretKey), day)
	for _, part := range []string{region, "s3", "aws4_request"} {
		k = hmacSHA256(k, part)
	}
	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s,SignedHeaders=%s,Signature=%s",
		accessKey, scope, signed, hex.EncodeToString(hmacSHA256(k, toSign))))
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// ErrDisabled is returned when no bucket is configured.
var ErrDisabled = errors.New("uploads aren't set up: set ACCESS_KEY_ID, ACCESS_KEY_SECRET and BUCKET_NAME where git-board-server runs")
