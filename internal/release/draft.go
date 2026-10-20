package release

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/guilbep/git-board/internal/branches"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// DraftIssue is an issue a release carries: one a merged branch names.
type DraftIssue struct {
	Number string `json:"number"` // as people say it: CO2-3075, #2518, else its id's first 7 characters
	Title  string `json:"title"`
	Kind   string `json:"kind"`  // config.Branches.Kind
	Merge  string `json:"merge"` // the merge commit that brought it
}

// Check is one thing a cut should hold.
type Check struct {
	OK     bool   `json:"ok"`
	Text   string `json:"text"`
	Detail string `json:"detail"` // why not, "" when OK
}

// Draft is what a cut would make, before it is made.
type Draft struct {
	From        string       `json:"from"`        // the followed env's commit, else the trunk head
	FromVersion string       `json:"fromVersion"` // what From runs as, else its dev version
	Undeployed  []string     `json:"undeployed"`  // trunk merges after From (follow only)
	Previous    *Tag         `json:"previous"`    // newest final reachable from From
	Proposed    Version      `json:"proposed"`    // Bump of Previous, final form
	Tag         string       `json:"tag"`         // what Cut would push: NextRC(...) or the final
	Issues      []DraftIssue `json:"issues"`
	Notes       string       `json:"notes"` // markdown, grouped by kind
	Checks      []Check      `json:"checks"`
}

// Draft drafts a cut of what the followed environment runs, else of the
// trunk's head: the next version from the issues merged since the previous
// final, the tag, the notes and the checks. final drafts the final rather
// than the next candidate; an app without candidate environments only cuts
// finals. It fetches unless Offline, and writes nothing.
func (r *Runner) Draft(ctx context.Context, final bool) (Draft, error) {
	st, err := r.Status(ctx)
	if err != nil {
		return Draft{}, err
	}
	d, err := r.from(ctx, st)
	if err != nil {
		return Draft{}, err
	}
	if d, err = r.draft(ctx, st, d, r.final(final)); err != nil {
		return Draft{}, err
	}
	d.Checks, err = r.checks(ctx, st, d.From, r.final(final))
	return d, err
}

// from is what a cut takes by default: what the followed environment
// runs, else the trunk's head at its dev version.
func (r *Runner) from(ctx context.Context, st Status) (Draft, error) {
	if e := envNamed(st, r.Cfg.Followed()); e.Commit != "" {
		return Draft{From: e.Commit, FromVersion: e.Version, Undeployed: e.Behind}, nil
	}
	v, err := r.devVersion(ctx, st.Head)
	return Draft{From: st.Head, FromVersion: v}, err
}

// draft fills in d, but its checks, for a cut of d.From (a full sha).
func (r *Runner) draft(ctx context.Context, st Status, d Draft, final bool) (Draft, error) {
	tags := tagsOf(st)
	for _, t := range tags { // newest first
		if t.Version.RC > 0 {
			continue
		}
		ok, err := IsAncestor(ctx, r.Dir, t.Commit, d.From)
		if err != nil {
			return d, err
		}
		if ok {
			d.Previous = &t
			break
		}
	}
	since, last := "", Version{}
	if d.Previous != nil {
		since, last = d.Previous.Commit, d.Previous.Version
	}
	issues, kinds, breaking, err := r.issues(ctx, since, d.From)
	if err != nil {
		return d, err
	}
	d.Issues, d.Notes = issues, notes(issues, r.Branches.Kinds)
	d.Proposed = Bump(last, kinds, breaking)
	d.Tag = tagFor(d.Proposed, final, tags).Tag()
	return d, nil
}

// Issues are the issues release tag t carries: those merged since the
// newest final of a lower version that t's commit contains, so a final
// carries its candidates'. It reads the local tags; it doesn't fetch.
func (r *Runner) Issues(ctx context.Context, t Tag) ([]DraftIssue, error) {
	tags, err := ReleaseTags(ctx, r.Dir)
	if err != nil {
		return nil, err
	}
	since := ""
	for _, p := range tags { // newest first
		if p.Version.RC != 0 || !p.Version.Less(t.Version.Final()) {
			continue
		}
		ok, err := IsAncestor(ctx, r.Dir, p.Commit, t.Commit)
		if err != nil {
			return nil, err
		}
		if ok {
			since = p.Commit
			break
		}
	}
	issues, _, _, err := r.issues(ctx, since, t.Commit)
	return issues, err
}

