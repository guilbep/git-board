package release

import "testing"

func TestParseTag(t *testing.T) {
	v, ok := ParseTag("v1.4.20-rc.2")
	if !ok || v != (Version{1, 4, 20, 2}) {
		t.Fatalf("ParseTag(v1.4.20-rc.2) = %v, %v", v, ok)
	}
	if got := v.Tag(); got != "v1.4.20-rc.2" {
		t.Errorf("Tag() = %q", got)
	}
	if got := v.String(); got != "1.4.20-rc.2" {
		t.Errorf("String() = %q", got)
	}
	for _, no := range []string{"1.4.20", "v1.4", "v1.4.20-beta.1", "v1.4.20-rc.0", "chore-tag"} {
		if _, ok := ParseTag(no); ok {
			t.Errorf("parsed %q", no)
		}
	}
}

func TestLess(t *testing.T) {
	order := []string{"v1.4.19", "v1.4.20-rc.1", "v1.4.20-rc.2", "v1.4.20", "v1.5.0-rc.1"}
	for i := 0; i+1 < len(order); i++ {
		a, _ := ParseTag(order[i])
		b, _ := ParseTag(order[i+1])
		if !a.Less(b) {
			t.Errorf("%s not Less than %s", order[i], order[i+1])
		}
		if b.Less(a) {
			t.Errorf("%s Less than %s", order[i+1], order[i])
		}
	}
}

func TestBump(t *testing.T) {
	last := Version{1, 4, 19, 0}
	if got := Bump(last, []string{"fix", "chore"}, false); got != (Version{1, 4, 20, 0}) {
		t.Errorf("patch bump = %v", got)
	}
	if got := Bump(last, []string{"fix", "feat"}, false); got != (Version{1, 5, 0, 0}) {
		t.Errorf("minor bump = %v", got)
	}
	if got := Bump(last, []string{"fix"}, true); got != (Version{2, 0, 0, 0}) {
		t.Errorf("major bump = %v", got)
	}
}

func TestNextRC(t *testing.T) {
	taken := []Version{{1, 4, 20, 1}, {1, 4, 20, 2}, {1, 4, 19, 5}}
	if got := NextRC(Version{1, 4, 20, 0}, taken); got != (Version{1, 4, 20, 3}) {
		t.Errorf("NextRC 1.4.20 = %v", got)
	}
	if got := NextRC(Version{1, 5, 0, 0}, taken); got != (Version{1, 5, 0, 1}) {
		t.Errorf("NextRC 1.5.0 = %v", got)
	}
}

func TestDevVersion(t *testing.T) {
	if got := DevVersion(Version{1, 4, 19, 0}, 7, "8c41d2e0f9b1aa"); got != "1.4.20-dev.7.g8c41d2e" {
		t.Errorf("DevVersion = %q", got)
	}
	if got := DevVersion(Version{1, 4, 19, 0}, 7, "abc"); got != "1.4.20-dev.7.gabc" {
		t.Errorf("short sha DevVersion = %q", got)
	}
}
