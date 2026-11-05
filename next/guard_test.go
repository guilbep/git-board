package next

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

const module = "github.com/guilbep/git-board"

// v1gen writes the golden boards with v1's code: the guard's one
// exception (doc.go).
const v1gen = module + "/next/golden/v1gen"

// imports is every package of the module with what it imports, its tests'
// imports too.
func imports(t *testing.T) map[string][]string {
	t.Helper()
	cmd := exec.Command("go", "list", "-f", `{{.ImportPath}} {{join .Imports " "}} {{join .TestImports " "}} {{join .XTestImports " "}}`, module+"/...")
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			t.Fatalf("go list: %v\n%s", err, ee.Stderr)
		}
		t.Fatalf("go list: %v", err)
	}
	pkgs := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(line)
		pkgs[f[0]] = f[1:]
	}
	return pkgs
}

func in(pkg, tree string) bool {
	return pkg == module+"/"+tree || strings.HasPrefix(pkg, module+"/"+tree+"/")
}

// TestNextImportsNoV1 checks that next/ stands alone: none of its
// packages imports core/, internal/ or cmd/ (but v1gen), and no package
// outside it imports next/.
func TestNextImportsNoV1(t *testing.T) {
	t.Parallel()
	pkgs := imports(t)
	if _, ok := pkgs[module+"/next"]; !ok {
		t.Fatal("go list doesn't list next: is the module's path " + module + "?")
	}
	for pkg, deps := range pkgs {
		for _, dep := range deps {
			switch {
			case in(pkg, "next") && pkg != v1gen && (in(dep, "core") || in(dep, "internal") || in(dep, "cmd")):
				t.Errorf("%s imports %s: next/ imports nothing of v1 (next/doc.go)", pkg, dep)
			case !in(pkg, "next") && in(dep, "next"):
				t.Errorf("%s imports %s: nothing outside next/ imports it (next/doc.go)", pkg, dep)
			}
		}
	}
}

// TestGuardSeesBothWays checks the guard isn't blind: v1gen, its
// exception, does import v1.
func TestGuardSeesBothWays(t *testing.T) {
	t.Parallel()
	deps, ok := imports(t)[v1gen]
	if !ok {
		t.Fatalf("go list doesn't list %s", v1gen)
	}
	for _, dep := range deps {
		if in(dep, "core") {
			return
		}
	}
	t.Fatalf("%s imports nothing of core/: the guard's exception names the wrong package", v1gen)
}
