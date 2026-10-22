package proof

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/config"
)

func TestPropose(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		files  map[string]string
		source string
		checks map[string]config.Suite
		skips  []string
	}{
		{
			name: "lefthook, as git-board's own",
			files: map[string]string{"lefthook.yml": `pre-commit:
  commands:
    gofmt:
      glob: "*.go"
      run: gofmt -l {staged_files}
    vet:
      run: go vet ./...
    typecheck:
      root: web/
      run: npm run typecheck
    both:
      run: go vet ./... && npm test
commit-msg:
  commands:
    lint:
      run: commitlint
`},
			source: "lefthook.yml",
			checks: map[string]config.Suite{
				"vet":       {Run: []string{"go", "vet", "./..."}},
				"typecheck": {Run: []string{"npm", "run", "typecheck"}, Dir: "web", Files: []string{"web/**"}},
			},
			skips: []string{"lint", "both", "gofmt"},
		},
		{
			name:   "co2-calculator's lefthook.yml",
			files:  map[string]string{"lefthook.yml": string(must(os.ReadFile("testdata/co2-calculator/lefthook.yml")))},
			source: "lefthook.yml",
			checks: map[string]config.Suite{"type-check": {Run: []string{"make", "type-check"}}},
			skips:  []string{"commitlint", "issue-from-branch", "check-secrets", "format-lint-staged", "ci-validation-protected-branches"},
		},
		{
			name:   "a Makefile",
			files:  map[string]string{"Makefile": "lint: ## lint\n\tgolangci-lint run\ntypecheck:\n\ttsc\nLINT:=x\nfmt:\n\tgofmt -w .\n"},
			source: "Makefile",
			checks: map[string]config.Suite{"lint": {Run: []string{"make", "lint"}}, "typecheck": {Run: []string{"make", "typecheck"}}},
		},
		{
			name:   "web/package.json",
			files:  map[string]string{"web/package.json": `{"scripts": {"typecheck": "vue-tsc", "dev": "vite"}}`, "Makefile": "build:\n\tgo build\n"},
			source: "web/package.json",
			checks: map[string]config.Suite{"typecheck": {Run: []string{"npm", "run", "typecheck"}, Dir: "web", Files: []string{"web/**"}}},
		},
		{name: "nothing", checks: map[string]config.Suite{}},
	} {
		dir := t.TempDir()
		for p, body := range c.files {
			_ = os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755)
			if err := os.WriteFile(filepath.Join(dir, p), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		p, err := Propose(dir)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		var skipped []string
		for _, s := range p.Skipped {
			skipped = append(skipped, s.Name)
		}
		slices.Sort(skipped)
		slices.Sort(c.skips)
		if p.Source != c.source || !reflect.DeepEqual(p.Checks, c.checks) || !slices.Equal(skipped, c.skips) {
			t.Errorf("%s: %+v", c.name, p)
		}
	}
}

// The proposal written into a toml parses back to the same checks, and
// writing it twice changes nothing.
func TestWithChecksRoundTrip(t *testing.T) {
	t.Parallel()
	checks := map[string]config.Suite{
		"vet":       {Run: []string{"go", "vet", "./..."}},
		"typecheck": {Run: []string{"npm", "run", "typecheck"}, Dir: "web", Files: []string{"web/**"}},
	}
	before := "# board\n[cycle]\nstart = \"ready\"\n"
	text := config.WithChecks(before, checks)
	if config.WithChecks(text, checks) != text || !strings.HasPrefix(text, before) {
		t.Errorf("not idempotent, or the file's head moved:\n%s", text)
	}
	cfg, _, err := config.Check([]byte(text))
	if err != nil {
		t.Fatalf("%v\n%s", err, text)
	}
	if !reflect.DeepEqual(cfg.Cycle.Checks, checks) {
		t.Errorf("read back %+v\n%s", cfg.Cycle.Checks, text)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
