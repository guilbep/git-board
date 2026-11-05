package app

import (
	"net/url"
	"testing"
)

// A forge's pictures are GitHub's attachments and their S3, and GitLab's
// uploads on its hosts: nothing else goes through the server.
func TestForgeMedia(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]bool{
		"https://github.com/user-attachments/assets/1234":                      true,
		"https://github.com/o/r/assets/5678/abc":                               true,
		"https://private-user-images.githubusercontent.com/1/x.png":            true,
		"https://github-production-user-asset-6210df.s3.amazonaws.com/1/x.png": true,
		"https://gitlab.com/o/r/uploads/abc/x.png":                             true,
		"https://gitlab.epfl.ch/o/r/uploads/abc/x.png":                         true,
		"https://gitlab.example.org/o/r/uploads/abc/x.png":                     false,
		"http://github.com/user-attachments/assets/1234":                       false,
		"https://github.com/o/r/issues/1":                                      false,
		"https://example.com/a.png":                                            false,
		"https://s3.amazonaws.com/github-production-user-asset-6210df/1/x.png": false,
	} {
		u, _ := url.Parse(raw)
		if got := ForgeMedia(u, []string{"gitlab.epfl.ch"}); got != want {
			t.Errorf("ForgeMedia(%s) = %v, want %v", raw, got, want)
		}
	}
}
