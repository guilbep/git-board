package forge

import "context"

// Release is a forge's release of a tag: GitHub's releases, GitLab's.
type Release struct {
	ID         string   // the forge's; GitLab's is the tag
	Tag        string   // v1.4.20, pushed already
	Name       string   // its title, the tag by default
	Body       string   // its notes, Markdown
	Prerelease bool     // a candidate; GitLab has no such flag, and keeps none
	Assets     []string // the files attached, by name
	URL        string   // its page
}

// Releaser is a forge that carries releases: the release mapper
// publishes ours there (next/domain/releases/releasesync). Both GitHub
// and GitLab are.
type Releaser interface {
	// Releases lists the project's releases, newest first.
	Releases(ctx context.Context) ([]Release, error)
	// CreateRelease makes the release of r.Tag, a tag the forge has.
	CreateRelease(ctx context.Context, r Release) (Release, error)
	// EditRelease sets the name, body and prerelease of the release id.
	EditRelease(ctx context.Context, id string, r Release) error
	// Attach uploads a file to the release id, as name.
	Attach(ctx context.Context, id, name string, data []byte) error
}