// issues are the issues named by the branches merged first-parent on to
// since from ("": all), hotfix/ branches aside, oldest first; kinds are
// their kinds, as labels and
// as branches say them (bug, fix), and breaking tells one is labelled so.
func (r *Runner) issues(ctx context.Context, from, to string) (issues []DraftIssue, kinds []string, breaking bool, err error) {
	ms, err := Merges(ctx, r.Dir, from, to)
	if err != nil || len(ms) == 0 {
		return nil, nil, false, err
	}
	all, err := r.B.ListIssues()
	if err != nil {
		return nil, nil, false, err
	}
	seen := map[string]bool{}
	for _, m := range ms {
		// a hotfix merged back carries no issue of its own: hotfix/1.0.2 would name #1
		if m.Branch == "" || strings.HasPrefix(m.Branch, "hotfix/") {
			continue
		}
		for _, is := range all {
			if seen[is.ID] || !branches.Names(m.Branch, is) {
				continue
			}
			seen[is.ID] = true
			k := r.Branches.Kind(is.Labels)
			issues = append(issues, DraftIssue{Number: number(is), Title: is.ShownTitle(), Kind: k, Merge: m.Commit})
			kinds = append(kinds, k, r.Branches.Kinds[k])
			breaking = breaking || slices.Contains(is.Labels, "breaking")
		}
	}
	return issues, kinds, breaking, nil
}

// number is how people name an issue: its project number, else its forge
// number, else its id's first 7 characters.
func number(is gitbugio.Issue) string {
	switch {
	case is.Number != "":
		return is.Number
	case is.BareNumber() != "":
		return "#" + is.BareNumber()
	}
	return is.ID[:min(7, len(is.ID))]
}

// notes are the issues as markdown, grouped by kind: features, then fixes,
// each read through kinds as a branch says it (bug as fix), then the other
// kinds by name, then the issues without one.
func notes(issues []DraftIssue, kinds map[string]string) string {
	rank := func(kind string) int {
		switch cmp.Or(kinds[kind], kind) {
		case "feat":
			return 0
		case "fix":
			return 1
		case "":
			return 3
		}
		return 2
	}
	sorted := slices.Clone(issues)
	slices.SortStableFunc(sorted, func(a, b DraftIssue) int {
		return cmp.Or(cmp.Compare(rank(a.Kind), rank(b.Kind)), strings.Compare(a.Kind, b.Kind))
	})
	var b strings.Builder
	for i, is := range sorted {
		if i == 0 || is.Kind != sorted[i-1].Kind {
			fmt.Fprintf(&b, "\n### %s\n\n", cmp.Or(is.Kind, "other"))
		}
		fmt.Fprintf(&b, "- %s %s\n", is.Number, is.Title)
	}
	return strings.TrimSpace(b.String())
}

// checks are what a cut of from holds, before it is made: it contains what
// the last environment runs, its images exist, and the environment it goes
// to moves forward.
func (r *Runner) checks(ctx context.Context, st Status, from string, final bool) ([]Check, error) {
	last, to := envNamed(st, r.Cfg.Last()), envNamed(st, r.deployTo(final))
	runs := "It contains what runs in " + last.Name
	if last.Adopted {
		d := last.Targets[0].Deployed
		runs += fmt.Sprintf(", %s (%.7s)", d.Version, d.Commit)
	}
	contains, err := r.containsCheck(ctx, last, from, runs)
	if err != nil {
		return nil, err
	}
	images, err := r.imagesCheck(ctx, st, from)
	if err != nil {
		return nil, err
	}
	forward, err := r.containsCheck(ctx, to, from, to.Name+" moves forward")
	if err != nil {
		return nil, err
	}
	return []Check{contains, images, forward}, nil
}

