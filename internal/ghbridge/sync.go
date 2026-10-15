package ghbridge

// Two-way sync of a GitHub Project's fields with git-board's labels.
//
// Each field of each issue is decided from three values: the Project's, the
// label's, and the one both last agreed on (the base, recorded in the bug's
// history by gitbugio.SetProjectBases). Only the side that changed since is
// taken; when both changed differently, the field is held, left alone on
// both sides, until someone resolves it.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/fields"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// Decision is what syncing one field does.
type Decision int

const (
	// Agreed: both sides have the same value.
	Agreed Decision = iota
	// TakeProject: only the Project changed; the label follows.
	TakeProject
	// TakeLocal: only git-board changed; the Project follows.
	TakeLocal
	// Held: both changed differently; a person decides.
	Held
)

func (d Decision) String() string {
	return [...]string{"agreed", "take project", "take local", "held"}[d]
}

// Decide decides one field from the Project's value, git-board's, and the
// one both last agreed on. "" is a value: an empty field, a missing label.
func Decide(project, local, base string, hasBase bool) Decision {
	switch {
	// first, so a run stopped halfway is recorded by the next one
	case project == local:
		return Agreed
	case !hasBase:
		// a field never set on the Project can't be told from one a
		// teammate cleared
		if project == "" {
			return Held
		}
		return TakeProject
	case local == base:
		return TakeProject
	case project == base:
		return TakeLocal
	default:
		return Held
	}
}

// ProjectMeta is what writing to a Project takes: its id, and its fields by
// name.
type ProjectMeta struct {
	ID     string
	Fields map[string]ProjectField
}

// ProjectField is a Project field: its id, and its options or iterations, by
// name, to their id.
type ProjectField struct {
	ID      string
	Options map[string]string
}

// The kinds of ProjectEdit.
const (
	EditOption    = "option"
	EditNumber    = "number"
	EditIteration = "iteration"
	EditClear     = "clear"
)

// ProjectEdit is one field written to one Project item.
type ProjectEdit struct {
	BugID   string
	ItemID  string
	Number  int
	Field   string // the git-board field
	Value   string // its value, "" to clear
	FieldID string
	Kind    string
	Arg     string // the option or iteration id, or the number
}

// Conflict is a field both sides changed differently since they last agreed.
type Conflict struct {
	BugID   string
	Number  int
	URL     string
	Title   string
	Field   string
	Base    string
	HasBase bool
	Local   string
	Project string
}

// SyncPlan is what a sync would do.
type SyncPlan struct {
	Changes   []IssueChange // label changes, one per matched issue
	Edits     []ProjectEdit
	Conflicts []Conflict
	// Bases are the values to record per bug once applied: the fields both
	// sides agree on afterwards, where that changed.
	Bases     map[string]map[string]string
	Unmatched []UnmatchedItem
}

// synced are the fields kept in step with the Project, with the Project's
// name for each and its raw value on an item.
var synced = []struct {
	name, project string
	value         func(Item) string
}{
	{"status", "Status", func(i Item) string { return i.Status }},
	{"prio", "Priority", func(i Item) string { return i.Priority }},
	{"mod", "Module", func(i Item) string { return normalizeModule(i.Module) }},
	{"est", "Est. hours", func(i Item) string { return formatEstHours(i.EstHours) }},
	{"sprint", "Sprint", sprintTitle},
}

// BasePrefix is where a Project's bases are recorded on bugs.
func BasePrefix(p config.GithubProject) string {
	return fmt.Sprintf("git-board:%s/%d:", strings.ToLower(p.Owner), p.Number)
}

// BuildSyncPlan decides every synced field of every matched issue, without
// writing anything. bases are the recorded bases, by bug id then field.
func BuildSyncPlan(items []Item, bugs []gitbugio.Issue, bases map[string]map[string]string, meta ProjectMeta, cfg *config.Config) SyncPlan {
	byURL := make(map[string]gitbugio.Issue, len(bugs))
	for _, b := range bugs {
		if b.GithubURL != "" {
			byURL[b.GithubURL] = b
		}
	}

	plan := SyncPlan{Bases: map[string]map[string]string{}}
	for _, item := range items {
		if item.Content.Type != "Issue" {
			plan.Unmatched = append(plan.Unmatched, UnmatchedItem{Number: item.Content.Number, URL: item.Content.URL, Title: item.Title,
				Reason: fmt.Sprintf("not an issue (content type %s), excluded", item.Content.Type), NotAnIssue: true})
			continue
		}
		bug, ok := byURL[item.Content.URL]
		if !ok {
			plan.Unmatched = append(plan.Unmatched, UnmatchedItem{Number: item.Content.Number, URL: item.Content.URL, Title: item.Title,
				Reason: "no matching git-bug bug (not pulled with `git bug bridge pull` yet?)"})
			continue
		}
		syncIssue(&plan, item, bug, bases[bug.ID], meta, cfg)
	}
	return plan
}

