package server

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/guilbep/git-board/internal/store"
)

// maxMedia caps an image the proxy fetches.
const maxMedia = 20 << 20

// mediaDir is where fetched images are kept, in git-board's folder of the
// repository: a page shows them again without the network.
const mediaDir = "git-board-media"

// mediaProxy serves the images of issue text, fetched from their forge:
// with the forge's token (a private repository's images need it), kept
// on disk, and only from a forge's image addresses, so it can't be used to
// reach anything else from this machine.
type mediaProxy struct {
	storage store.Storage
	client  *http.Client
	token   func(host string) string
	allowed func(*url.URL) bool
}

func newMediaProxy(storage store.Storage, token func(string) string, gitlabHosts func() []string) *mediaProxy {
	m := &mediaProxy{storage: storage, token: token,
		allowed: func(u *url.URL) bool { return forgeMedia(u, gitlabHosts()) }}
	m.client = &http.Client{
		Timeout: 30 * time.Second,
		// GitHub redirects to S3; each hop must be a forge image address too
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 5 || !m.allowed(req.URL) {
				return errors.New("redirect off the forge")
			}
			return nil
		},
	}
	return m
}

// forgeMedia tells whether u is where a forge keeps the images of issues.
func forgeMedia(u *url.URL, gitlabHosts []string) bool {
	if u.Scheme != "https" {
		return false
	}
	host, p := u.Hostname(), u.Path
	switch {
	case host == "github.com":
		return strings.HasPrefix(p, "/user-attachments/") || strings.Contains(p, "/assets/")
	case strings.HasSuffix(host, ".githubusercontent.com"):
		return true
	case strings.HasPrefix(host, "github-production-user-asset-") && strings.HasSuffix(host, ".s3.amazonaws.com"):
		return true
	case host == "gitlab.com" || containsString(gitlabHosts, host):
		return strings.Contains(p, "/uploads/")
	}
	return false
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (m *mediaProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("url")
	u, err := url.Parse(raw)
	if err != nil || !m.allowed(u) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("not a forge image: %q", raw))
		return
	}
	sum := sha256.Sum256([]byte(raw))
	name := path.Join(mediaDir, hex.EncodeToString(sum[:]))

	contentType, data, err := m.cached(name)
	if err != nil {
		if contentType, data, err = m.fetch(u); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		m.keep(name, contentType, data)
	}
	h := w.Header()
	h.Set("Content-Type", contentType)
	h.Set("Cache-Control", "private, max-age=86400")
	h.Set("X-Content-Type-Options", "nosniff")
	// an SVG opened on its own can't run its scripts
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	_, _ = w.Write(data)
}

func (m *mediaProxy) fetch(u *url.URL) (string, []byte, error) {
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return "", nil, err
	}
	// Go drops the header on a redirect to another host (S3)
	if t := m.token(u.Hostname()); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("the forge answered %s", resp.Status)
	}
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return "", nil, fmt.Errorf("not an image: %s", contentType)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxMedia+1))
	if err != nil {
		return "", nil, err
	}
	if len(data) > maxMedia {
		return "", nil, fmt.Errorf("an image over %d MB", maxMedia>>20)
	}
	return contentType, data, nil
}

// cached returns a kept image: its type on the first line, then its bytes.
func (m *mediaProxy) cached(name string) (string, []byte, error) {
	f, err := m.storage.Open(name)
	if err != nil {
		return "", nil, err
	}
	defer f.Close()
	all, err := io.ReadAll(f)
	if err != nil {
		return "", nil, err
	}
	contentType, data, ok := strings.Cut(string(all), "\n")
	if !ok {
		return "", nil, errors.New("bad cache entry")
	}
	return contentType, []byte(data), nil
}

// keep stores an image; a failure only costs a fetch next time.
func (m *mediaProxy) keep(name, contentType string, data []byte) {
	f, err := m.storage.Create(name)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = io.WriteString(f, contentType+"\n")
	_, _ = f.Write(data)
}
