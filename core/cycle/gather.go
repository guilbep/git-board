package cycle

import (
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/inbox"
	"github.com/guilbep/git-board/core/model"
)

// Sources is what Gather reads besides the thread: each is the app's to
// read (internal/evidence), the cycle computes from it and fetches none.
type Sources struct {
	Stages Stages
	Kind   string   // the issue's kind ([branches] kinds): step 4's rule
	Me     inbox.Me // the viewer: what its identity wrote is its login's, with or without a login
	// Base is where the issue's branches start: origin/<base>, or <base>
	// when only that exists.
	Base     string
	Branches []Branch // the branches naming the issue, against Base
	Repo     Repo     // their files and the plan file; nil reads none
	Pulls    Pulls    // nil: no PRs
	// Render is the plan's Markdown as HTML (Plan.HTML); nil leaves it "".
	Render func(markdown string) string
}

// Branch is one of the issue's branches against its base, as git has it.
type Branch struct {
	Name   string
	Remote string    // "" for a local branch
	Head   string    // its last commit
	At     time.Time // its last commit's date
	Ahead  int       // its commits the base hasn't
	Behind int       // the base's commits it hasn't
	Merged bool      // its head is in the base
	InBase bool      // its changes are in the base, merged or squashed or rebased in
}

// Repo is the git the evidence reads, beyond the branches: the files a
// branch changed since the base, the commit that first added one there,
// the files under a directory with a line matching a pattern, a file at
// a ref, and who wrote a commit.
type Repo interface {
	ChangedFiles(base, ref string) ([]string, error)
	FirstCommitAdding(base, ref, path string) string
	GrepFiles(ref, dir, pattern string) ([]string, error)
	ReadFile(ref, path string) ([]byte, error)
	Author(sha string) (string, error)
}

// Pulls is the forge's PRs by head branch, as the app keeps them (the
// server's cache), read-only here.
type Pulls interface {
	// PR is the PR whose head is branch, and when it was read; nil when
	// none or never read.
	PR(branch string) (*forge.PR, time.Time)
}

// Gather is the issue's Input: what its thread says (its fields, its
// people, its criteria, its comments' metadata), its branches as lanes
// with the tests their diffs changed, its plan file, and its PRs.
// PickLane makes one lane the evidence. th.Inherited is the roles it
// inherits, the app's to fill.
func Gather(th model.Thread, src Sources) (Input, error) {
	stages := src.Stages
	me := src.Me.Login
	in := Input{
		Stages: stages,
		Status: field(th.Issue, "status"),
		Closed: th.Status == "closed",
		Risk:   field(th.Issue, "risk"),
		Kind:   src.Kind,
		Me:     me,
		People: map[string]string{},
	}
	for _, l := range th.AllLabels() {
		if a, ok := strings.CutPrefix(l, "assignee:"); ok {
			in.Assignees = append(in.Assignees, a)
		}
	}
	login := func(p model.Person) string {
		if me != "" && p.ID != "" && p.ID == src.Me.ID {
			return me
		}
		return p.Login
	}
	in.Author = login(th.Author)
	if len(th.Events) > 0 && th.Events[0].Kind == "created" {
		in.Criteria, in.CriteriaProblems = Criteria(th.Events[0].Body, stages.Criteria)
	}
	// each gate's person: the issue's value of the field its role names
	holder := func(role string) string {
		if role == "" {
			return ""
		}
		return field(th.Issue, role)
	}
	for _, gate := range []string{"risk", "plan", "review", "spec"} {
		in.People[gate] = holder(stages.Roles[gate])
	}
	for gate, role := range stages.Roles {
		if strings.HasPrefix(gate, "signoff:") {
			in.People[gate] = holder(role)
		}
	}

	if err := gatherCode(src, th.Issue, &in); err != nil {
		return in, err
	}

	for _, e := range th.Events {
		m, by, at := e.Meta, login(e.Author), e.At
		switch {
		case e.Kind != "comment" || m == nil:
		case m["plan:approve"] != "":
			if p := in.People["plan"]; p == "" || strings.EqualFold(p, by) {
				in.PlanApproval = &Approval{By: by, Commit: m["plan:commit"], At: at}
			}
		case m["signoff:status"] != "":
			in.Signoffs = append(in.Signoffs, Signoff{Status: m["signoff:status"], By: by, Commit: m["signoff:commit"], Why: m["signoff:why"], At: at})
		case m["verify:run"] != "":
			in.Runs = append(in.Runs, Run{By: by, Commit: m["verify:commit"], At: at})
		case m["intent:criteria"] == "none":
			in.NoCriteria = true
		case m["verify:criterion"] != "":
			if i, err := strconv.Atoi(m["verify:criterion"]); err == nil {
				in.Checks = append(in.Checks, Check{Index: i, By: by, Commit: m["verify:commit"], At: at})
			}
		}
	}

	if src.Pulls != nil {
		for i := range in.Lanes {
			in.Lanes[i].PR, in.Lanes[i].PRAt = src.Pulls.PR(in.Lanes[i].Branch)
		}
		if in.PlanBranch != "" {
			in.PlanPR, _ = src.Pulls.PR(in.PlanBranch)
		}
	}
	return PickLane(in), nil
}

