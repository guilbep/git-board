package gitbugio

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/git-bug/git-bug/cache"
	bugent "github.com/git-bug/git-bug/entities/bug"
	"github.com/git-bug/git-bug/entity"
)

// Review is a comment's review metadata, the review:* keys the fork's sync
// mirrors to the pull request (docs/prd/work-unit.md, M4): a line comment
// (path, line, side, commit), a reply to one (reply-to and its anchor), a
// review (state), a pull request's comment the sync brought in (pr alone).
// nil for a plain comment.
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

// CommentWith comments on an issue as the user identity, with metadata
// (review:* keys), and returns the comment's id.
func (b *Backend) CommentWith(idPrefix, body string, metadata map[string]string) (string, error) {
	me, err := b.c.GetUserIdentity(context.Background())
	if err != nil {
		return "", err
	}
	var id entity.CombinedId
	err = b.write(idPrefix, func(bug *cache.BugCache) (err error) {
		id, _, err = bug.AddCommentRaw(me, time.Now().Unix(), body, nil, metadata)
		return err
	})
	return string(id), err
}

// SetCommentMetadata sets metadata on one of an issue's comments: resolves
// a thread (review:resolved), stamps its pull request (review:pr).
func (b *Backend) SetCommentMetadata(idPrefix, commentID string, metadata map[string]string) error {
	return b.write(idPrefix, func(bug *cache.BugCache) error {
		c, err := bug.Snapshot().SearchComment(entity.CombinedId(commentID))
		if err != nil {
			return ErrNoSuchComment
		}
		_, err = bug.SetMetadata(context.Background(), c.TargetId(), metadata)
		return err
	})
}

// CommentReview is the review metadata of one of an issue's comments, nil
// for a plain comment: the anchor a reply copies.
func (b *Backend) CommentReview(idPrefix, commentID string) (*Review, error) {
	bug, err := b.c.Bugs().ResolvePrefix(context.Background(), idPrefix)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", idPrefix, err)
	}
	snap := bug.Snapshot()
	c, err := snap.SearchComment(entity.CombinedId(commentID))
	if err != nil {
		return nil, ErrNoSuchComment
	}
	return reviewOf(onForges(snap, nil).meta[c.TargetId()]), nil
}

// StampPR sets review:pr on the issue's review comments that have none, of
// those whose commit mine tells: written before the branch had a pull
// request, they wait for it, as the sync sends a review comment where its
// review:pr says. It returns how many it stamped.
func (b *Backend) StampPR(idPrefix, pr string, mine func(commit string) bool) (int, error) {
	n := 0
	err := b.write(idPrefix, func(bug *cache.BugCache) error {
		snap := bug.Snapshot()
		meta := onForges(snap, nil).meta
		for _, op := range snap.Operations {
			m := meta[op.Id()]
			if _, ok := op.(*bugent.AddCommentOperation); !ok || m["review:commit"] == "" || m["review:pr"] != "" || !mine(m["review:commit"]) {
				continue
			}
			if _, err := bug.SetMetadata(context.Background(), op.Id(), map[string]string{"review:pr": pr}); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

// isReview tells a comment the sync never posts on an issue: one with a
// review:* key, or pulled from a pull request (sync:<remote>:pr-comment).
func isReview(meta map[string]string) bool {
	for k := range meta {
		if strings.HasPrefix(k, "review:") || strings.HasSuffix(k, ":pr-comment") {
			return true
		}
	}
	return false
}
