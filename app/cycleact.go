package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/guilbep/git-board/domain/cycle"
	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/domain/releases/proof"
	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/kernel"
	"github.com/guilbep/git-board/tools/branches"
	"github.com/guilbep/git-board/tools/ship"
)

// The cycle's actions (experimental; api.md route 50): what a step's
// person presses, v1's internal/server cycle, pulls and review handlers
// through the app. Each writes what its gate needs as a comment with
// metadata in its step's discussion (cycle:step), moves the status when
// the gate opens the next step, or acts on origin's forge (the pull
// requests); and answers the issue's cycle as it is after.

// CycleActions are the actions, as route 50 names them.
var CycleActions = []string{
	"no-criteria", "spec-verdict", "set-risk", "move-ready", "approve-plan", "start",
	"ready-for-review", "review", "resolve", "to-validation", "verify", "check", "signoff",
	"open-pull", "edit-pull", "merge", "refresh",
}

// CycleAct is one action on an issue's cycle; each action reads the
// fields it names.
type CycleAct struct {
	Name   string // the issue
	Me     string // the viewer's login; "" for the person who writes
	Action string

	Risk     string // set-risk: low, normal or structural
	Verdict  string // spec-verdict: approve or request-changes
	On       string // spec-verdict: the description's version, body@v<n>
	Body     string // spec-verdict, review: the comment; edit-pull: the PR's description
	Output   string // verify: a run's output
	Commit   string // verify, check, review: the commit (the evidence's head by default)
	Index    *int   // check: the criterion, from 0
	Why      string // signoff: the override's reason
	State    string // review: APPROVE, REQUEST_CHANGES or COMMENT; none for a line comment
	Path     string // review: a line comment's file
	Line     int    // review: its line
	Side     string // review: old or new
	ReplyTo  string // review: the line comment answered
	Comment  string // resolve: the thread's first comment
	Resolved bool   // resolve: true resolves, false reopens
	Branch   string // open-pull: the issue's branch on origin
	Number   int    // edit-pull, merge: the PR
	Title    string // edit-pull: its title
	SHA      string // merge: the head the page showed
}

// ErrNotYours is an action whose step waits for someone else.
func errNotYours(c cycle.Cycle) error {
	return &Error{Class: Forbidden, Err: fmt.Errorf("not yours: step %d waits for %s to %s", c.Step, cmp.Or(c.Waits.Login, "anyone"), c.Waits.What)}
}

// errUnproven is a gate the signed check results (ADR 0001) don't open.
func errUnproven(format string, args ...any) error {
	return &Error{Class: Usage, Err: fmt.Errorf(format, args...)}
}

// Act does one of the cycle's actions, and answers the cycle after it.
func (b *Board) Act(ctx context.Context, a CycleAct) (CycleView, error) {
	if !b.CycleOn() {
		return CycleView{}, ErrCycleOff
	}
	if !slices.Contains(CycleActions, a.Action) {
		return CycleView{}, &Error{Class: NotFound, Err: fmt.Errorf("no cycle action %q: %s", a.Action, strings.Join(CycleActions, ", "))}
	}
	if a.Action == "refresh" {
		return b.cycleRead(ctx, a.Name, a.Me, true)
	}
	cur, err := b.Cycle(ctx, a.Name, a.Me)
	if err != nil {
		return CycleView{}, err
	}
	id := cur.ID
	switch a.Action {
	case "open-pull", "edit-pull", "merge":
		err = b.pullAct(ctx, cur, a)
	case "review", "resolve":
		err = b.review(ctx, cur, a)
	default:
		err = b.gateAct(ctx, cur, a)
	}
	if err != nil {
		return CycleView{}, err
	}
	return b.Cycle(ctx, id, a.Me)
}

// may refuses an action the viewer's step doesn't offer.
func may(c cycle.Cycle, action string) error {
	if slices.Contains(c.Actions, action) {
		return nil
	}
	return errNotYours(c)
}

// waitsFor refuses when a gate has a person and it isn't the viewer.
func waitsFor(role, login, me string) error {
	if login == "" || strings.EqualFold(login, me) {
		return nil
	}
	return &Error{Class: Forbidden, Err: fmt.Errorf("waits for the %s role: %s", role, login)}
}

