package github

import (
	"io"
	"net/http"
	"slices"
	"strconv"
	"testing"

	"github.com/guilbep/git-board/next/forge"
	"github.com/guilbep/git-board/next/forge/fake"
)

func (h *hub) releaseJSON(r forge.Release) map[string]any {
	n, _ := strconv.ParseInt(r.ID, 10, 64)
	var assets []map[string]string
	for _, a := range r.Assets {
		assets = append(assets, map[string]string{"name": a})
	}
	return map[string]any{"id": n, "tag_name": r.Tag, "name": r.Name, "body": r.Body, "prerelease": r.Prerelease,
		"html_url": r.URL, "upload_url": h.url + "/upload/" + r.ID + "{?name,label}", "assets": assets}
}

func (h *hub) releases(w http.ResponseWriter, r *http.Request, ff forge.Forge) {
	rs, _ := ff.(forge.Releaser).Releases(r.Context())
	var out []map[string]any
	if r.URL.Query().Get("page") == "1" {
		for _, x := range rs {
			out = append(out, h.releaseJSON(x))
		}
	}
	write(w, http.StatusOK, out)
}

func (h *hub) release(w http.ResponseWriter, r *http.Request, ff forge.Forge) {
	rs, _ := ff.(forge.Releaser).Releases(r.Context())
	i := slices.IndexFunc(rs, func(x forge.Release) bool { return x.ID == r.PathValue("id") })
	if i < 0 {
		fail(w, forge.ErrNotFound)
		return
	}
	write(w, http.StatusOK, h.releaseJSON(rs[i]))
}

type releaseIn struct {
	Tag        string `json:"tag_name"`
	Name       string `json:"name"`
	Body       string `json:"body"`
	Prerelease bool   `json:"prerelease"`
}

func (h *hub) createRelease(w http.ResponseWriter, r *http.Request, ff forge.Forge) {
	var in releaseIn
	if !read(w, r, &in) {
		return
	}
	got, err := ff.(forge.Releaser).CreateRelease(r.Context(), forge.Release{Tag: in.Tag, Name: in.Name, Body: in.Body, Prerelease: in.Prerelease})
	if err != nil {
		write(w, http.StatusUnprocessableEntity, map[string]string{"message": "Validation Failed"})
		return
	}
	write(w, http.StatusCreated, h.releaseJSON(got))
}

func (h *hub) editRelease(w http.ResponseWriter, r *http.Request, ff forge.Forge) {
	var in releaseIn
	if !read(w, r, &in) {
		return
	}
	if err := ff.(forge.Releaser).EditRelease(r.Context(), r.PathValue("id"), forge.Release{Name: in.Name, Body: in.Body, Prerelease: in.Prerelease}); err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, map[string]any{})
}

func (h *hub) attach(w http.ResponseWriter, r *http.Request, ff forge.Forge) {
	data, _ := io.ReadAll(r.Body)
	if r.Header.Get("Content-Type") != "application/octet-stream" {
		write(w, http.StatusBadRequest, map[string]string{"message": "no content type"})
		return
	}
	if err := ff.(forge.Releaser).Attach(r.Context(), r.PathValue("id"), r.URL.Query().Get("name"), data); err != nil {
		write(w, http.StatusUnprocessableEntity, map[string]string{"message": "Validation Failed"})
		return
	}
	write(w, http.StatusCreated, map[string]any{})
}

// A release is made, listed, edited and given files through GitHub's
// API; a second of one tag is refused.
func TestReleases(t *testing.T) {
	t.Parallel()
	f := fake.New("example.com/p")
	h := newHub(t, f)
	a := h.adapter(t)
	ctx := t.Context()
	made, err := a.CreateRelease(ctx, forge.Release{Tag: "v1.0.0-rc.1", Name: "v1.0.0-rc.1", Body: "notes", Prerelease: true})
	if err != nil || made.ID == "" || made.Tag != "v1.0.0-rc.1" || !made.Prerelease {
		t.Fatalf("CreateRelease = %+v, %v", made, err)
	}
	if _, err := a.CreateRelease(ctx, forge.Release{Tag: "v1.0.0-rc.1"}); err == nil {
		t.Error("a second release of one tag was made")
	}
	if err := a.EditRelease(ctx, made.ID, forge.Release{Name: "v1.0.0-rc.1", Body: "yanked: broken\n\nnotes"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Attach(ctx, made.ID, "app_linux_amd64.tar.gz", []byte("bytes")); err != nil {
		t.Fatal(err)
	}
	rs, err := a.Releases(ctx)
	if err != nil || len(rs) != 1 {
		t.Fatalf("Releases = %+v, %v", rs, err)
	}
	if r := rs[0]; r.Body != "yanked: broken\n\nnotes" || r.Prerelease || !slices.Equal(r.Assets, []string{"app_linux_amd64.tar.gz"}) {
		t.Errorf("release = %+v", r)
	}
	if b, ok := f.File("v1.0.0-rc.1", "app_linux_amd64.tar.gz"); !ok || string(b) != "bytes" {
		t.Errorf("file = %q, %v", b, ok)
	}
	if err := a.EditRelease(ctx, "999", forge.Release{}); err == nil {
		t.Error("an edit of no release landed")
	}
}
