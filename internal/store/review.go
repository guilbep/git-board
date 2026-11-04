package store

import (
	"strconv"

	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/model"
	"github.com/guilbep/git-board/core/oplog"
)

// reviewOf reads a comment operation's review:* keys; a line that isn't a
// number is left out.
func reviewOf(meta map[string]string) *model.Review {
	r := model.Review{
		Path:     meta["review:path"],
		Side:     meta["review:side"],
		Commit:   meta["review:commit"],
		PR:       meta["review:pr"],
		ReplyTo:  meta["review:reply-to"],
		State:    meta["review:state"],
		Resolved: meta["review:resolved"] == "true",
	}
	r.Line, _ = strconv.Atoi(meta["review:line"])
	if r == (model.Review{}) {
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
func (b *Backend) CommentReview(idPrefix, commentID string) (*model.Review, error) {
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

// CommentMeta is the metadata of one of an issue's comments, as set-meta
// left it: what a thread's checks read.
func (b *Backend) CommentMeta(idPrefix, commentID string) (map[string]string, error) {
	is, l, err := b.issue(idPrefix)
	if err != nil {
		return nil, err
	}
	_, _, meta, err := text(is, l, commentID)
	return meta, err
}
