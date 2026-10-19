package release

import (
	"context"
	"fmt"
	"strings"
)

// Notes is the release notes in CHANGELOG form, newest first: finals, and
// candidates too when all. Each release is "## <tag> (<date>)" and its
// record's notes; a yanked one says so. A tag without a record is left
// out. It fetches unless Offline.
func (r *Runner) Notes(ctx context.Context, all bool) (string, error) {
	st, err := r.Status(ctx)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, row := range st.Releases {
		if row.Record == nil || (row.Tag.Version.RC != 0 && !all) {
			continue
		}
		fmt.Fprintf(&b, "## %s (%s)", row.Tag.Name, row.Tag.Date.Format("2006-01-02"))
		if y := row.Record.Yanked; y != "" {
			fmt.Fprintf(&b, " — yanked: %s", y)
		}
		fmt.Fprintf(&b, "\n\n%s\n\n", strings.TrimSpace(row.Record.Notes))
	}
	return b.String(), nil
}

// Yank marks tag's release as not to be used, with the reason, and pushes
// the record. It fetches the records first; offline it refuses.
func (r *Runner) Yank(ctx context.Context, tag, reason string) error {
	return r.editRecord(ctx, "a yank", tag, func(id string) error { return r.B.YankRelease(id, reason) })
}

// SetNotes replaces tag's release notes, as Yank does.
func (r *Runner) SetNotes(ctx context.Context, tag, notes string) error {
	return r.editRecord(ctx, "a notes edit", tag, func(id string) error { return r.B.SetReleaseNotes(id, notes) })
}

// Comment adds message to tag's release conversation, as Yank does.
func (r *Runner) Comment(ctx context.Context, tag, message string) error {
	return r.editRecord(ctx, "a comment", tag, func(id string) error { return r.B.CommentRelease(id, message) })
}

// editRecord fetches the authority remote, applies edit to the record of
// tag's commit and pushes the records; offline it refuses what.
func (r *Runner) editRecord(ctx context.Context, what, tag string, edit func(id string) error) error {
	if err := r.online(what); err != nil {
		return err
	}
	if err := r.fetch(ctx); err != nil {
		return err
	}
	t, err := r.tagNamed(ctx, tag)
	if err != nil {
		return err
	}
	rec, ok, err := r.B.ReleaseFor(t.Commit)
	switch {
	case err != nil:
		return err
	case !ok:
		return fmt.Errorf("%s has no release record", tag)
	}
	if err := edit(rec.ID); err != nil {
		return err
	}
	return r.B.PushReleases(r.Cfg.RemoteName())
}
