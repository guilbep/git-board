package forge

// PR is a pull request as the gate shows it: the code's way into a base
// branch, found by its head branch's name and never stored. The words are
// GitHub's (internal/ghpr reads them); a forge naming them otherwise maps
// to them. The cycle reads it as evidence (core/cycle).
type PR struct {
	Number      int      `json:"number"`
	URL         string   `json:"url"`
	State       string   `json:"state"` // OPEN, MERGED, CLOSED
	Draft       bool     `json:"draft"`
	Title       string   `json:"title"`
	Body        string   `json:"body"` // the description, Markdown
	Base        string   `json:"base"`
	Head        string   `json:"head"`                   // its head commit
	MergeCommit string   `json:"merge_commit,omitempty"` // on the base, once merged
	Author      string   `json:"author"`
	Decision    string   `json:"decision"`    // reviewDecision: APPROVED, REVIEW_REQUIRED, CHANGES_REQUESTED, "" (no review rule)
	Mergeable   string   `json:"mergeable"`   // MERGEABLE, CONFLICTING, UNKNOWN
	MergeState  string   `json:"merge_state"` // mergeStateStatus
	Checks      []Check  `json:"checks"`
	Reviews     []Review `json:"reviews"`   // latestOpinionatedReviews: a COMMENTED review hides no approval
	Requested   []string `json:"requested"` // the people asked to review
	// set by the gate (ghpr.Gate)
	CanMerge bool     `json:"can_merge"`
	Checking bool     `json:"checking"` // the forge is still computing mergeability
	Waits    []string `json:"waits"`
}

// Check is one of a PR's checks: a check run or a commit status.
type Check struct {
	Name     string `json:"name"`
	State    string `json:"state"` // success, failure, pending, expected (required, never reported)
	Required bool   `json:"required"`
	URL      string `json:"url,omitempty"`
}

// Review is a person's latest approval or request for changes on a PR,
// or a review the forge dismissed.
type Review struct {
	Login  string `json:"login"`
	State  string `json:"state"`  // APPROVED, CHANGES_REQUESTED, DISMISSED
	Commit string `json:"commit"` // the commit it was made at
}
