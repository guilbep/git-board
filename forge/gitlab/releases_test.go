package gitlab

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"testing"

	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/fake"
)

func releaseJSON(r forge.Release) map[string]any {
	var links []map[string]string
	for _, a := range r.Assets {
		links = append(links, map[string]string{"name": a})
	}
	return map[string]any{"tag_name": r.Tag, "name": r.Name, "description": r.Body,
		"_links": map[string]string{"self": r.URL}, "assets": map[string]any{"links": links}}
}

func (s *server) releases(w http.ResponseWriter, r *http.Request, view forge.Forge) {
	rs, _ := view.(forge.Releaser).Releases(r.Context())
	out := []map[string]any{}
	for _, x := range rs {
		out = append(out, releaseJSON(x))
	}
	reply(w, 200, out)
}

// id is the fake's id of the release of a tag: GitLab names it by its tag.
func id(r *http.Request, view forge.Forge, tag string) string {
	rs, _ := view.(forge.Releaser).Releases(r.Context())
	if i := slices.IndexFunc(rs, func(x forge.Release) bool { return x.Tag == tag }); i >= 0 {
		return rs[i].ID
	}
	return ""
}

func (s *server) createRelease(w http.ResponseWriter, r *http.Request, view forge.Forge) {
	var in struct{ Tag_name, Name, Description string }
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		reply(w, 400, map[string]string{"message": err.Error()})
		return
	}
	got, err := view.(forge.Releaser).CreateRelease(r.Context(), forge.Release{Tag: in.Tag_name, Name: in.Name, Body: in.Description})
	if err != nil {
		reply(w, 409, map[string]string{"message": "Release already exists"})
		return
	}
	reply(w, 201, releaseJSON(got))
}

func (s *server) editRelease(w http.ResponseWriter, r *http.Request, view forge.Forge) {
	var in struct{ Name, Description string }
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		reply(w, 400, map[string]string{"message": err.Error()})
		return
	}
	if err := view.(forge.Releaser).EditRelease(r.Context(), id(r, view, r.PathValue("tag")), forge.Release{Name: in.Name, Body: in.Description}); err != nil {
		fail(w, err)
		return
	}
	reply(w, 200, map[string]any{})
}

func (s *server) upload(w http.ResponseWriter, r *http.Request, _ forge.Forge) {
	f, h, err := r.FormFile("file")
	if err != nil {
		reply(w, 400, map[string]string{"message": err.Error()})
		return
	}
	data, _ := io.ReadAll(f)
	path := "/-/project/1/uploads/abc/" + h.Filename
	s.mu.Lock()
	if s.uploads == nil {
		s.uploads = map[string][]byte{}
	}
	s.uploads[path] = data
	s.mu.Unlock()
	reply(w, 201, map[string]string{"full_path": path, "url": "/uploads/abc/" + h.Filename})
}

func (s *server) link(w http.ResponseWriter, r *http.Request, view forge.Forge) {
	var in struct{ Name, URL string }
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		reply(w, 400, map[string]string{"message": err.Error()})
		return
	}
	u, _ := http.NewRequest(http.MethodGet, in.URL, nil)
	s.mu.Lock()
	data, ok := s.uploads[u.URL.Path]
	s.mu.Unlock()
	if !ok {
		reply(w, 400, map[string]string{"message": "no such upload"})
		return
	}
	if err := view.(forge.Releaser).Attach(r.Context(), id(r, view, r.PathValue("tag")), in.Name, data); err != nil {
		reply(w, 409, map[string]string{"message": "exists"})
		return
	}
	reply(w, 201, map[string]any{})
}

// A release is made, listed, edited and given files through GitLab's API,
// named by its tag; its files are uploads linked from it.
func TestReleases(t *testing.T) {
	t.Parallel()
	f := fake.New("gitlab.example.com/g/p")
	_, a := serve(t, f, "g/p")
	ctx := t.Context()
	made, err := a.CreateRelease(ctx, forge.Release{Tag: "v1.0.0", Name: "v1.0.0", Body: "notes"})
	if err != nil || made.ID != "v1.0.0" {
		t.Fatalf("CreateRelease = %+v, %v", made, err)
	}
	if _, err := a.CreateRelease(ctx, forge.Release{Tag: "v1.0.0"}); err == nil {
		t.Error("a second release of one tag was made")
	}
	if err := a.EditRelease(ctx, made.ID, forge.Release{Name: "v1.0.0", Body: "more notes"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Attach(ctx, made.ID, "app_linux_amd64.tar.gz", []byte("bytes")); err != nil {
		t.Fatal(err)
	}
	rs, err := a.Releases(ctx)
	if err != nil || len(rs) != 1 {
		t.Fatalf("Releases = %+v, %v", rs, err)
	}
	if r := rs[0]; r.Body != "more notes" || !slices.Equal(r.Assets, []string{"app_linux_amd64.tar.gz"}) {
		t.Errorf("release = %+v", r)
	}
	if b, ok := f.File("v1.0.0", "app_linux_amd64.tar.gz"); !ok || string(b) != "bytes" {
		t.Errorf("file = %q, %v", b, ok)
	}
}
