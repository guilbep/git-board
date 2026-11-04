package core

import (
	"fmt"
	"os/exec"
	"slices"
	"sort"
	"strings"
	"testing"
)

// The core's layers, ADR 0020: each imports only its own and those below.
const (
	kernel = iota
	issues
	releases
	cycle
)

var layerNames = []string{"kernel", "issues", "releases", "cycle"}

// layerOf is each core package's layer, by the longest prefix that names
// it. A core package missing here fails TestEveryCorePackageHasALayer.
var layerOf = map[string]int{
	"core":                   kernel,
	"core/oplog":             kernel,
	"core/datastore":         kernel,
	"core/gitstore":          kernel,
	"core/pgstore":           kernel,
	"core/mdstore":           kernel,
	"core/people":            kernel,
	"core/watch":             kernel,
	"core/forge":             kernel,
	"core/internal":          kernel,
	"core/forge/conformance": issues, // the issue sync's suite, run per forge
	"core/issue":             issues,
	"core/index":             issues,
	"core/fields":            issues,
	"core/filter":            issues,
	"core/search":            issues,
	"core/inbox":             issues,
	"core/model":             issues,
	"core/syncer":            issues,
	"core/proof":             releases,
	"core/release":           releases,
}

const module = "github.com/guilbep/git-board/"

// layer is pkg's layer, by the longest prefix in layerOf; ok is false when
// none names it. "core" names the root package alone: as a prefix it
// would name every package, and a new one would never lack a layer.
func layer(pkg string) (l int, ok bool) {
	best := ""
	for p, pl := range layerOf {
		if (pkg == p || p != "core" && strings.HasPrefix(pkg, p+"/")) && len(p) > len(best) {
			best, l = p, pl
		}
	}
	return l, best != ""
}

// imports is each core package's imports of core packages, tests left out:
// a test may reach up to build its fixtures.
func imports(t *testing.T) map[string][]string {
	t.Helper()
	out, err := exec.Command("go", "list", "-f", `{{.ImportPath}} {{join .Imports " "}}`, "./...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	m := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(line)
		pkg := strings.TrimPrefix(f[0], module)
		m[pkg] = nil
		for _, imp := range f[1:] {
			if rel, ok := strings.CutPrefix(imp, module); ok && strings.HasPrefix(rel, "core") {
				m[pkg] = append(m[pkg], rel)
			}
		}
	}
	return m
}

func TestEveryCorePackageHasALayer(t *testing.T) {
	t.Parallel()
	var missing []string
	for pkg := range imports(t) {
		if _, ok := layer(pkg); !ok {
			missing = append(missing, pkg)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("give these a layer in layerOf (ADR 0020):\n%s", strings.Join(missing, "\n"))
	}
}

// violations is every import of a layer above the importer's.
func violations(deps map[string][]string) []string {
	var bad []string
	for pkg, imps := range deps {
		pl, ok := layer(pkg)
		if !ok {
			continue
		}
		for _, imp := range imps {
			if il, ok := layer(imp); ok && il > pl {
				bad = append(bad, fmt.Sprintf("%s (%s) imports %s (%s)", pkg, layerNames[pl], imp, layerNames[il]))
			}
		}
	}
	sort.Strings(bad)
	return bad
}

func TestLayersImportOnlyBelow(t *testing.T) {
	t.Parallel()
	if bad := violations(imports(t)); len(bad) > 0 {
		t.Fatalf("a layer imports one above it (ADR 0020):\n%s", strings.Join(bad, "\n"))
	}
}

func TestGuardSeesAPackageWithoutALayer(t *testing.T) {
	t.Parallel()
	if _, ok := layer("core/shiny"); ok {
		t.Error("core/shiny has a layer, and layerOf doesn't list it")
	}
	if l, ok := layer("core/forge/github"); !ok || l != kernel {
		t.Errorf("core/forge/github: %v, %v; want the kernel, by core/forge", l, ok)
	}
}

func TestGuardSeesALayerImportingUp(t *testing.T) {
	t.Parallel()
	deps := map[string][]string{
		"core/oplog":        {"core/datastore"},
		"core/people":       {"core/issue"},
		"core/issue":        {"core/oplog"},
		"core/forge/github": {"core/forge", "core/syncer"},
	}
	want := []string{
		"core/forge/github (kernel) imports core/syncer (issues)",
		"core/people (kernel) imports core/issue (issues)",
	}
	if got := violations(deps); !slices.Equal(got, want) {
		t.Fatalf("violations = %q, want %q", got, want)
	}
}
