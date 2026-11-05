package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/guilbep/git-board/next/forge"
)

// A GitHub project's releases: the release mapper publishes ours there.

var _ forge.Releaser = (*Forge)(nil)

type ghRelease struct {
	ID         int64  `json:"id"`
	Tag        string `json:"tag_name"`
	Name       string `json:"name"`
	Body       string `json:"body"`
	Prerelease bool   `json:"prerelease"`
	HTMLURL    string `json:"html_url"`
	UploadURL  string `json:"upload_url"`
	Assets     []struct {
		Name string `json:"name"`
	} `json:"assets"`
}

func (g ghRelease) release() forge.Release {
	r := forge.Release{ID: id(g.ID), Tag: g.Tag, Name: g.Name, Body: lf(g.Body), Prerelease: g.Prerelease, URL: g.HTMLURL}
	for _, a := range g.Assets {
		r.Assets = append(r.Assets, a.Name)
	}
	return r
}

// Releases lists the repository's releases, newest first.
func (f *Forge) Releases(ctx context.Context) ([]forge.Release, error) {
	all, err := pages(ctx, f, f.s.repo+"/releases", func(r ghRelease) int64 { return r.ID })
	out := make([]forge.Release, len(all))
	for i, r := range all {
		out[i] = r.release()
	}
	return out, err
}

// CreateRelease makes the release of r.Tag, which the repository has.
func (f *Forge) CreateRelease(ctx context.Context, r forge.Release) (forge.Release, error) {
	var g ghRelease
	in := map[string]any{"tag_name": r.Tag, "name": r.Name, "body": r.Body, "prerelease": r.Prerelease}
	if err := f.call(ctx, http.MethodPost, f.s.repo+"/releases", in, &g); err != nil {
		return forge.Release{}, err
	}
	return g.release(), nil
}

// EditRelease sets a release's name, body and prerelease.
func (f *Forge) EditRelease(ctx context.Context, rid string, r forge.Release) error {
	in := map[string]any{"name": r.Name, "body": r.Body, "prerelease": r.Prerelease}
	return f.call(ctx, http.MethodPatch, f.s.repo+"/releases/"+url.PathEscape(rid), in, nil)
}

// Attach uploads a file to a release, through the upload URL GitHub
// gives for it: on uploads.<host>, or the API's own host.
func (f *Forge) Attach(ctx context.Context, rid, name string, data []byte) error {
	var g ghRelease
	if err := f.call(ctx, http.MethodGet, f.s.repo+"/releases/"+url.PathEscape(rid), nil, &g); err != nil {
		return err
	}
	at, _, _ := strings.Cut(g.UploadURL, "{")
	api, _ := url.Parse(f.s.api)
	if err := checkURL(at, "uploads."+f.s.host, api.Host); err != nil {
		return err
	}
	if err := f.s.pace(ctx, f.Key()); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, at+"?name="+url.QueryEscape(name), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+f.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "git-board")
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := f.s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &e)
		return statusError(http.MethodPost, at, resp.StatusCode, fmt.Sprintf("%s: %s", name, e.Message), false)
	}
	return nil
}
