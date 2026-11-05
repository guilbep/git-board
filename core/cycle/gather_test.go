package cycle

import "testing"

func TestMatchTest(t *testing.T) {
	t.Parallel()
	for path, want := range map[string]bool{
		"internal/x/y_test.go": true, "web/src/a.test.ts": true, "tests/unit/t.py": true,
		"test_x.py": true, "src/tests.ts": false, "web/src/a.ts": false,
	} {
		matched := false
		for _, pat := range (Config{}).Stages(nil).TestFiles {
			matched = matched || matchTest(pat, path)
		}
		if matched != want {
			t.Errorf("%s: matched %v, want %v", path, matched, want)
		}
	}
}