func syncIssue(plan *SyncPlan, item Item, bug gitbugio.Issue, bases map[string]string, meta ProjectMeta, cfg *config.Config) {
	change := IssueChange{BugID: bug.ID, GithubURL: bug.GithubURL, Number: item.Content.Number, Title: item.Title}
	newBases := map[string]string{}
	record := func(field, value string) {
		if old, ok := bases[field]; !ok || old != value {
			newBases[field] = value
		}
	}
	setLabel := func(field, value string) {
		res := labelChange(bug.Labels, field, value)
		change.Fields = append(change.Fields, res)
		change.Added = append(change.Added, res.Added...)
		change.Removed = append(change.Removed, res.Removed...)
	}

	for _, f := range synced {
		pf, onProject := meta.Fields[f.project]
		if !onProject {
			continue
		}
		raw := f.value(item)
		p, ok := toLabel(cfg, f.name, raw)
		if !ok {
			change.Warnings = append(change.Warnings, fmt.Sprintf("%s value %q from the Project has no matching option, field left alone", f.name, raw))
			continue
		}
		l, ok := localValue(bug.Labels, f.name)
		if !ok {
			change.Warnings = append(change.Warnings, fmt.Sprintf("several %s labels, field left alone", f.name))
			continue
		}
		base, hasBase := bases[f.name]

		switch Decide(p, l, base, hasBase) {
		case Agreed:
			record(f.name, p)
		case TakeProject:
			setLabel(f.name, p)
			record(f.name, p)
		case TakeLocal:
			kind, arg, ok := toProject(cfg, pf, f.name, l)
			if !ok {
				change.Warnings = append(change.Warnings, fmt.Sprintf("%s value %q has no option on the Project, not written", f.name, l))
				continue
			}
			plan.Edits = append(plan.Edits, ProjectEdit{BugID: bug.ID, ItemID: item.ID, Number: item.Content.Number,
				Field: f.name, Value: l, FieldID: pf.ID, Kind: kind, Arg: arg})
			record(f.name, l)
		case Held:
			plan.Conflicts = append(plan.Conflicts, Conflict{BugID: bug.ID, Number: item.Content.Number, URL: item.Content.URL,
				Title: item.Title, Field: f.name, Base: base, HasBase: hasBase, Local: l, Project: p})
		}
	}

	// ponytail: assignees only flow from the Project to the labels, as
	// changes since the last sync, so local additions stay; writing them
	// back is a write to the GitHub issue itself
	_, hasAssignees := bases["assignee"]
	added, removed, assigned := syncAssignees(item, bug.Labels, bases["assignee"], hasAssignees)
	if len(added)+len(removed) > 0 {
		change.Fields = append(change.Fields, FieldResult{Field: "assignee", Added: added, Removed: removed})
		change.Added = append(change.Added, added...)
		change.Removed = append(change.Removed, removed...)
	}
	record("assignee", assigned)

	if res, _ := planPlainLabel(bug.Labels, "category", item.Category); res.Field != "" {
		change.Fields = append(change.Fields, res)
		change.Added = append(change.Added, res.Added...)
	}

	plan.Changes = append(plan.Changes, change)
	if len(newBases) > 0 {
		plan.Bases[bug.ID] = newBases
	}
}

// labelChange sets a single field's label to value, or removes it for "".
func labelChange(labels []string, field, value string) FieldResult {
	res := FieldResult{Field: field, Value: value}
	if value != "" {
		_, res.Added, res.Removed = fields.Diff(labels, field, value, true)
		return res
	}
	for _, l := range labels {
		if strings.HasPrefix(l, field+":") {
			res.Removed = append(res.Removed, l)
		}
	}
	return res
}

// Resolve turns a held conflict into a plan keeping one side: git-board's
// value written to the Project (ours), or the Project's set on the label
// (theirs). Both then agree on it.
func Resolve(c Conflict, ours bool, item Item, bug gitbugio.Issue, meta ProjectMeta, cfg *config.Config) (SyncPlan, error) {
	change := IssueChange{BugID: bug.ID, GithubURL: bug.GithubURL, Number: c.Number, Title: c.Title}
	plan := SyncPlan{Bases: map[string]map[string]string{}}
	value := c.Project
	if ours {
		value = c.Local
		var pf ProjectField
		for _, f := range synced {
			if f.name == c.Field {
				pf = meta.Fields[f.project]
			}
		}
		kind, arg, ok := toProject(cfg, pf, c.Field, c.Local)
		if !ok {
			return SyncPlan{}, fmt.Errorf("%s value %q has no option on the Project", c.Field, c.Local)
		}
		plan.Edits = []ProjectEdit{{BugID: bug.ID, ItemID: item.ID, Number: c.Number, Field: c.Field,
			Value: c.Local, FieldID: pf.ID, Kind: kind, Arg: arg}}
	} else {
		res := labelChange(bug.Labels, c.Field, c.Project)
		change.Fields, change.Added, change.Removed = []FieldResult{res}, res.Added, res.Removed
	}
	plan.Changes = []IssueChange{change}
	plan.Bases[bug.ID] = map[string]string{c.Field: value}
	return plan, nil
}

