package gitbugio

import (
	"strconv"

	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
)

// Review is a comment's review metadata, its review:* keys
// (docs/prd/work-unit.md, M4): a line comment (path, line, side, commit), a
// reply to one (reply-to and its anchor), a review (state). They stay in
// git-board: mirroring them to pull requests is out of version 1, and a
// pr is only what the fork's sync left. nil for a plain comment.
type Review struct {
	Path     string `json:"path,omitempty"`
	Line     int    `json:"line,omitempty"`
	Side     string `json:"side,omitempty"`   // old | new
	Commit   string `json:"commit,omitempty"` // the full SHA the comment was written on
	PR       string `json:"pr,omitempty"`     // github.com/o/r#26
	ReplyTo  string `json:"reply_to,omitempty"`
	State    string `json:"state,omitempty"`    // APPROVE | REQUEST_CHANGES | COMMENT
	Resolved bool   `json:"resolved,omitempty"` // the last review:resolved set on it
	// filled by the server: the issue's branch whose head has Commit, and,
	// from that head, whether a change since covers the line, where the
	// line is at head, and its text at Commit
	Branch   string `json:"branch,omitempty"`
	Outdated bool   `json:"outdated,omitempty"`
	HeadLine int    `json:"head_line,omitempty"`
	Text     string `json:"text,omitempty"`
}

// reviewOf reads a comment operation's review:* keys; a line that isn't a
// number is left out.
func reviewOf(meta map[string]string) *Review {
	r := Review{
		Path:     meta["review:path"],
		Side:     meta["review:side"],
		Commit:   meta["review:commit"],
		PR:       meta["review:pr"],
		ReplyTo:  meta["review:reply-to"],
		State:    meta["review:state"],
		Resolved: meta["review:resolved"] == "true",
	}
	r.Line, _ = strconv.Atoi(meta["review:line"])
	if r == (Review{}) {
		return nil
	}
	return &r
}

// CommentWith comments on an issue as you, with metadata (review:* keys),
// and returns the comment's id.
func (b *Backend) CommentWith(idPrefix, body string, metadata map[string]string) (string, error) {
	return b.AddCommentMeta(idPrefix, body, metadata)
}

// SetCommentMetadata sets metadata on one of an issue's comments: resolves
// a thread (review:resolved).
func (b *Backend) SetCommentMetadata(idPrefix, commentID string, metadata map[string]string) error {
	_, err := b.do(idPrefix, func(me string, is *issue.Issue, l *oplog.Log) ([]oplog.Op, error) {
		target, _, _, err := text(is, l, commentID)
		if err != nil {
			return nil, err
		}
		return []oplog.Op{issue.SetMeta(me, target, metadata)}, nil
	})
	return err
}

// CommentReview is the review metadata of one of an issue's comments, nil
// for a plain comment: the anchor a reply copies.
func (b *Backend) CommentReview(idPrefix, commentID string) (*Review, error) {
	is, l, err := b.issue(idPrefix)
	if err != nil {
		return nil, err
	}
	_, _, meta, err := text(is, l, commentID)
	if err != nil {
		return nil, err
	}
	return reviewOf(meta), nil
}
