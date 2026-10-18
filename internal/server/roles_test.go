package server

import (
	"testing"

	"github.com/guilbep/git-board/internal/config"
)

// The page learns each role's project value with its field, even before
// any issue sets the role.
func TestFieldsCarryRoles(t *testing.T) {
	ts, s := newIssueServer(t)
	s.cfg = &config.Config{Roles: map[string]string{"lead-dev": "bob"}}
	var fields map[string]struct {
		Type     string `json:"type"`
		Inherits string `json:"inherits"`
	}
	call(t, "GET", ts.URL+"/api/config/fields", nil, &fields)
	if f := fields["lead-dev"]; f.Type != config.TypePerson || f.Inherits != "bob" {
		t.Errorf("lead-dev: %+v", f)
	}
}