// syncAssignees applies the Project's assignee changes since base to the
// labels, and returns the new base.
func syncAssignees(item Item, labels []string, base string, hasBase bool) (added, removed []string, newBase string) {
	project := slices.Sorted(slices.Values(item.Assignees))
	var before []string
	if hasBase && base != "" {
		before = strings.Split(base, ",")
	}
	have := map[string]bool{}
	for _, l := range labels {
		have[l] = true
	}
	for _, a := range project {
		if !slices.Contains(before, a) && !have["assignee:"+a] {
			added = append(added, "assignee:"+a)
		}
	}
	for _, a := range before {
		if !slices.Contains(project, a) && have["assignee:"+a] {
			removed = append(removed, "assignee:"+a)
		}
	}
	return added, removed, strings.Join(project, ",")
}

// localValue returns git-board's value of a single field, "" without a
// label; false with several.
func localValue(labels []string, field string) (string, bool) {
	var values []string
	for _, l := range labels {
		if v, ok := strings.CutPrefix(l, field+":"); ok {
			values = append(values, v)
		}
	}
	switch len(values) {
	case 0:
		return "", true
	case 1:
		return values[0], true
	}
	return "", false
}

// toLabel turns the Project's raw value of a field into git-board's value;
// false when no option matches, or nothing is left of it as a slug.
func toLabel(cfg *config.Config, field, raw string) (string, bool) {
	if raw == "" {
		return "", true
	}
	f := cfg.Fields[field]
	switch field {
	case "sprint":
		value := sprintSlug(cfg, raw)
		return value, value != ""
	case "est":
		return raw, f.KnownValue(raw)
	}
	for value, name := range f.Project {
		if name == raw {
			return value, true
		}
	}
	// "-" slugs to nothing: not a value, and not "cleared" either
	value := slugify(raw)
	return value, value != "" && f.KnownValue(value)
}

// sprintSlug is the label value of a Project iteration: its [[sprints.list]]
// slug when listed (that's where @current finds its dates), else its title
// as a slug, so a new iteration needs no config.
func sprintSlug(cfg *config.Config, title string) string {
	for _, s := range cfg.Sprints.List {
		if strings.EqualFold(strings.TrimSpace(s.Title), strings.TrimSpace(title)) {
			return s.Slug
		}
	}
	return slugify(title)
}

// toProject turns git-board's value of a field into a Project edit; false
// when the Project has no option for it.
func toProject(cfg *config.Config, pf ProjectField, field, value string) (kind, arg string, ok bool) {
	if value == "" {
		return EditClear, "", true
	}
	switch field {
	case "est":
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return "", "", false
		}
		return EditNumber, value, true
	case "sprint":
		for _, title := range slices.Sorted(maps.Keys(pf.Options)) {
			if sprintSlug(cfg, title) == value {
				return EditIteration, pf.Options[title], true
			}
		}
		return "", "", false
	}
	if name, ok := cfg.Fields[field].Project[value]; ok {
		id, ok := pf.Options[name]
		return EditOption, id, ok
	}
	// sorted, so duplicate names pick the same option every run
	for _, name := range slices.Sorted(maps.Keys(pf.Options)) {
		if slugify(normalizeModule(name)) == value {
			return EditOption, pf.Options[name], true
		}
	}
	return "", "", false
}

// ProjectWriter writes one field of one Project item.
type ProjectWriter interface {
	Edit(projectID string, e ProjectEdit) error
}

// BaseWriter records what a Project and git-board agree on.
type BaseWriter interface {
	SetProjectBases(idPrefix, prefix string, values map[string]string) error
}

var _ BaseWriter = (*gitbugio.Backend)(nil)

