package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/uploads"
)

// uploadStore is the bucket attachments go to: set by tests, else from the
// environment (ACCESS_KEY_ID, ACCESS_KEY_SECRET, BUCKET_NAME) and
// [uploads]; nil when none is set up.
func (s *apiServer) uploadStore() *uploads.Store {
	if s.store != nil {
		return s.store
	}
	repo := filepath.Base(filepath.Dir(config.Path()))
	if st, ok := uploads.FromEnv(s.cfg.Uploads, repo); ok {
		return st
	}
	return nil
}

func (s *apiServer) handleUploadStatus(w http.ResponseWriter, r *http.Request) {
	st := s.uploadStore()
	res := map[string]any{"enabled": st != nil, "max_bytes": uploads.MaxBytes}
	if st != nil {
		res["public_url"] = st.PublicURL
	}
	writeJSON(w, http.StatusOK, res)
}

// handleUpload puts one file (multipart field "file") on the CDN and
// answers its public address, and the Markdown that shows it.
func (s *apiServer) handleUpload(w http.ResponseWriter, r *http.Request) {
	st := s.uploadStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, uploads.ErrDisabled)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, uploads.MaxBytes+1<<20)
	file, header, err := r.FormFile("file")
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			err = fmt.Errorf("a file is %d MB at most", uploads.MaxBytes>>20)
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	contentType, err := uploads.Check(header.Filename, data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	url, err := st.Put(r.Context(), header.Filename, data, contentType)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	name := strings.NewReplacer("[", "(", "]", ")").Replace(header.Filename)
	md := fmt.Sprintf("[%s](%s)", name, url)
	if strings.HasPrefix(contentType, "image/") {
		md = "!" + md
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url, "name": header.Filename, "type": contentType, "markdown": md})
}
