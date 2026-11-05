package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestVersion: --version prints v1's line, which make install-latest
// runs.
func TestVersion(t *testing.T) {
	t.Parallel()
	var out, errs bytes.Buffer
	if code := Main(context.Background(), []string{"--version"}, &out, &errs); code != 0 {
		t.Fatalf("--version: exit %d, %s", code, errs.String())
	}
	if got := out.String(); !strings.HasPrefix(got, "git-board version ") || !strings.HasSuffix(got, "\n") || strings.Count(got, "\n") != 1 {
		t.Errorf("--version printed %q, want one line \"git-board version <v>\"", got)
	}
}