// field is the issue's value of a field: its first name:value label, its
// own or inherited; "" for none.
func field(is model.Issue, name string) string {
	for _, l := range is.AllLabels() {
		if v, ok := strings.CutPrefix(l, name+":"); ok {
			return v
		}
	}
	return ""
}

// gatherCode makes the branches naming the issue lanes, each with the
// tests its diff changed, and finds the plan file. PickLane, once the PRs
// are in, makes one the evidence.
func gatherCode(src Sources, is model.Issue, in *Input) error {
	in.Base = src.Base
	ref := func(br Branch) string { return strings.TrimPrefix(br.Remote+"/"+br.Name, "/") }
	var plan string
	seen := map[string]int{} // a branch's local and remote copies are one lane: the newer, local on a tie
	shared, work := map[string]bool{}, map[string]bool{}
	for _, br := range src.Branches {
		if strings.HasPrefix(br.Name, "plan/") {
			if in.PlanBranch == "" {
				in.PlanBranch, in.PlanHead, plan = br.Name, br.Head, ref(br)
			}
			continue
		}
		shared[br.Name] = shared[br.Name] || br.Remote != ""
		work[br.Name] = work[br.Name] || br.Ahead > 0
		// in the base: merged and the base moved on, or squashed in (a
		// branch still at the base's commit is a new one, not merged work)
		l := Lane{Branch: br.Name, Head: br.Head, At: br.At, Ref: ref(br), InBase: br.Merged && br.Behind > 0 || br.InBase && !br.Merged}
		if i, ok := seen[br.Name]; ok {
			if l.At.After(in.Lanes[i].At) {
				in.Lanes[i] = l
			}
			continue
		}
		seen[br.Name] = len(in.Lanes)
		in.Lanes = append(in.Lanes, l)
	}
	// a lane is shared (on origin, made there by Create too) or holds
	// work: a branch only here with nothing in it is no one's lane yet,
	// and holds no issue back
	in.Lanes = slices.DeleteFunc(in.Lanes, func(l Lane) bool { return !shared[l.Branch] && !work[l.Branch] && !l.InBase })
	// newest first: the plan file is looked for in the newest lane first
	slices.SortStableFunc(in.Lanes, func(a, b Lane) int { return b.At.Compare(a.At) })

	r := src.Repo
	if r == nil {
		return nil
	}
	for i := range in.Lanes {
		l := &in.Lanes[i]
		files, err := r.ChangedFiles(in.Base, l.Ref)
		if err != nil {
			return err
		}
		for _, f := range files {
			if slices.ContainsFunc(in.Stages.TestFiles, func(p string) bool { return matchTest(p, f) }) {
				l.Files = append(l.Files, TestFile{Path: f, Commit: r.FirstCommitAdding(in.Base, l.Ref, f)})
			}
		}
	}

	re := `^issue: *["']?([A-Za-z0-9]+-|#)?` + regexp.QuoteMeta(is.BareNumber()) + `["']? *$`
	if is.BareNumber() == "" {
		re = `^issue: *["']?` + PlanIssue(is)
	}
	refs := make([]string, 0, len(in.Lanes)+2)
	for _, l := range in.Lanes {
		refs = append(refs, l.Ref)
	}
	for _, at := range append(refs, plan, in.Base) {
		if at == "" {
			continue
		}
		// a ref git can't read (no plans directory) is no plan there
		files, _ := r.GrepFiles(at, in.Stages.Plans, re)
		if len(files) == 0 {
			continue
		}
		data, _ := r.ReadFile(at, files[0])
		in.Plan = &Plan{Path: files[0], Ref: at, Status: frontMatter(string(data), "status"), Issue: frontMatter(string(data), "issue")}
		if src.Render != nil {
			in.Plan.HTML = src.Render(body(string(data)))
		}
		if sha := r.FirstCommitAdding(in.Base, at, files[0]); sha != "" {
			if by, err := r.Author(sha); err == nil {
				in.Plan.By = by
			}
		}
		break
	}
	return nil
}

// PlanIssue is what a plan file's front matter writes as issue: for
// gatherCode to find it: the issue's bare number, else its git-bug id's
// first 7 characters. The agent's prompt names it.
func PlanIssue(is model.Issue) string {
	if n := is.BareNumber(); n != "" {
		return n
	}
	return is.ID[:7]
}

// body is the document below its front matter.
func body(doc string) string {
	lines := strings.Split(doc, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return doc
	}
	for i, l := range lines[1:] {
		if strings.TrimSpace(l) == "---" {
			return strings.TrimLeft(strings.Join(lines[i+2:], "\n"), "\n")
		}
	}
	return doc
}

// frontMatter is a key's value in a Markdown file's front matter: the
// lines between its first two --- lines.
func frontMatter(doc, key string) string {
	lines := strings.Split(doc, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "---" {
			break
		}
		if v, ok := strings.CutPrefix(l, key+":"); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

// matchTest tells whether a path is a test by the pattern: the pattern
// matches the path or its base name, or a dir/** pattern holds it.
func matchTest(pat, p string) bool {
	if dir, ok := strings.CutSuffix(pat, "/**"); ok {
		return strings.HasPrefix(p, dir+"/")
	}
	full, _ := path.Match(pat, p)
	base, _ := path.Match(pat, path.Base(p))
	return full || base
}
