package ghbridge

// Two-way sync of a GitHub Project's fields with git-board's labels.
//
// Each field of each issue is decided from three values: the Project's, the
// label's, and the one both last agreed on (the base, recorded in the bug's
// history by store.SetProjectBases). Only the side that changed since is
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
	"github.com/guilbep/git-board/internal/store"
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
// name; and its title, for Settings › Sync to show it answers.
type ProjectMeta struct {
	ID     string
	Title  string
	Fields map[string]ProjectField
	// Assignable is who can be assigned, by repository ("owner/name"):
	// GitHub silently drops anyone else. nil when not looked up.
	Assignable map[string][]string
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
	// EditAssignees edits the GitHub issue itself (URL), not a Project
	// field: assignees are the issue's.
	EditAssignees = "assignees"
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

	URL         string   // EditAssignees: the issue
	Add, Remove []string // EditAssignees: the logins
}

// Conflict is a field both sides changed differently since they last agreed.
type Conflict struct {
	BugID   string `json:"bug_id"`
	Number  int    `json:"number"`
	URL     string `json:"url"`
	Title   string `json:"title"`
	Field   string `json:"field"`
	Base    string `json:"base"`
	HasBase bool   `json:"has_base"`
	Local   string `json:"local"`   // git-board's value
	Project string `json:"project"` // the Project's
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
	{"moscow", "MoSCoW", func(i Item) string { return i.MoSCoW }},
	{"milestone", "Milestone", func(i Item) string {
		if i.Milestone == nil {
			return ""
		}
		return i.Milestone.Title
	}},
}

// readOnly are the synced fields only GitHub writes: the Project shows the
// issue's milestone, which a Project edit can't set. The Project's value
// always wins.
// ponytail: the title alone, as a slug; its due date and a write back
// (the issue's milestone, by title) when a project needs them.
var readOnly = map[string]bool{"milestone": true}

// SyncedFields are the names of the fields kept in step with the Project,
// in order: config.ProjectFields.
func SyncedFields() []string {
	var names []string
	for _, f := range synced {
		names = append(names, f.name)
	}
	return names
}

// BasePrefix is where a Project's bases are recorded on bugs.
func BasePrefix(p config.GithubProject) string {
	return fmt.Sprintf("git-board:%s/%d:", strings.ToLower(p.Owner), p.Number)
}

