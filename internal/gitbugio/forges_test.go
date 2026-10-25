package gitbugio_test

import "testing"

// Get brings the forge's issues in and sends nothing; Send sends and gets
// nothing.
func TestGetAndSendAreOneWay(t *testing.T) {
	t.Skip("M5 phase 2")
}

// The token git-board acts on GitHub with is the one of whoever writes
// from it: its identity's GitHub login, as the bridge maps an author to an
// account, never another person's token.
func TestGitHubToken(t *testing.T) {
	t.Skip("M5 phase 2")
}