// gateAct writes a gate's comment, then moves the status when the gate
// opens the next step.
func (b *Board) gateAct(ctx context.Context, cur CycleView, a CycleAct) error {
	ev, stages, me := cur.Evidence, cur.Evidence.Stages, cur.Evidence.Me
	id := cur.ID
	atVerify := cur.Step == 7 && !cur.Done
	commit := cmp.Or(a.Commit, ev.Head)
	meta := map[string]string{}
	if cur.At != "" {
		meta["cycle:step"] = cur.At
	}
	body, status := "", "" // the comment, and the status after it
	switch a.Action {
	case "no-criteria":
		if err := may(cur.Cycle, a.Action); err != nil {
			return err
		}
		body, meta["intent:criteria"], meta["cycle:step"] = "Accepted without acceptance criteria", "none", "intent"
	case "spec-verdict":
		th, err := b.Issues.Show(ctx, id)
		if err != nil {
			return err
		}
		v := BodyVersion(th)
		if a.Verdict != "approve" && a.Verdict != "request-changes" {
			return usagef("verdict: approve or request-changes")
		}
		if a.On != fmt.Sprintf("body@v%d", v) {
			return &Error{Class: Changed, Code: "changed", Err: fmt.Errorf("v%d is published: give the verdict on it", v)}
		}
		if login := ev.People["spec"]; login != "" {
			if err := waitsFor(cmp.Or(stages.Roles["spec"], "spec"), login, me); err != nil {
				return err
			}
		} else if ev.Author != "" && strings.EqualFold(ev.Author, me) {
			return &Error{Class: Forbidden, Err: errors.New("the description's author does not approve it")}
		}
		text := strings.TrimSpace(a.Body)
		switch {
		case a.Verdict == "request-changes" && text == "":
			return usagef("say which changes")
		case a.Verdict == "approve" && len(ev.Criteria) == 0 && !ev.NoCriteria:
			return usagef("add acceptance criteria first")
		}
		head := fmt.Sprintf("**Approved v%d of the description**", v)
		if a.Verdict == "request-changes" {
			head = fmt.Sprintf("**Changes requested on v%d of the description**", v)
		}
		body = strings.TrimSuffix(head+"\n\n"+text, "\n\n")
		meta["spec:verdict"], meta["spec:on"], meta["cycle:step"] = a.Verdict, a.On, "intent"
		if a.Verdict == "approve" && cur.Step == 1 && stages.Ready != "" {
			status = stages.Ready
		}
	case "set-risk":
		if !slices.Contains([]string{"low", "normal", "structural"}, a.Risk) {
			return usagef("risk %q: low, normal or structural", a.Risk)
		}
		if err := waitsFor("risk", ev.People["risk"], me); err != nil {
			return err
		}
		_, err := b.Issues.Set(ctx, issues.SetReq{Name: id, Assign: []string{"risk=" + a.Risk}, Force: true})
		return err
	case "move-ready", "start", "to-validation":
		if err := may(cur.Cycle, a.Action); err != nil {
			return err
		}
		status = map[string]string{"move-ready": stages.Ready, "start": first(stages.InProgress), "to-validation": first(stages.Validation)}[a.Action]
		if status == "" {
			return usagef("no status to move to: [cycle] names none")
		}
	case "approve-plan":
		if err := waitsFor("plan", ev.People["plan"], me); err != nil {
			return err
		}
		if cur.Step != 3 || cur.Done {
			return usagef("not at Plan")
		}
		if ev.Risk == "structural" {
			return usagef("the plan's gate is approved on the forge")
		}
		body, meta["plan:approve"], meta["cycle:step"] = "Plan approved", "1", "plan"
		if c := cmp.Or(ev.Head, ev.PlanHead); c != "" {
			meta["plan:commit"] = c
		}
		if cur.Stage == "ready" {
			status = first(stages.InProgress)
		}
	case "ready-for-review":
		if err := may(cur.Cycle, a.Action); err != nil {
			return err
		}
		if len(stages.InReview) == 0 {
			return usagef("no in-review status: [cycle] names none")
		}
		// no branch is no code to prove: the gate is open
		if ev.Head != "" {
			g, err := b.gate(ctx, ev.Base, ev.Head)
			if err != nil {
				return err
			}
			if !g.Proven {
				return errUnproven("%s", g.Reason)
			}
		}
		status = stages.InReview[0]
	case "verify":
		if !atVerify {
			return usagef("not at Verify")
		}
		if strings.TrimSpace(a.Output) == "" {
			return usagef("a run needs its output")
		}
		fence := "```"
		for strings.Contains(a.Output, fence) {
			fence += "`"
		}
		body, meta["verify:run"] = "Run"+at(commit)+"\n\n"+fence+"\n"+strings.TrimRight(a.Output, "\n")+"\n"+fence, "1"
	case "check":
		if !atVerify {
			return usagef("not at Verify")
		}
		if a.Index == nil || *a.Index < 0 || *a.Index >= len(ev.Criteria) {
			return usagef("index: one of the %d acceptance criteria, from 0", len(ev.Criteria))
		}
		body, meta["verify:criterion"] = "Checked: "+ev.Criteria[*a.Index], strconv.Itoa(*a.Index)
	case "signoff":
		if err := waitsFor("signoff", ev.People["signoff:"+ev.Status], me); err != nil {
			return err
		}
		if !atVerify {
			return usagef("not at Verify")
		}
		if err := b.signoffProof(ctx, ev, a.Why); err != nil {
			return err
		}
		body, meta["signoff:status"] = "Signed off "+ev.Status+at(ev.Head), ev.Status
		if ev.Head != "" {
			meta["signoff:commit"] = ev.Head
		}
		if why := strings.TrimSpace(a.Why); why != "" {
			body, meta["signoff:why"] = body+"\n\n"+why, why
		}
		// the next validation status, or after the last one the closing one
		i := slices.Index(stages.Validation, ev.Status)
		status = stages.Closing
		if i >= 0 && i+1 < len(stages.Validation) {
			status = stages.Validation[i+1]
		}
	}
	if (a.Action == "verify" || a.Action == "check") && commit != "" {
		meta["verify:commit"] = commit
	}
	if body != "" {
		if _, _, err := b.Issues.CommentMeta(ctx, id, body, meta); err != nil {
			return err
		}
	}
	// a check ticks its box in the description too: a tick, not a version
	if a.Action == "check" {
		th, err := b.Issues.Show(ctx, id)
		if err != nil {
			return err
		}
		if d := th.Events[0]; d.Kind == "created" {
			if ticked, ok := cycle.TickCriterion(d.Body, stages.Criteria, *a.Index); ok {
				if err := b.Issues.EditText(ctx, id, kernel.OpID(d.ID), ticked, d.Body); err != nil {
					return err
				}
			}
		}
	}
	if status != "" && status != ev.Status {
		if _, err := b.Issues.Set(ctx, issues.SetReq{Name: id, Assign: []string{"status=" + status}}); err != nil {
			return err
		}
	}
	return nil
}

