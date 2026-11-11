package httpapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/guilbep/git-board/app"
)

// Attachments (docs/rewrite/api.md, routes 40 and 41; ADR 0015): whether
// files can be attached, and one file put on the CDN. The upload is the
// one request that isn't JSON: multipart/form-data, its file in "file",
// with the page's X-Git-Board-Upload: 1 (guard.go).

func uploadStatus(ctx context.Context, _ none) (app.UploadStatus, error) {
	return board(ctx).Uploads(), nil
}

// maxUpload is the most an upload's request may weigh: the file and its
// form.
const maxUpload = 26 << 20

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	file, header, err := r.FormFile("file")
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			err = fmt.Errorf("a file is %d MB at most", app.UploadMaxBytes>>20)
		}
		writeError(w, r, logOf(r.Context()), invalid("file", err))
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, r, logOf(r.Context()), invalid("file", err))
		return
	}
	up, err := board(r.Context()).Upload(r.Context(), header.Filename, data)
	if err != nil {
		writeError(w, r, logOf(r.Context()), err)
		return
	}
	writeJSON(w, r, http.StatusOK, up)
}

// media is route 42: a forge's picture of issue text, through this
// server, which has the forge's token (app.Media). Its ETag answers a
// browser's If-None-Match with 304; an address that names one picture
// for good is kept a year, any other a day, then asked again. private:
// a shared cache keeps none.
// ponytail: an <img> sends no Authorization: a server several people
// reach needs a cookie or a signed ?sig= before it serves a private
// repository's pictures; this one serves its own user's, with their token.
func media(w http.ResponseWriter, r *http.Request) {
	p, err := board(r.Context()).Media(r.Context(), r.URL.Query().Get("url"))
	if err != nil {
		writeError(w, r, logOf(r.Context()), on("url", err))
		return
	}
	h := w.Header()
	h.Set("Content-Type", p.Type)
	h.Set("ETag", p.ETag)
	if p.Immutable {
		h.Set("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "private, max-age=86400")
	}
	h.Set("X-Content-Type-Options", "nosniff")
	// an SVG opened on its own can't run its scripts
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(p.Data))
}
