package cycle

import (
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/branches"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghpr"
	"github.com/guilbep/git-board/internal/markdown"
	"github.com/guilbep/git-board/internal/store"
)

// Pulls is the server's PR cache, read-only here.
type Pulls interface {
	// PR is the cached PR whose head is branch, and when it was read; nil
	// when none or never read.
	PR(branch string) (*ghpr.PR, time.Time)
}

// Gather fills an Input for the issue from git-bug, git and the PR cache
// (nil: none). me is the viewer's login.
func Gather(b *store.Backend, cfg *config.Config, th store.Thread, me string, pulls Pulls) (Input, error) {
	th.Inherited = cfg.RolesFor(th.Labels)
	stages := cfg.Stages()
	in := Input{
		Stages: stages,
		Status: board.FieldValue(th.Issue, "status"),
		Closed: th.Status == "closed",
		Risk:   board.FieldValue(th.Issue, "risk"),
		Kind:   cfg.Branches.Kind(th.AllLabels()),
		Me:     me,
		People: map[string]string{},
	}
	for _, l := range th.AllLabels() {
		if a, ok := strings.CutPrefix(l, "assignee:"); ok {
			in.Assignees = append(in.Assignees, a)
		}
	}
	// What the identity git-board writes as wrote is the viewer's, with or
	// without a login (--me names the viewer).
	myID := ""
	if p, ok, err := b.Me(); err == nil && ok {
		myID = p.ID
	}
	login := func(p store.Person) string {
		if me != "" && p.ID != "" && p.ID == myID {
			return me
		}
		return p.Login
	}
	in.Author = login(th.Author)
	if len(th.Events) > 0 && th.Events[0].Kind == "created" {
		in.Criteria, in.CriteriaProblems = Criteria(th.Events[0].Body, stages.Criteria)
	}
	holder := func(role string) string {
		if role == "" {
			return ""
		}
		return board.FieldValue(th.Issue, role)
	}
	in.People["risk"], in.People["plan"], in.People["review"] = holder(cfg.Cycle.Risk), holder(cfg.Cycle.Plan), holder(cfg.Cycle.Review)
	for status, role := range cfg.Cycle.Signoffs {
		in.People["signoff:"+status] = holder(role)
	}

	if err := gatherCode(b, cfg, th.Issue, stages, &in); err != nil {
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
		case m["verify:criterion"] != "":
			if i, err := strconv.Atoi(m["verify:criterion"]); err == nil {
				in.Checks = append(in.Checks, Check{Index: i, By: by, Commit: m["verify:commit"], At: at})
			}
		}
	}

	if pulls != nil {
		if in.Branch != "" {
			in.PR, in.PRAt = pulls.PR(in.Branch)
		}
		if in.PlanBranch != "" {
			in.PlanPR, _ = pulls.PR(in.PlanBranch)
		}
	}
	return in, nil
}

// gatherCode reads the branches naming the issue, the tests the evidence
// branch changed, and the plan file.
func gatherCode(b *store.Backend, cfg *config.Config, is store.Issue, stages config.Stages, in *Input) error {
	sug, err := branches.Suggest(b, cfg.Branches, is)
	if err != nil {
		return err
	}
	linked, err := branches.Linked(b, is, sug.Start)
	if err != nil {
		return err
	}
	in.Base = sug.Start
	ref := func(br branches.Branch) string { return strings.TrimPrefix(br.Remote+"/"+br.Name, "/") }
	var evidence, plan string
	var newest time.Time
	for _, br := range linked {
		switch {
		case strings.HasPrefix(br.Name, "plan/"):
			if in.PlanBranch == "" {
				in.PlanBranch, in.PlanHead, plan = br.Name, br.LastCommit.SHA, ref(br)
			}
		case evidence == "" || br.LastCommit.Date.After(newest): // local first on a tie
			in.Branch, in.Head, evidence, newest = br.Name, br.LastCommit.SHA, ref(br), br.LastCommit.Date
		}
	}

	if evidence != "" {
		files, err := b.ChangedFiles(in.Base, evidence)
		if err != nil {
			return err
		}
		for _, f := range files {
			if slices.ContainsFunc(stages.TestFiles, func(p string) bool { return matchTest(p, f) }) {
				in.Tests = append(in.Tests, TestFile{Path: f, Commit: b.FirstCommitAdding(in.Base, evidence, f)})
			}
		}
	}

	re := `^issue: *["']?([A-Za-z0-9]+-|#)?` + regexp.QuoteMeta(is.BareNumber()) + `["']? *$`
	if is.BareNumber() == "" {
		re = `^issue: *["']?` + PlanIssue(is)
	}
	for _, r := range []string{evidence, plan, in.Base} {
		if r == "" {
			continue
		}
		// a ref git can't read (no plans directory) is no plan there
		files, _ := b.GrepFiles(r, stages.Plans, re)
		if len(files) == 0 {
			continue
		}
		data, _ := b.ReadFile(r, files[0])
		// ponytail: links as the file writes them, plan files carry no uploads
		in.Plan = &Plan{Path: files[0], Ref: r, Status: frontMatter(string(data), "status"), Issue: frontMatter(string(data), "issue"), HTML: markdown.Render(body(string(data)), func(u string) string { return u })}
		if sha := b.FirstCommitAdding(in.Base, r, files[0]); sha != "" {
			if c, err := b.ReadCommit(sha); err == nil {
				in.Plan.By = c.Author
			}
		}
		break
	}
	return nil
}

// PlanIssue is what a plan file's front matter writes as issue: for
// gatherCode to find it: the issue's bare number, else its git-bug id's
// first 7 characters. The agent's prompt names it.
func PlanIssue(is store.Issue) string {
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
