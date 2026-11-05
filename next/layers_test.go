package next_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/guilbep/git-board/"

// mayImport is each package of next/ and the packages of next/ it may
// import (docs/rewrite/design.md, Packages): the layers, drawn before the
// code grows on them. A package missing here fails the test: a new one
// takes its place in the table first.
var mayImport = map[string][]string{
	"next/store":                       {},
	"next/kernel/pack":                 {},
	"next/kernel/search":               {},
	"next/kernel":                      {"next/store", "next/kernel/pack"},
	"next/store/git":                   {"next/store", "next/kernel/pack"},
	"next/store/mem":                   {"next/store", "next/kernel/pack"},
	"next/store/md":                    {"next/store", "next/kernel/pack"},
	"next/store/forgecopy":             {"next/store", "next/store/git"},
	"next/store/pg":                    {"next/store", "next/kernel/pack"},
	"next/store/pg/pgtest":             {},
	"next/store/storetest":             {"next/store", "next/kernel/pack"},
	"next/view":                        {"next/kernel", "next/kernel/search"},
	"next/view/viewtest":               {"next/kernel", "next/kernel/pack", "next/kernel/search", "next/store", "next/store/mem"},
	"next/conf":                        {},
	"next/domain/people":               {"next/kernel", "next/kernel/pack"},
	"next/sync":                        {"next/kernel", "next/kernel/pack", "next/store", "next/forge"},
	"next/domain/issues":               {"next/kernel", "next/kernel/pack", "next/kernel/search", "next/conf", "next/domain/people", "next/sync"},
	"next/domain/issues/issuesync":     {"next/kernel", "next/domain/issues", "next/domain/people", "next/sync", "next/forge"},
	"next/domain/issues/mdfiles":       {"next/kernel", "next/domain/issues", "next/domain/people"},
	"next/domain/releases":             {"next/kernel", "next/conf", "next/domain/people"},
	"next/domain/releases/proof":       {},
	"next/domain/releases/releasesync": {"next/domain/releases", "next/forge", "next/kernel", "next/sync"},
	"next/domain/cycle":                {"next/conf", "next/domain/issues", "next/domain/people", "next/domain/releases/proof", "next/forge"},
	"next/domain/cycle/work":           {"next/domain/cycle", "next/domain/issues", "next/domain/people"},
	"next/forge":                       {"next/conf"},
	"next/forge/fake":                  {"next/forge"},
	"next/forge/forgetest":             {"next/forge", "next/forge/fake", "next/kernel", "next/domain/issues", "next/domain/issues/issuesync", "next/domain/people", "next/store/git", "next/view"},
	"next/forge/github":                {"next/forge"},
	"next/forge/gitlab":                {"next/forge"},
	"next/hermetic":                    {},
	"next/tools":                       {"next/conf"},
	"next/tools/code":                  {},
	"next/tools/branches":              {"next/domain/issues", "next/tools/code"},
	"next/tools/ship":                  {"next/domain/releases", "next/domain/issues", "next/forge", "next/tools/branches", "next/tools/code", "next/domain/releases/proof"},
	"next/tools/ship/shiptest":         {"next/app", "next/domain/releases", "next/domain/issues/issuesync", "next/kernel", "next/sync", "next/tools/ship"},
	"next/app":                         {"next/kernel", "next/kernel/pack", "next/conf", "next/domain/issues", "next/domain/issues/mdfiles", "next/domain/people", "next/domain/releases", "next/domain/cycle", "next/forge", "next/forge/github", "next/forge/gitlab", "next/domain/issues/issuesync", "next/tools", "next/store", "next/store/git", "next/store/md", "next/store/pg", "next/store/forgecopy", "next/view", "next/sync", "next/domain/releases/releasesync", "next/tools/ship", "next/tools/branches", "next/tools/code", "next/domain/releases/proof"},
	"next/surface/http":                {"next/app", "next/conf", "next/kernel", "next/domain/issues", "next/domain/people", "next/forge"},
	"next/surface/cli":                 {"next/app", "next/conf", "next/kernel", "next/domain/issues", "next/domain/people", "next/forge", "next/domain/releases", "next/tools/ship", "next/tools/branches", "next/surface/tui", "next/surface/http"},
	"next/surface/tui":                 {"next/app", "next/domain/issues"},
	"next/cmd/git-board-next":          {"next/surface/cli"},
	"next/golden":                      {},
	"next/golden/v1gen":                {"next/domain/issues", "next/domain/issues/issuesync", "next/domain/people", "next/forge"}, // its tests: the sync cross test
}

// v1gen writes the golden boards with v1's code (guard_test.go): the one
// package of next/ that imports v1, deleted with it.
const v1gen = "next/golden/v1gen"

// webApp is the web app's build, embedded (ADR 0012): not v1's code but
// the page both binaries serve, which outlives v1. The HTTP surface alone
// imports it.
const webApp, webServer = "web", "next/surface/http"

// testsMayImport is what a package's tests may import besides: the
// datastores and the view, to run on real ones, and the conformance suite.
var testsMayImport = []string{"next/store", "next/store/git", "next/store/mem", "next/store/md", "next/store/pg", "next/store/pg/pgtest", "next/store/storetest", "next/view", "next/view/viewtest", "next/kernel", "next/kernel/pack", "next/hermetic", "next/forge/fake", "next/forge/forgetest", "next/tools/ship/shiptest"}

// The layers hold, and next/ and v1 never import each other: deleting v1
// is one git rm (design.md, section 5).
func TestLayers(t *testing.T) {
	t.Parallel()
	root := ".."
	seen := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".") && path != root) {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		pkg := filepath.ToSlash(filepath.Dir(rel))
		inNext := pkg == "next" || strings.HasPrefix(pkg, "next/")
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		test := strings.HasSuffix(path, "_test.go")
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			dep, ours := strings.CutPrefix(p, module)
			if !ours {
				continue
			}
			toNext := dep == "next" || strings.HasPrefix(dep, "next/")
			switch {
			case !inNext && toNext:
				t.Errorf("%s imports %s: v1 never imports next/", rel, p)
			case inNext && dep == webApp && pkg == webServer: // the web app it serves
			case inNext && !toNext && pkg != v1gen:
				t.Errorf("%s imports %s: next/ copies what it needs from v1 and owns it", rel, p)
			case inNext && !toNext: // v1gen, the exception above
			case inNext && pkg != "next" && dep != pkg:
				allowed, known := mayImport[pkg]
				if !known {
					t.Errorf("%s: package %s has no place in the layers (mayImport)", rel, pkg)
				} else if !slices.Contains(allowed, dep) && !(test && slices.Contains(testsMayImport, dep)) {
					t.Errorf("%s imports %s: %s may import %v", rel, dep, pkg, allowed)
				}
			}
		}
		if inNext && pkg != "next" {
			seen[pkg] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for pkg := range mayImport {
		if !seen[pkg] {
			t.Errorf("mayImport names %s, which isn't there", pkg)
		}
	}
}
