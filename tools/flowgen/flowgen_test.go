package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const root = "../.."

// The stories match the code, and docs/spec/data-flow.html is what they
// and the code draw today: a renamed function, a moved import or an
// edited story fails here until the page is generated again.
func TestPageIsCurrent(t *testing.T) {
	t.Parallel()
	got, err := Generate(root, "docs/spec/flows.json")
	if err != nil {
		t.Fatal(err)
	}
	cur, err := os.ReadFile(filepath.Join(root, "docs/spec/data-flow.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, cur) {
		t.Fatal("docs/spec/data-flow.html is not up to date: run make flow")
	}
}

// A story that names what the code doesn't have is refused, with the step.
func TestStoriesAreChecked(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join(root, "docs/spec/flows.json"))
	if err != nil {
		t.Fatal(err)
	}
	for name, edit := range map[string][2]string{
		"a function":    {`"kernel.K.through"`, `"kernel.K.thru"`},
		"an import":     {`"hops": "kernel -> kernel/pack"`, `"hops": "kernel -> store/git"`},
		"a route":       {`"surface/http:GET /api/events"`, `"surface/http:GET /api/evts"`},
		"a backend's":   {`"$store.$storeType.Add"`, `"$store.$storeType.Append"`},
		"a when":        {`"when": "moved"`, `"when": "nope"`},
		"a hidden pkg":  {`"hide": ["web"]`, `"hide": ["nope"]`},
		"an option pkg": {`"pkg": "store/md"`, `"pkg": "store/markdown"`},
	} {
		if !bytes.Contains(raw, []byte(edit[0])) {
			t.Fatalf("%s: flows.json has no %s to break", name, edit[0])
		}
		dir := t.TempDir()
		bad := bytes.Replace(raw, []byte(edit[0]), []byte(edit[1]), 1)
		if err := os.MkdirAll(filepath.Join(dir, "docs/spec"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "docs/spec/flows.json"), bad, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Generate(root, filepath.Join(dir, "docs/spec/flows.json")); err == nil {
			t.Errorf("%s: %s accepted", name, edit[1])
		} else {
			t.Logf("%s: %v", name, strings.ReplaceAll(err.Error(), "\n", " / "))
		}
	}
}
