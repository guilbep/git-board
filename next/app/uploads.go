package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/guilbep/git-board/next/tools/uploads"
)

// Attachments (ADR 0015): the files of issue text go to an S3 bucket a
// CDN serves (next/tools/uploads), keys from the server's environment,
// addresses from [uploads].

// UploadMaxBytes caps a file: GitHub's limit for attachments.
const UploadMaxBytes = uploads.MaxBytes

// UploadBucket is an S3 bucket a CDN serves.
type UploadBucket = uploads.Store

// UploadStore, when set, is the bucket instead of the environment's: the
// tests' fake.
var UploadStore *UploadBucket

// UploadStatus says whether files can be attached, and where they go.
type UploadStatus struct {
	Enabled  bool   `json:"enabled"`
	Where    string `json:"where,omitempty"` // the CDN's address of the board's folder
	MaxBytes int    `json:"max_bytes"`
}

// Uploaded is a file put on the CDN: its address, and the Markdown that
// shows it.
type Uploaded struct {
	URL      string `json:"url"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Markdown string `json:"markdown"`
}

// uploadStore is the bucket, false when none is set up.
func (b *Board) uploadStore() (*uploads.Store, bool) {
	if UploadStore != nil {
		return UploadStore, true
	}
	u, _ := b.Config.Tools.Uploads.Get()
	return uploads.FromEnv(u, filepath.Base(b.Dir))
}

// Uploads says whether files can be attached here.
func (b *Board) Uploads() UploadStatus {
	st, ok := b.uploadStore()
	if !ok {
		return UploadStatus{MaxBytes: uploads.MaxBytes}
	}
	return UploadStatus{Enabled: true, Where: st.PublicURL + "/" + st.Prefix, MaxBytes: uploads.MaxBytes}
}

// Upload puts one file on the CDN: refused when its kind isn't one an
// issue carries, or no bucket is set up.
func (b *Board) Upload(ctx context.Context, name string, data []byte) (Uploaded, error) {
	st, ok := b.uploadStore()
	if !ok {
		return Uploaded{}, &Error{Class: Missing, Err: uploads.ErrDisabled}
	}
	contentType, err := uploads.Check(name, data)
	if err != nil {
		return Uploaded{}, &Error{Class: Usage, Err: err}
	}
	url, err := st.Put(ctx, name, data, contentType)
	if err != nil {
		return Uploaded{}, err
	}
	shown := strings.NewReplacer("[", "(", "]", ")").Replace(name)
	md := fmt.Sprintf("[%s](%s)", shown, url)
	if strings.HasPrefix(contentType, "image/") {
		md = "!" + md
	}
	return Uploaded{URL: url, Name: name, Type: contentType, Markdown: md}, nil
}
