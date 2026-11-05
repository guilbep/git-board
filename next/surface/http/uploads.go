package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/guilbep/git-board/next/app"
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
	writeJSON(w, http.StatusOK, up)
}