// signoffProof refuses a sign-off nothing proves, unless why says why:
// a run at the branch's head, or at the commit merged on the base; once
// merged on a base that declares checks, their signed results there
// (ADR 0001 §9), and a pasted run no longer counts.
func (b *Board) signoffProof(ctx context.Context, ev cycle.Input, why string) error {
	merged := ""
	if ev.PR != nil && ev.PR.State == "MERGED" {
		merged = ev.PR.MergeCommit
	}
	ran := slices.ContainsFunc(ev.Runs, func(r cycle.Run) bool {
		return ev.Head == "" || r.Commit == ev.Head || (merged != "" && r.Commit == merged)
	})
	override := strings.TrimSpace(why) != ""
	if merged != "" {
		g, err := b.gate(ctx, ev.Base, merged)
		if err != nil {
			g = proof.GateResult{Checks: []proof.CheckStatus{{}}, Reason: err.Error()}
		}
		if len(g.Checks) > 0 {
			if !g.Proven && !override {
				return errUnproven("not proven%s: %s; sign off with a reason to override", at(merged), g.Reason)
			}
			ran = true
		}
	}
	if !ran && !override {
		return errUnproven("no run%s; sign off with a reason to override", at(ev.Head))
	}
	return nil
}

// gate is the signed check results' verdict on commit, against the
// checks and signers committed on base (ADR 0001).
func (b *Board) gate(ctx context.Context, base, commit string) (proof.GateResult, error) {
	rules, err := AtBase(ctx, b.Dir, cmp.Or(base, b.BaseRef(ctx, b.Config.Branches.Base)))
	if err != nil {
		return proof.GateResult{}, err
	}
	return rules.Gate(ctx, b.Dir, commit)
}

