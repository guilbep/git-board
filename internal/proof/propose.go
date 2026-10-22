package proof

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/guilbep/git-board/internal/config"
)

// Proposal is the [cycle.checks.*] a repository's own hooks and scripts
// suggest: what "Propose checks" shows before it writes them.
type Proposal struct {
	Source  string                  `json:"source"` // the file read: lefthook.yml, Makefile, package.json; "" for none
	Checks  map[string]config.Suite `json:"checks"`
	Skipped []Skip                  `json:"skipped"`
}

// Skip is a command not proposed, and why.
type Skip struct {
	Name string `json:"name"`
	Run  string `json:"run"`
	Why  string `json:"why"`
}

// Propose reads dir's pre-commit hooks from lefthook.yml, else its
// Makefile's lint and type-check targets, else package.json's lint and
// type-check scripts. Only a command that is an argv runs the same in a
// clean checkout: one on the staged files, a shell line or a script is
// skipped with the reason. A proposed check reads every file, or its
// root's: narrowing files is the user's call.
// ponytail: three readers, a page each; a general one when a fourth
// source shows up.
func Propose(dir string) (Proposal, error) {
	for _, f := range []string{"lefthook.yml", ".lefthook.yml", "lefthook.yaml"} {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err == nil {
			return fromLefthook(f, data)
		}
	}
	if data, err := os.ReadFile(filepath.Join(dir, "Makefile")); err == nil {
		if p := fromMakefile(data); len(p.Checks) > 0 {
			return p, nil
		}
	}
	pkgs, _ := filepath.Glob(filepath.Join(dir, "*", "package.json"))
	for _, f := range append([]string{filepath.Join(dir, "package.json")}, pkgs...) {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		rel, _ := filepath.Rel(dir, filepath.Dir(f))
		if p := fromPackageJSON(filepath.ToSlash(rel), data); len(p.Checks) > 0 {
			return p, nil
		}
	}
	return Proposal{Checks: map[string]config.Suite{}, Skipped: []Skip{}}, nil
}

var (
	shellLine = regexp.MustCompile("[$|&;<>`()*?\"'\\\\\n]")
	checkName = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

func fromLefthook(file string, data []byte) (Proposal, error) {
	// the top level holds settings (colors: true) beside the hooks
	var top map[string]yaml.Node
	if err := yaml.Unmarshal(data, &top); err != nil {
		return Proposal{}, fmt.Errorf("%s: %w", file, err)
	}
	p := Proposal{Source: file, Checks: map[string]config.Suite{}, Skipped: []Skip{}}
	for _, hook := range slices.Sorted(maps.Keys(top)) {
		var h struct {
			Commands map[string]struct {
				Run  string `yaml:"run"`
				Root string `yaml:"root"`
			} `yaml:"commands"`
		}
		node := top[hook]
		if node.Kind != yaml.MappingNode || node.Decode(&h) != nil {
			continue
		}
		cmds := h.Commands
		for _, name := range slices.Sorted(maps.Keys(cmds)) {
			c := cmds[name]
			run := strings.TrimSpace(c.Run)
			argv := strings.Fields(run)
			skip := func(why string) { p.Skipped = append(p.Skipped, Skip{Name: name, Run: run, Why: why}) }
			switch {
			case hook != "pre-commit":
				skip("a " + hook + " hook: the gate checks what pre-commit does")
			case strings.Contains(run, "{"):
				skip("runs on lefthook's file list ({staged_files} and the like), not the whole tree")
			case shellLine.MatchString(run):
				skip("a shell line: write it as a script, then add [cycle.checks." + name + "] by hand")
			case len(argv) == 0:
				skip("no run")
			case slices.Contains([]string{"sh", "bash", "zsh"}, argv[0]):
				skip("a script, which may read only the staged files: add it by hand if it checks the whole tree")
			case !checkName.MatchString(name):
				skip("a name of letters, digits, '.', '_' and '-' only")
			default:
				s := config.Suite{Run: argv}
				if root := strings.Trim(c.Root, "/"); root != "" {
					s.Dir, s.Files = root, []string{root + "/**"}
				}
				p.Checks[name] = s
			}
		}
	}
	return p, nil
}

// makeTargets are the Makefile targets and package.json scripts proposed:
// the ones that check without rewriting files.
var makeTargets = []string{"lint", "vet", "typecheck", "type-check"}

func fromMakefile(data []byte) Proposal {
	p := Proposal{Source: "Makefile", Checks: map[string]config.Suite{}, Skipped: []Skip{}}
	for _, l := range strings.Split(string(data), "\n") {
		target, _, ok := strings.Cut(l, ":")
		if ok && slices.Contains(makeTargets, target) && !strings.HasPrefix(l[len(target)+1:], "=") {
			p.Checks[target] = config.Suite{Run: []string{"make", target}}
		}
	}
	return p
}

func fromPackageJSON(dir string, data []byte) Proposal {
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	file := filepath.ToSlash(filepath.Join(dir, "package.json"))
	p := Proposal{Source: file, Checks: map[string]config.Suite{}, Skipped: []Skip{}}
	if json.Unmarshal(data, &pkg) != nil {
		return p
	}
	for _, script := range makeTargets {
		if _, ok := pkg.Scripts[script]; !ok {
			continue
		}
		s := config.Suite{Run: []string{"npm", "run", script}}
		if dir != "." {
			s.Dir, s.Files = dir, []string{dir + "/**"}
		}
		p.Checks[script] = s
	}
	return p
}