// ApplySync applies a plan issue by issue: Project edits, then labels, then
// bases, so a run stopped halfway leaves agreeing values that the next run
// records. Without write in the config nothing is written to the Project,
// and the bases of those fields aren't recorded; it returns how many edits
// it skipped. It stops at the first failed write.
func ApplySync(labels LabelWriter, project ProjectWriter, bases BaseWriter, plan SyncPlan, cfg *config.Config, meta ProjectMeta) (skipped int, err error) {
	edits := map[string][]ProjectEdit{}
	for _, e := range plan.Edits {
		edits[e.BugID] = append(edits[e.BugID], e)
	}
	prefix := BasePrefix(cfg.GithubProject)

	for _, c := range plan.Changes {
		record := plan.Bases[c.BugID]
		for _, e := range edits[c.BugID] {
			if !cfg.GithubProject.Write {
				skipped++
				record = without(record, e.Field)
				continue
			}
			if err := project.Edit(meta.ID, e); err != nil {
				return skipped, fmt.Errorf("#%d %s: %w", e.Number, e.Field, err)
			}
		}
		if len(c.Added)+len(c.Removed) > 0 {
			if _, err := labels.ApplyLabelChange(c.BugID, c.Added, c.Removed); err != nil {
				return skipped, fmt.Errorf("#%d: %w", c.Number, err)
			}
		}
		if len(record) > 0 {
			if err := bases.SetProjectBases(c.BugID, prefix, record); err != nil {
				return skipped, fmt.Errorf("#%d: %w", c.Number, err)
			}
		}
	}
	return skipped, nil
}

func without(m map[string]string, key string) map[string]string {
	res := make(map[string]string, len(m))
	for k, v := range m {
		if k != key {
			res[k] = v
		}
	}
	return res
}

// LoadBases reads the recorded bases of the bugs a Project's items match.
func LoadBases(b interface {
	ProjectBases(idPrefix, prefix string) (map[string]string, error)
}, prefix string, items []Item, bugs []gitbugio.Issue) (map[string]map[string]string, error) {
	urls := map[string]bool{}
	for _, it := range items {
		urls[it.Content.URL] = true
	}
	res := map[string]map[string]string{}
	for _, bug := range bugs {
		if bug.GithubURL == "" || !urls[bug.GithubURL] {
			continue
		}
		bases, err := b.ProjectBases(bug.ID, prefix)
		if err != nil {
			return nil, err
		}
		res[bug.ID] = bases
	}
	return res, nil
}

// FetchProjectMeta reads a Project's id, and its fields' ids and options or
// iterations, with `gh api graphql`.
func FetchProjectMeta(owner string, number int) (ProjectMeta, error) {
	const query = `query($owner:String!,$n:Int!){repositoryOwner(login:$owner){... on ProjectV2Owner{projectV2(number:$n){id fields(first:100){nodes{
		... on ProjectV2FieldCommon{id name}
		... on ProjectV2SingleSelectField{options{id name}}
		... on ProjectV2IterationField{configuration{iterations{id title} completedIterations{id title}}}}}}}}}`
	out, err := gh("api", "graphql", "-f", "owner="+owner, "-F", "n="+strconv.Itoa(number), "-f", "query="+query)
	if err != nil {
		return ProjectMeta{}, err
	}
	type named struct{ ID, Name, Title string }
	var resp struct {
		Data struct {
			RepositoryOwner struct {
				ProjectV2 *struct {
					ID     string
					Fields struct {
						Nodes []struct {
							ID, Name      string
							Options       []named
							Configuration struct{ Iterations, CompletedIterations []named }
						}
					}
				}
			}
		}
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return ProjectMeta{}, fmt.Errorf("reading the Project's fields: %w", err)
	}
	p := resp.Data.RepositoryOwner.ProjectV2
	if p == nil {
		return ProjectMeta{}, fmt.Errorf("no Project %d for %s", number, owner)
	}
	meta := ProjectMeta{ID: p.ID, Fields: map[string]ProjectField{}}
	for _, f := range p.Fields.Nodes {
		field := ProjectField{ID: f.ID, Options: map[string]string{}}
		for _, o := range f.Options {
			field.Options[o.Name] = o.ID
		}
		for _, it := range append(f.Configuration.Iterations, f.Configuration.CompletedIterations...) {
			field.Options[it.Title] = it.ID
		}
		meta.Fields[f.Name] = field
	}
	return meta, nil
}

// GhWriter writes Project fields with `gh project item-edit`.
type GhWriter struct{}

func (GhWriter) Edit(projectID string, e ProjectEdit) error {
	args := []string{"project", "item-edit", "--id", e.ItemID, "--project-id", projectID, "--field-id", e.FieldID}
	switch e.Kind {
	case EditOption:
		args = append(args, "--single-select-option-id", e.Arg)
	case EditNumber:
		args = append(args, "--number", e.Arg)
	case EditIteration:
		args = append(args, "--iteration-id", e.Arg)
	case EditClear:
		args = append(args, "--clear")
	default:
		return fmt.Errorf("unknown edit kind %q", e.Kind)
	}
	_, err := gh(args...)
	return err
}

func gh(args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh %s: %w: %s", strings.Join(args[:2], " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