// reviewStates are a review's verdicts, GitHub's words.
var reviewStates = []string{"APPROVE", "REQUEST_CHANGES", "COMMENT"}

// review writes a review on the whole change (state), a line comment
// (path, line, side) or a reply to one (reply_to), as comments with
// review:* metadata the sync mirrors; or resolves a thread (resolve).
func (b *Board) review(ctx context.Context, cur CycleView, a CycleAct) error {
	id := cur.ID
	if a.Action == "resolve" {
		meta, err := b.Issues.CommentMetaOf(ctx, id, kernel.OpID(a.Comment))
		if err != nil {
			return err
		}
		if meta["review:path"] == "" || meta["review:reply-to"] != "" {
			return usagef("resolve a thread on its first line comment")
		}
		return b.Issues.SetTextMeta(ctx, id, kernel.OpID(a.Comment), map[string]string{"review:resolved": strconv.FormatBool(a.Resolved)})
	}
	text := strings.TrimSpace(a.Body)
	meta := map[string]string{"cycle:step": cmp.Or(cur.At, "review")}
	commit := cmp.Or(a.Commit, cur.Evidence.Head)
	switch {
	case a.ReplyTo != "":
		parent, err := b.Issues.CommentMetaOf(ctx, id, kernel.OpID(a.ReplyTo))
		if err != nil {
			return err
		}
		if parent["review:path"] == "" {
			return usagef("a reply answers a line comment")
		}
		root := cmp.Or(parent["review:reply-to"], a.ReplyTo) // a reply answers its thread's first comment
		for _, k := range []string{"review:path", "review:line", "review:side", "review:commit", "review:pr"} {
			if parent[k] != "" {
				meta[k] = parent[k]
			}
		}
		meta["review:reply-to"] = root
		commit = ""
	case a.State != "":
		if !slices.Contains(reviewStates, a.State) {
			return usagef("state: APPROVE, REQUEST_CHANGES or COMMENT")
		}
		if a.Path != "" || a.Line != 0 || a.Side != "" {
			return usagef("a review is on the whole change: no path, line or side")
		}
		if text == "" { // the gate is a human who can explain the diff
			return usagef("explain the change in one line to approve it, or say what to change")
		}
		meta["review:state"] = a.State
	case a.Path != "":
		if a.Line < 1 || a.Side != "old" && a.Side != "new" {
			return usagef("a line comment needs a line and a side, old or new")
		}
		meta["review:path"], meta["review:line"], meta["review:side"] = a.Path, strconv.Itoa(a.Line), a.Side
	default:
		return usagef("a review needs a state, a line comment a path, a reply reply_to")
	}
	if text == "" {
		return usagef("an empty comment")
	}
	if commit != "" {
		sha, err := ship.Git(ctx, b.Dir, "rev-parse", "--verify", "--end-of-options", commit+"^{commit}")
		if err != nil {
			return usagef("commit %q: %v", commit, err)
		}
		meta["review:commit"] = sha
	} // no branch: a review of what the issue says, at no commit
	if pr := cur.Evidence.PR; pr != nil && pr.URL != "" && meta["review:pr"] == "" {
		meta["review:pr"] = prKey(pr.URL)
	}
	_, _, err := b.Issues.CommentMeta(ctx, id, a.Body, meta)
	return err
}

// prKey is a PR's URL as review:pr writes it: github.com/o/r#7.
var prURL = regexp.MustCompile(`^https?://([^/]+)/([^/]+)/([^/]+)/(?:pull|-/merge_requests)/([0-9]+)$`)

func prKey(u string) string {
	m := prURL.FindStringSubmatch(u)
	if m == nil {
		return ""
	}
	return m[1] + "/" + m[2] + "/" + m[3] + "#" + m[4]
}

