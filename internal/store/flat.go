package store

import (
	"slices"
	"strings"

	"github.com/guilbep/git-board/internal/config"
)

// SetFlatKinds tells the backend the kinds cfg stores as plain labels
// ([branches] kinds without [fields.kind] options): it then reads bug as
// kind:bug and writes kind:bug as bug, so everything above it sees one kind
// field and git-bug and the forges keep the plain label. None when cfg has
// kind:* labels. Safe beside reads: the server sets it on a save.
func (b *Backend) SetFlatKinds(cfg *config.Config) {
	if !cfg.FlatKinds() {
		b.flat.Store(nil)
		return
	}
	flat := map[string]bool{}
	for k := range cfg.Branches.Kinds {
		flat[k] = true
	}
	b.flat.Store(&flat)
}

// SetHeld tells the backend the labels the forges' sync keeps off each
// forge (config.Held): its runs, a switch's included, hold them, and a
// thread shows none waiting for that forge. Safe beside reads, as
// SetFlatKinds.
func (b *Backend) SetHeld(cfg *config.Config) {
	b.cfg.Store(cfg)
}

// held is SetHeld's config's Held, nil when none was set.
func (b *Backend) held() func(key, label string) bool {
	if cfg := b.cfg.Load(); cfg != nil {
		return cfg.Held
	}
	return nil
}

// heldOn says which labels the sync holds off the forge of an issue's URL.
// ponytail: the forge's key as the URL's host and path, Held reading the
// host alone
func (b *Backend) heldOn(u string) func(string) bool {
	held := b.held()
	_, key, _ := strings.Cut(u, "://")
	return func(l string) bool { return held != nil && held(key, l) }
}

// flatKind is the flat kind label l is, as kind:bug or as bug.
func (b *Backend) flatKind(l string) (string, bool) {
	flat := b.flat.Load()
	if flat == nil {
		return "", false
	}
	k := strings.TrimPrefix(l, "kind:")
	return k, (*flat)[k]
}

// assignee is an assignee label in any case, as sync reads it (Assignee:
// from a forge), spelled assignee:.
func assignee(l string) (string, bool) {
	const p = "assignee:"
	if len(l) > len(p) && strings.EqualFold(l[:len(p)], p) {
		return p + l[len(p):], true
	}
	return l, false
}

// shown are stored labels as git-board reads them: a flat kind's bug is
// kind:bug, once with a kind:bug stored beside it; Assignee:bob is
// assignee:bob, once.
func (b *Backend) shown(labels []string) []string {
	if len(labels) == 0 {
		return labels
	}
	res := make([]string, 0, len(labels))
	for _, l := range labels {
		if k, ok := b.flatKind(l); ok {
			l = "kind:" + k
		}
		l, _ = assignee(l)
		if !slices.Contains(res, l) {
			res = append(res, l)
		}
	}
	return res
}

// stored is a label change as git-bug keeps it, raw the labels it has now:
// a flat kind is added as bug, and removing it removes each spelling raw
// has (bug, or a kind:bug written before).
// ponytail: a kind:bug beside bug heals on the next change of kind only
func (b *Backend) stored(added, removed, raw []string) (a, r []string) {
	for _, l := range added {
		if as, ok := assignee(l); ok && slices.ContainsFunc(raw, func(x string) bool { y, _ := assignee(x); return y == as }) {
			continue // there, in some case
		}
		if k, ok := b.flatKind(l); ok {
			if slices.Contains(raw, k) || slices.Contains(a, k) {
				continue
			}
			l = k
		}
		a = append(a, l)
	}
	for _, l := range removed {
		if as, ok := assignee(l); ok {
			for _, x := range raw {
				if y, _ := assignee(x); y == as && !slices.Contains(r, x) {
					r = append(r, x)
				}
			}
			continue
		}
		k, ok := b.flatKind(l)
		if !ok {
			r = append(r, l)
			continue
		}
		for _, s := range []string{k, "kind:" + k} {
			if slices.Contains(raw, s) && !slices.Contains(r, s) {
				r = append(r, s)
			}
		}
	}
	return a, r
}
