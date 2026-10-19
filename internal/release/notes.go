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
	if err := r.online("a yank"); err != nil {
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
	if err := r.B.YankRelease(rec.ID, reason); err != nil {
		return err
	}
	return r.B.PushReleases(r.Cfg.RemoteName())
}
