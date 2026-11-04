package server

import (
	"testing"

	"github.com/guilbep/git-board/core/fields"
	"github.com/guilbep/git-board/internal/config"
)

// The page learns each role's project value with its field, even before
// any issue sets the role.
func TestFieldsCarryRoles(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	s.cfg = &config.Config{Schema: fields.Schema{Roles: map[string]string{"lead-dev": "bob"}}}
	var got map[string]struct {
		Type     string `json:"type"`
		Inherits string `json:"inherits"`
	}
	call(t, "GET", ts.URL+"/api/config/fields", nil, &got)
	if f := got["lead-dev"]; f.Type != fields.TypePerson || f.Inherits != "bob" {
		t.Errorf("lead-dev: %+v", f)
	}
}