func (r *Runner) containsCheck(ctx context.Context, e EnvState, from, text string) (Check, error) {
	if !e.Adopted {
		return Check{Text: text, Detail: e.Name + " isn't adopted yet"}, nil
	}
	why, err := r.contains(ctx, e, from)
	return Check{OK: why == "", Text: text, Detail: why}, err
}

func (r *Runner) imagesCheck(ctx context.Context, st Status, from string) (Check, error) {
	rec, _, err := r.B.ReleaseFor(from)
	if err != nil {
		return Check{}, err
	}
	where, _ := r.where("any environment", deployedOf(st.Envs...)...) // an image in none is reported missing
	var regs, names, missing []string
	for _, img := range r.Cfg.Images {
		names = append(names, img.Name)
		if len(where[img.Name]) == 0 {
			missing = append(missing, img.Name+" (in no overlay)")
		}
		for _, reg := range where[img.Name] {
			if !slices.Contains(regs, reg) {
				regs = append(regs, reg)
			}
			d, err := r.existing(ctx, rec.Artifacts, img.Name, reg, from)
			if err != nil {
				return Check{}, fmt.Errorf("%s: %w", img.Name, err)
			}
			if d == "" {
				missing = append(missing, reg+"/"+img.Name)
			}
		}
	}
	c := Check{OK: len(missing) == 0, Text: fmt.Sprintf("Images for %.7s exist on %s: %s", from, strings.Join(regs, ", "), strings.Join(names, ", "))}
	if !c.OK {
		c.Detail = "missing " + strings.Join(missing, ", ")
	}
	return c, nil
}

// contains is why from doesn't contain what an adopted target of e runs,
// naming the hotfix branch that has it, or "" when it contains them all.
func (r *Runner) contains(ctx context.Context, e EnvState, from string) (string, error) {
	for _, t := range e.Targets {
		d := t.Deployed
		if d.Commit == "" {
			continue
		}
		ok, err := IsAncestor(ctx, r.Dir, d.Commit, from)
		if err != nil {
			return "", fmt.Errorf("%s on %s: %w", e.Name, t.Repo, err)
		}
		if ok {
			continue
		}
		why := fmt.Sprintf("it doesn't contain what runs in %s (%s)", e.Name, d.Version)
		remote := "refs/remotes/" + r.Cfg.RemoteName() + "/"
		out, err := Git(ctx, r.Dir, "for-each-ref", "--contains", d.Commit, "--format=%(refname)", "refs/heads/hotfix/", remote+"hotfix/")
		if err != nil {
			return "", err
		}
		if ref, _, _ := strings.Cut(out, "\n"); ref != "" {
			why += fmt.Sprintf(": merge %s into %s first", strings.TrimPrefix(strings.TrimPrefix(ref, "refs/heads/"), remote), r.base())
		}
		return why, nil
	}
	return "", nil
}

// followsLast tells whether the last environment follows the trunk: a
// single-environment app, whose cut tags what it runs and deploys nowhere.
// An app with no environment (binaries only) deploys nowhere too.
func (r *Runner) followsLast() bool {
	f := r.Cfg.Followed()
	return r.Cfg.Last() == "" || f != "" && f == r.Cfg.Last()
}

// final tells whether a cut is a final: asked for, or the app has no
// candidate environment.
func (r *Runner) final(asked bool) bool { return asked || len(r.Cfg.Candidates()) == 0 }

// deployTo is the environment a cut goes to: the first candidate one, the
// last for a final.
func (r *Runner) deployTo(final bool) string {
	if final {
		return r.Cfg.Last()
	}
	return r.Cfg.Candidates()[0]
}

// tagFor is what a cut of v pushes: its final, or its next candidate.
func tagFor(v Version, final bool, tags []Tag) Version {
	if final {
		return v.Final()
	}
	var taken []Version
	for _, t := range tags {
		taken = append(taken, t.Version)
	}
	return NextRC(v, taken)
}

func tagsOf(st Status) []Tag {
	tags := make([]Tag, len(st.Releases))
	for i, row := range st.Releases {
		tags[i] = row.Tag
	}
	return tags
}

func envNamed(st Status, name string) EnvState {
	for _, e := range st.Envs {
		if e.Name == name {
			return e
		}
	}
	return EnvState{Name: name}
}