// pullAct opens, edits or merges one of the issue's pull requests on
// origin's forge, as the person who writes; the cache takes the answer.
func (b *Board) pullAct(ctx context.Context, cur CycleView, a CycleAct) error {
	if b.PRs == nil {
		return ErrNoPuller
	}
	f, err := b.PRs.puller(ctx, b)
	if err != nil {
		return err
	}
	lanes := cur.Evidence.Lanes
	onOrigin := func(name string) bool {
		if !slices.ContainsFunc(lanes, func(l cycle.Lane) bool { return l.Branch == name }) {
			return false
		}
		_, err := ship.Git(ctx, b.Dir, "rev-parse", "--verify", "-q", "refs/remotes/"+NumbersRemote+"/"+name)
		return err == nil
	}
	switch a.Action {
	case "open-pull":
		if !onOrigin(a.Branch) {
			return usagef("%q isn't a branch of this issue on origin: push it first", a.Branch)
		}
		th, err := b.Issues.Show(ctx, cur.ID)
		if err != nil {
			return err
		}
		sug, err := branches.Suggest(ctx, b.Code(), b.Config.Branches, th.Issue)
		if err != nil {
			return err
		}
		pr, err := f.OpenPR(ctx, a.Branch, sug.Base, prTitle(th.Issue), prBody(th.Issue))
		if err != nil {
			return forgeErr(err)
		}
		b.PRs.Put(a.Branch, pr)
		return nil
	}
	// edit-pull, merge: one of the issue's lanes' PRs, found again
	for _, l := range lanes {
		pr, err := f.FindPR(ctx, l.Branch)
		if err != nil {
			return forgeErr(err)
		}
		if pr == nil || pr.Number != a.Number {
			continue
		}
		switch a.Action {
		case "edit-pull":
			if strings.TrimSpace(a.Title) == "" {
				return usagef("an edit needs a title")
			}
			if err := f.EditPR(ctx, a.Number, a.Title, a.Body); err != nil {
				return forgeErr(err)
			}
			pr.Title, pr.Body = a.Title, a.Body
		case "merge":
			if a.SHA == "" {
				return usagef("a merge needs the head commit shown (sha)")
			}
			if err := f.MergePR(ctx, a.Number, a.SHA); err != nil {
				return forgeErr(err)
			}
			if pr, err = f.FindPR(ctx, l.Branch); err != nil {
				return forgeErr(err)
			}
		}
		b.PRs.Put(l.Branch, pr)
		return nil
	}
	return usagef("#%d isn't the pull request of a branch of this issue", a.Number)
}

// forgeErr is a forge's answer as the app classes it: a refusal is the
// request's, the rest the forge's.
func forgeErr(err error) error {
	if errors.Is(err, forge.ErrRefused) || errors.Is(err, forge.ErrNotFound) {
		return &Error{Class: Usage, Err: err}
	}
	return err
}

// prTitle is the issue as lists show it: CO2-3075 and its title.
func prTitle(is issues.Issue) string { return strings.TrimSpace(is.Number + " " + is.Title) }

// prBody names what the pull request delivers: the issue by its project
// number, else by its id's first 7 characters. Never a closing keyword:
// merging doesn't finish the issue, its criteria do.
func prBody(is issues.Issue) string {
	return "Refs: " + cmp.Or(is.Number, is.ID[:min(7, len(is.ID))])
}

func first(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}

// at is " at " and a commit's first 7 characters; "" for none.
func at(sha string) string {
	if sha == "" {
		return ""
	}
	return " at " + sha[:min(7, len(sha))]
}

// unticked is a text with its task boxes unticked: a tick is no version.
var unticked = regexp.MustCompile(`(?m)^(\s*[-*+]\s+)\[[ xX]\]`)

// BodyVersion is the description's version, as the page counts it: 1 +
// each edit that changes the text past its ticks.
func BodyVersion(th issues.Thread) int {
	v := 1
	if len(th.Events) == 0 || th.Events[0].Kind != "created" {
		return v
	}
	h := th.Events[0].History
	for i := 1; i < len(h); i++ {
		if unticked.ReplaceAllString(h[i].Body, "$1[ ]") != unticked.ReplaceAllString(h[i-1].Body, "$1[ ]") {
			v++
		}
	}
	return v
}
