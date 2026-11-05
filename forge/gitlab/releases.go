package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/guilbep/git-board/forge"
)

// A GitLab project's releases: the release mapper publishes ours there.
// A release is named by its tag; GitLab keeps no prerelease flag, and
// its files are the project's uploads, linked from the release.

var _ forge.Releaser = (*Forge)(nil)

type glRelease struct {
	Tag         string `json:"tag_name"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Links       struct {
		Self string `json:"self"`
	} `json:"_links"`
	Assets struct {
		Links []struct {
			Name string `json:"name"`
		} `json:"links"`
	} `json:"assets"`
}

func (g glRelease) key() int64 {
	h := fnv.New64a()
	h.Write([]byte(g.Tag))
	return int64(h.Sum64() >> 1)
}

func (g glRelease) release() forge.Release {
	r := forge.Release{ID: g.Tag, Tag: g.Tag, Name: g.Name, Body: text(g.Description), URL: g.Links.Self}
	for _, l := range g.Assets.Links {
		r.Assets = append(r.Assets, l.Name)
	}
	return r
}

// Releases lists the project's releases, newest first.
func (f *Forge) Releases(ctx context.Context) ([]forge.Release, error) {
	var out []forge.Release
	err := each(ctx, f, f.p.api+"/releases", nil, func(g glRelease) (bool, error) {
		out = append(out, g.release())
		return true, nil
	})
	return out, err
}

// CreateRelease makes the release of r.Tag, which the project has.
func (f *Forge) CreateRelease(ctx context.Context, r forge.Release) (forge.Release, error) {
	var g glRelease
	in := map[string]string{"tag_name": r.Tag, "name": r.Name, "description": r.Body}
	if _, err := f.call(ctx, http.MethodPost, f.p.api+"/releases", nil, in, &g); err != nil {
		return forge.Release{}, err
	}
	return g.release(), nil
}

// EditRelease sets a release's name and description.
func (f *Forge) EditRelease(ctx context.Context, tag string, r forge.Release) error {
	in := map[string]string{"name": r.Name, "description": r.Body}
	_, err := f.call(ctx, http.MethodPut, f.p.api+"/releases/"+url.PathEscape(tag), nil, in, nil)
	return err
}

// Attach uploads a file to the project, and links it from the release.
func (f *Forge) Attach(ctx context.Context, tag, name string, data []byte) error {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", name)
	if err == nil {
		_, err = part.Write(data)
	}
	if err == nil {
		err = mw.Close()
	}
	if err != nil {
		return err
	}
	if err := f.wait(ctx, true); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.p.api+"/uploads", &body)
	if err != nil {
		return err
	}
	req.Header.Set("PRIVATE-TOKEN", f.token)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := f.p.http.Do(req)
	if err != nil {
		return fmt.Errorf("gitlab: POST /uploads: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return refusal("POST /uploads "+name, resp.StatusCode, raw)
	}
	var up struct {
		FullPath string `json:"full_path"`
		URL      string `json:"url"`
	}
	if err := json.Unmarshal(raw, &up); err != nil {
		return fmt.Errorf("gitlab: POST /uploads: %w", err)
	}
	link := up.FullPath
	if link == "" {
		link = up.URL
	}
	if !strings.HasPrefix(link, "http") {
		link = strings.TrimSuffix(f.p.root, "/api/v4") + link
	}
	in := map[string]string{"name": name, "url": link, "link_type": "package"}
	_, err = f.call(ctx, http.MethodPost, f.p.api+"/releases/"+url.PathEscape(tag)+"/assets/links", nil, in, nil)
	return err
}
