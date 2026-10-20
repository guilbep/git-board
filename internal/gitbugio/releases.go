package gitbugio

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/git-bug/git-bug/cache"
	"github.com/git-bug/git-bug/entities/identity"
	"github.com/git-bug/git-bug/entity"
	"github.com/git-bug/git-bug/entity/dag"
	"github.com/git-bug/git-bug/util/lamport"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

// A release's page, kept in git as a git-bug entity of git-board's own, in
// refs/releases/<id>: one record per released commit, frozen at creation
// but for its notes, comments and yank. Its names are the tags on its
// commit, read from git, never stored.

// ReleasesNamespace is where the records live: refs/releases.
const ReleasesNamespace = "releases"

var releaseDef = dag.Definition{
	Typename:             "release",
	Namespace:            ReleasesNamespace,
	OperationUnmarshaler: unmarshalReleaseOp,
	FormatVersion:        1,
}

// Artifact is an image a release shipped, by digest.
type Artifact struct {
	Image    string `json:"image"`
	Registry string `json:"registry"`
	Digest   string `json:"digest"`
}

// ChartArtifact is the Helm chart a release shipped.
type ChartArtifact struct {
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

// ReleaseComment is a comment on a release.
type ReleaseComment struct {
	Author  string    `json:"author"`
	Message string    `json:"message"`
	Date    time.Time `json:"date"`
}

// Asset is an archive published with a release: its name and sha256, as
// checksums.txt has them.
type Asset struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

// ReleaseRecord is a release's page: what it shipped and what was said
// about it.
type ReleaseRecord struct {
	ID        string           `json:"id"`
	Commit    string           `json:"commit"`
	Previous  string           `json:"previous"` // the previous final's tag
	Artifacts []Artifact       `json:"artifacts"`
	Chart     ChartArtifact    `json:"chart"`
	Assets    []Asset          `json:"assets"` // the archives published with its tag
	Notes     string           `json:"notes"`
	Comments  []ReleaseComment `json:"comments"`
	Yanked    string           `json:"yanked"`  // the reason, "" when not
	Author    string           `json:"author"`  // the creating identity's name
	Created   time.Time        `json:"created"` // as its author's clock had it

	created lamport.Time // when two clones made one for a commit, the earlier counts
}

func asEntity(e *dag.Entity) *dag.Entity { return e }

func compileRelease(e *dag.Entity) ReleaseRecord {
	r := ReleaseRecord{ID: e.Id().String(), created: e.CreateLamportTime()}
	for _, op := range e.Operations() {
		if op, ok := op.(releaseOp); ok { // not a later git-board's
			op.apply(&r)
		}
	}
	return r
}

// counts tells r is the record of its commit rather than other: the
// earlier made, by Lamport time, then the smaller id.
func (r ReleaseRecord) counts(other ReleaseRecord) bool {
	return cmp.Or(cmp.Compare(r.created, other.created), cmp.Compare(r.ID, other.ID)) < 0
}

// releaseResolvers find a record's authors in the identity cache.
func (b *Backend) releaseResolvers() entity.Resolvers {
	return entity.Resolvers{&cache.IdentityCache{}: entity.ResolverFunc[*cache.IdentityCache](b.c.Identities().Resolve)}
}

// Releases is the records, one per commit, newest first. A second record
// for a commit, made on another clone before the two met, never shows.
func (b *Backend) Releases() ([]ReleaseRecord, error) {
	byCommit := map[string]ReleaseRecord{}
	for s := range dag.ReadAll(releaseDef, asEntity, b.repo, b.releaseResolvers()) {
		if s.Err != nil {
			return nil, fmt.Errorf("reading the releases: %w", s.Err)
		}
		r := compileRelease(s.Entity)
		if cur, ok := byCommit[r.Commit]; !ok || r.counts(cur) {
			byCommit[r.Commit] = r
		}
	}
	return slices.SortedFunc(maps.Values(byCommit), func(x, y ReleaseRecord) int {
		return cmp.Or(y.Created.Compare(x.Created), cmp.Compare(x.ID, y.ID))
	}), nil
}

// ReleaseFor is commit's record; ok is false while it has none.
// ponytail: reads every record, an index by commit if there get to be many
func (b *Backend) ReleaseFor(commit string) (r ReleaseRecord, ok bool, err error) {
	rs, err := b.Releases()
	if i := slices.IndexFunc(rs, func(x ReleaseRecord) bool { return x.Commit == commit }); i >= 0 {
		return rs[i], true, nil
	}
	return ReleaseRecord{}, false, err
}

// CreateRelease records r's commit, previous, artifacts and chart, by you;
// the rest is the other operations'. A commit that has a record already
// keeps it, and it is returned.
func (b *Backend) CreateRelease(r ReleaseRecord) (ReleaseRecord, error) {
	if got, ok, err := b.ReleaseFor(r.Commit); ok || err != nil {
		return got, err
	}
	me, err := b.c.GetUserIdentity()
	if err != nil {
		return ReleaseRecord{}, err
	}
	e := dag.New(releaseDef)
	e.Append(&releaseCreate{OpBase: opBase(releaseCreateOp, me), Commit: r.Commit, Previous: r.Previous, Artifacts: r.Artifacts, Chart: r.Chart, Assets: r.Assets})
	if err := e.Commit(b.repo); err != nil {
		return ReleaseRecord{}, fmt.Errorf("recording the release of %s: %w", r.Commit, err)
	}
	return compileRelease(e), nil
}

// SetReleaseNotes replaces the notes of the record with this id.
func (b *Backend) SetReleaseNotes(id, notes string) error {
	return b.editRelease(id, func(me identity.Interface) dag.Operation {
		return &releaseSetNotes{OpBase: opBase(releaseSetNotesOp, me), Notes: notes}
	})
}

// CommentRelease comments on the record with this id.
func (b *Backend) CommentRelease(id, message string) error {
	return b.editRelease(id, func(me identity.Interface) dag.Operation {
		return &releaseComment{OpBase: opBase(releaseCommentOp, me), Message: message}
	})
}

// YankRelease marks the record with this id yanked, for reason.
func (b *Backend) YankRelease(id, reason string) error {
	return b.editRelease(id, func(me identity.Interface) dag.Operation {
		return &releaseYank{OpBase: opBase(releaseYankOp, me), Reason: reason}
	})
}

// editRelease adds an operation by you to the record with this id.
func (b *Backend) editRelease(id string, op func(me identity.Interface) dag.Operation) error {
	e, err := dag.Read(releaseDef, asEntity, b.repo, b.releaseResolvers(), entity.Id(id))
	if err != nil {
		return fmt.Errorf("reading release %s: %w", id, err)
	}
	me, err := b.c.GetUserIdentity()
	if err != nil {
		return err
	}
	e.Append(op(me))
	if err := e.Commit(b.repo); err != nil {
		return fmt.Errorf("editing release %s: %w", id, err)
	}
	return nil
}

// PushReleases pushes refs/releases to remote, as Push does with the
// issues. The identities go along, as a record names its authors by
// identity.
func (b *Backend) PushReleases(remote string) error {
	if _, err := b.repo.PushRefs(remote, b.c.Identities().GetNamespace(), ReleasesNamespace); err != nil {
		return fmt.Errorf("pushing the releases to %s: %w", remote, err)
	}
	return nil
}

// PullReleases fetches remote's records, and the identities that wrote
// them, and merges them into this clone's. An empty remote is nothing to
// pull.
func (b *Backend) PullReleases(remote string) error {
	_, err := b.repo.FetchRefs(remote, b.c.Identities().GetNamespace(), ReleasesNamespace)
	if errors.Is(err, transport.ErrEmptyRemoteRepository) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("fetching the releases from %s: %w", remote, err)
	}
	return errors.Join(mergeErr(b.c.Identities().MergeAll(remote)), b.mergeReleases(remote))
}

// mergeReleases merges remote's fetched records into this clone's, once
// the identities that wrote them are merged.
func (b *Backend) mergeReleases(remote string) error {
	// a merge commit, when both sides edited a record, needs an author;
	// without one only that record fails
	var me identity.Interface
	if i, err := b.c.GetUserIdentity(); err == nil {
		me = i
	}
	return mergeErr(dag.MergeAll(releaseDef, asEntity, b.repo, b.releaseResolvers(), remote, me))
}
