package forge

import (
	"testing"
)

// A table's way states the mirror's direction (ADR 0022): push, pull, or
// both; both is the empty key's value, and anything else is refused.
func TestCheckForgeWay(t *testing.T) {
	t.Parallel()
	for way, ok := range map[string]bool{"": true, "push": true, "pull": true, "both": true, "send": false, "PUSH": false, "push-pull": false} {
		f := Table{Name: "gh", Kind: "github", Project: "o/r", Way: way}
		err := checkForge(&f)
		if ok != (err == nil) {
			t.Errorf("way %q: err = %v, want ok = %v", way, err, ok)
		}
		want := way
		if want == "" {
			want = WayBoth
		}
		if err == nil && f.Direction() != want {
			t.Errorf("way %q: Direction = %q, want %q", way, f.Direction(), want)
		}
	}
}
