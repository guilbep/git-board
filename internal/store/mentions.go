package store

import "github.com/guilbep/git-board/core/model"

// Mentioning is the issues whose description or a comment mentions login,
// by id: the full-text index narrows (it finds the login anywhere, in an
// email too), the mention pattern confirms on their text.
func (b *Backend) Mentioning(login string) (map[string]bool, error) {
	res := map[string]bool{}
	if login == "" {
		return res, nil
	}
	hits, err := b.SearchText(login)
	if err != nil {
		return nil, err
	}
	for id := range hits {
		is, _, err := b.loadIssue(id)
		if err != nil {
			return nil, err
		}
		mentions := model.Mentions(is.Body, login)
		for _, c := range is.Comments {
			mentions = mentions || model.Mentions(c.Body, login)
		}
		if mentions {
			res[id] = true
		}
	}
	return res, nil
}
