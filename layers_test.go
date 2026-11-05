package gitboard_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/guilbep/git-board/"

// mayImport is each package of the module and the packages of the module
// it may import (docs/rewrite/design.md, Packages; ADR 0021): the layers,
// drawn before the code grows on them. A package missing here fails the
// test: a new one takes its place in the table first. The table has no
// cycle (TestLayersHaveNoCycle), so no package imports one above it.
var mayImport = map[string][]string{
	"store":                       {},
	"kernel/pack":                 {},
	"kernel/search":               {},
	"kernel":                      {"store", "kernel/pack"},
	"store/git":                   {"store", "kernel/pack"},
	"store/mem":                   {"store", "kernel/pack"},
	"store/md":                    {"store", "kernel/pack"},
	"store/forgecopy":             {"store", "store/git"},
	"store/pg":                    {"store", "kernel/pack"},
	"store/pg/pgtest":             {},
	"store/storetest":             {"store", "kernel/pack"},
	"view":                        {"kernel", "kernel/search"},
	"view/viewtest":               {"kernel", "kernel/pack", "kernel/search", "store", "store/mem"},
	"conf":                        {},
	"domain/people":               {"kernel", "kernel/pack"},
	"sync":                        {"kernel", "kernel/pack", "store", "forge"},
	"domain/issues":               {"kernel", "kernel/pack", "kernel/search", "conf", "domain/people", "sync"},
	"domain/issues/issuesync":     {"kernel", "domain/issues", "domain/people", "sync", "forge"},
	"domain/issues/mdfiles":       {"kernel", "domain/issues", "domain/people"},
	"domain/releases":             {"kernel", "conf", "domain/people"},
	"domain/releases/proof":       {},
	"domain/releases/releasesync": {"domain/releases", "forge", "kernel", "sync"},
	"domain/cycle":                {"conf", "domain/issues", "domain/people", "domain/releases/proof", "forge"},
	"domain/cycle/work":           {"domain/cycle", "domain/issues", "domain/people"},
	"forge":                       {"conf"},
	"forge/fake":                  {"forge"},
	"forge/forgetest":             {"forge", "forge/fake", "kernel", "domain/issues", "domain/issues/issuesync", "domain/people", "store/git", "view"},
	"forge/github":                {"forge"},
	"forge/gitlab":                {"forge"},
	"hermetic":                    {},
	"tools":                       {"conf"},
	"tools/code":                  {},
	"tools/ghproject":             {"domain/issues", "domain/people", "forge", "forge/github", "kernel", "sync"},
	"tools/ghproject/projectfake": {},
	"tools/branches":              {"domain/issues", "tools/code"},
	"tools/ship":                  {"domain/releases", "domain/issues", "forge", "tools/branches", "tools/code", "domain/releases/proof"},
	"tools/ship/shiptest":         {"app", "domain/releases", "domain/issues/issuesync", "kernel", "sync", "tools/ship"},
	"app":                         {"kernel", "kernel/pack", "conf", "domain/issues", "domain/issues/mdfiles", "domain/people", "domain/releases", "domain/cycle", "forge", "forge/github", "forge/gitlab", "domain/issues/issuesync", "tools", "store", "store/git", "store/md", "store/pg", "store/forgecopy", "view", "sync", "domain/releases/releasesync", "tools/ship", "tools/branches", "tools/code", "domain/releases/proof", "tools/ghproject", "tools/insights", "domain/cycle/work", "tools/uploads"},
	"web":                         {},
	"surface/http":                {"web", "app", "conf", "kernel", "domain/issues", "domain/people", "forge", "tools/ghproject", "domain/releases", "tools/ship"},
	"surface/cli":                 {"app", "conf", "kernel", "domain/issues", "domain/people", "forge", "domain/releases", "tools/ship", "tools/branches", "surface/tui", "surface/http", "tools/ghproject"},
	"tools/insights":              {},
	"tools/boardgen":              {"kernel", "kernel/pack"},
	"tools/perfcheck":             {},
	"tools/flowgen":               {}, // reads the code as text, as this test does
	"tools/uploads":               {"tools"},
	"surface/tui":                 {"app", "domain/issues"},
	"cmd/git-board":               {"surface/cli"},
	"cmd/boardgen":                {"tools/boardgen"},
	"golden":                      {},
}

// testsMayImport is what a package's tests may import besides: the
// datastores and the view, to run on real ones, and the conformance suite.
var testsMayImport = []string{"tools/boardgen", "store", "store/git", "store/mem", "store/md", "store/pg", "store/pg/pgtest", "store/storetest", "view", "view/viewtest", "kernel", "kernel/pack", "hermetic", "forge/fake", "forge/forgetest", "tools/ship/shiptest", "tools/ghproject/projectfake"}

// contrib holds modules of their own (contrib/board-agents, ADR 0021):
// git-board imports nothing of them, and they nothing of git-board but
// through its CLI and HTTP API.
var contrib = []string{module + "contrib/", "github.com/guilbep/board-agents"}

// The layers hold: each package imports only what mayImport lets it, and
// nothing in or out of contrib/ crosses.
func TestLayers(t *testing.T) {
	t.Parallel()
	root := "."
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
		rel := filepath.ToSlash(path)
		pkg := filepath.ToSlash(filepath.Dir(rel))
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		inContrib := strings.HasPrefix(pkg, "contrib/")
		test := strings.HasSuffix(path, "_test.go")
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if slices.ContainsFunc(contrib, func(c string) bool { return strings.HasPrefix(p, c) }) && !inContrib {
				t.Errorf("%s imports %s: nothing imports contrib/", rel, p)
				continue
			}
			dep, ours := strings.CutPrefix(p, module)
			switch {
			case !ours:
			case inContrib:
				t.Errorf("%s imports %s: contrib/ speaks to git-board through its CLI and API only", rel, p)
			case pkg != "." && dep != pkg:
				allowed, known := mayImport[pkg]
				if !known {
					t.Errorf("%s: package %s has no place in the layers (mayImport)", rel, pkg)
				} else if !slices.Contains(allowed, dep) && !(test && slices.Contains(testsMayImport, dep)) {
					t.Errorf("%s imports %s: %s may import %v", rel, dep, pkg, allowed)
				}
			}
		}
		if pkg != "." && !inContrib {
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
	for pkg := range seen {
		if _, ok := mayImport[pkg]; !ok {
			t.Errorf("package %s has no place in the layers (mayImport)", pkg)
		}
	}
}

// TestLayersHaveNoCycle: the table is a stack of layers, nothing above
// reached from below. testsMayImport is apart: a test may use a datastore
// or a fake from anywhere.
func TestLayersHaveNoCycle(t *testing.T) {
	t.Parallel()
	const (
		visiting = 1
		done     = 2
	)
	state := map[string]int{}
	var visit func(pkg string, path []string)
	visit = func(pkg string, path []string) {
		switch state[pkg] {
		case visiting:
			t.Errorf("mayImport has a cycle: %s", strings.Join(append(path, pkg), " -> "))
			return
		case done:
			return
		}
		state[pkg] = visiting
		for _, dep := range mayImport[pkg] {
			if _, ok := mayImport[dep]; !ok {
				t.Errorf("mayImport: %s may import %s, which has no place in the table", pkg, dep)
				continue
			}
			visit(dep, append(path, pkg))
		}
		state[pkg] = done
	}
	for _, pkg := range slices.Sorted(maps.Keys(mayImport)) {
		visit(pkg, nil)
	}
}