// BuildSyncPlan decides every synced field of every matched issue, without
// writing anything. bases are the recorded bases, by bug id then field.
func BuildSyncPlan(items []Item, bugs []store.Issue, bases map[string]map[string]string, meta ProjectMeta, cfg *config.Config) SyncPlan {
	byURL := make(map[string]store.Issue, len(bugs))
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

func syncIssue(plan *SyncPlan, item Item, bug store.Issue, bases map[string]string, meta ProjectMeta, cfg *config.Config) {
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

		d := Decide(p, l, base, hasBase)
		if readOnly[f.name] && d != Agreed {
			d = TakeProject
		}
		switch d {
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

	// Assignees merge as a set, person by person, so they never conflict.
	assignable, checked := meta.Assignable[repoOf(item.Content.URL)]
	a := syncAssignees(item, bug.Labels, bases["assignee"], cfg.GithubProject.Write)
	if checked {
		var refused []string
		a.ghAdd = slices.DeleteFunc(a.ghAdd, func(login string) bool {
			if slices.Contains(assignable, login) {
				return false
			}
			refused = append(refused, login)
			return true
		})
		for _, login := range refused {
			change.Warnings = append(change.Warnings, fmt.Sprintf("%s can't be assigned on GitHub (no access to %s), kept in git-board only", login, repoOf(item.Content.URL)))
			a.base = strings.Join(slices.DeleteFunc(strings.Split(a.base, ","), func(s string) bool { return s == login }), ",")
		}
	}
	if len(a.added)+len(a.removed) > 0 {
		change.Fields = append(change.Fields, FieldResult{Field: "assignee", Added: a.added, Removed: a.removed})
		change.Added = append(change.Added, a.added...)
		change.Removed = append(change.Removed, a.removed...)
	}
	if len(a.ghAdd)+len(a.ghRemove) > 0 {
		plan.Edits = append(plan.Edits, ProjectEdit{BugID: bug.ID, ItemID: item.ID, Number: item.Content.Number,
			Field: "assignee", Value: a.base, Kind: EditAssignees, URL: item.Content.URL, Add: a.ghAdd, Remove: a.ghRemove})
	}
	record("assignee", a.base)

	res, warning := planKind(cfg, bug.Labels, item.Labels, item.Category)
	if warning != "" {
		change.Warnings = append(change.Warnings, warning)
	}
	if res.Field != "" {
		change.Fields = append(change.Fields, res)
		change.Added = append(change.Added, res.Added...)
		change.Removed = append(change.Removed, res.Removed...)
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
func Resolve(c Conflict, ours bool, item Item, bug store.Issue, meta ProjectMeta, cfg *config.Config) (SyncPlan, error) {
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

// repoOf is "owner/name" of an issue URL.
func repoOf(issueURL string) string {
	parts := strings.Split(strings.TrimPrefix(issueURL, "https://github.com/"), "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

// assigneeSync is how one issue's assignees merge.
type assigneeSync struct {
	added, removed  []string // labels
	ghAdd, ghRemove []string // logins, on the GitHub issue
	base            string   // what both then hold, recorded as the base
}

// syncAssignees merges an issue's assignees person by person against base,
// what both sides last agreed on: a change on GitHub reaches the labels,
// a change in git-board reaches the issue — with write, else assignees only
// come from GitHub and base is GitHub's. With no base yet, both sides'
// assignees are kept.
func syncAssignees(item Item, labels []string, base string, write bool) assigneeSync {
	project := slices.Sorted(slices.Values(item.Assignees))
	var before []string
	if base != "" {
		before = strings.Split(base, ",")
	}
	var local []string
	for _, l := range labels {
		if a, ok := strings.CutPrefix(l, "assignee:"); ok {
			local = append(local, a)
		}
	}

	var res assigneeSync
	for _, a := range project {
		if !slices.Contains(before, a) && !slices.Contains(local, a) {
			res.added = append(res.added, "assignee:"+a)
		}
	}
	for _, a := range before {
		if !slices.Contains(project, a) && slices.Contains(local, a) {
			res.removed = append(res.removed, "assignee:"+a)
		}
	}
	if !write {
		res.base = strings.Join(project, ",")
		return res
	}
	for _, a := range local {
		if !slices.Contains(before, a) && !slices.Contains(project, a) {
			res.ghAdd = append(res.ghAdd, a)
		}
	}
	for _, a := range before {
		if slices.Contains(project, a) && !slices.Contains(local, a) {
			res.ghRemove = append(res.ghRemove, a)
		}
	}
	agreed := slices.Concat(project, res.ghAdd)
	agreed = slices.DeleteFunc(agreed, func(a string) bool { return slices.Contains(res.ghRemove, a) })
	slices.Sort(agreed)
	res.base = strings.Join(agreed, ",")
	return res
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
	value := config.Slug(raw)
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
	return config.Slug(title)
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
		if config.Slug(normalizeModule(name)) == value {
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

var _ BaseWriter = (*store.Backend)(nil)

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

// LabelsOnly is the plan's git-board side: its label changes, no Project
// edit. Each edit left out takes its base with it, as ApplySync drops an
// edit it skips: what isn't sent isn't agreed.
func (p SyncPlan) LabelsOnly() SyncPlan {
	q := p
	q.Edits, q.Bases = nil, maps.Clone(p.Bases)
	for _, e := range p.Edits {
		q.Bases[e.BugID] = without(q.Bases[e.BugID], e.Field)
	}
	return q
}

// ProjectOnly is the plan's Project side: its edits, no label change. Each
// label change left out takes its base with it.
func (p SyncPlan) ProjectOnly() SyncPlan {
	q := p
	q.Changes, q.Bases = make([]IssueChange, len(p.Changes)), maps.Clone(p.Bases)
	for i, c := range p.Changes {
		for _, f := range c.Fields {
			q.Bases[c.BugID] = without(q.Bases[c.BugID], f.Field)
		}
		c.Fields, c.Added, c.Removed = nil, nil, nil
		q.Changes[i] = c
	}
	return q
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
}, prefix string, items []Item, bugs []store.Issue) (map[string]map[string]string, error) {
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

// FetchProjectMeta reads a Project's id and title, and its fields' ids and
// options or iterations, with `gh api graphql`.
func FetchProjectMeta(owner string, number int) (ProjectMeta, error) {
	const query = `query($owner:String!,$n:Int!){repositoryOwner(login:$owner){... on ProjectV2Owner{projectV2(number:$n){id title fields(first:100){nodes{
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
					ID, Title string
					Fields    struct {
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
	meta := ProjectMeta{ID: p.ID, Title: p.Title, Fields: map[string]ProjectField{}}
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

// FetchAssignable lists who can be assigned on a repository ("owner/name").
func FetchAssignable(repo string) ([]string, error) {
	out, err := gh("api", "--paginate", "repos/"+repo+"/assignees?per_page=100", "--jq", ".[].login")
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
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
	case EditAssignees:
		args = []string{"issue", "edit", e.URL}
		if len(e.Add) > 0 {
			args = append(args, "--add-assignee", strings.Join(e.Add, ","))
		}
		if len(e.Remove) > 0 {
			args = append(args, "--remove-assignee", strings.Join(e.Remove, ","))
		}
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
