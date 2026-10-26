package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	licenceFile = regexp.MustCompile(`(?i)^(licen[cs]e|copying)([._-].*)?$`)
	spdx        = regexp.MustCompile(`SPDX-License-Identifier:\s*([^\n` + "`" + `]+)`)
)

// licence names a licence text by its SPDX identifier, else by its title
// or its grant, at its start: a licence text quotes others further down,
// as MPL-2.0 names the GPL as a secondary licence.
func licence(text string) string {
	if m := spdx.FindStringSubmatch(text); m != nil {
		return strings.TrimSpace(m[1])
	}
	upper := strings.ToUpper(text[:min(len(text), 1000)])
	for _, l := range []struct{ name, mark string }{
		{"AGPL", "GNU AFFERO GENERAL PUBLIC LICENSE"},
		{"LGPL", "GNU LESSER GENERAL PUBLIC LICENSE"},
		{"LGPL", "GNU LIBRARY GENERAL PUBLIC LICENSE"},
		{"GPL", "GNU GENERAL PUBLIC LICENSE"},
		{"MPL-2.0", "MOZILLA PUBLIC LICENSE"},
		{"Apache-2.0", "APACHE LICENSE"},
		{"MIT", "PERMISSION IS HEREBY GRANTED, FREE OF CHARGE"},
		{"ISC", "PERMISSION TO USE, COPY, MODIFY, AND/OR DISTRIBUTE"},
		{"BSD", "REDISTRIBUTION AND USE IN SOURCE AND BINARY FORMS"},
		{"Unlicense", "THIS IS FREE AND UNENCUMBERED SOFTWARE"},
		{"public domain", "PUBLIC DOMAIN"},
	} {
		if strings.Contains(upper, l.mark) {
			return l.name
		}
	}
	return "unknown"
}

// TestLicences reads the licence of every module the build and its tests
// use, from the module cache, and fails on any GPL, LGPL or AGPL. -v lists
// them: make licences.
func TestLicences(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("go", "list", "-deps", "-test", "-f",
		`{{with .Module}}{{.Path}}@{{.Version}} {{with .Replace}}{{.Dir}}{{else}}{{.Dir}}{{end}}{{end}}`, "./...")
	cmd.Dir = "../.."
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		mod, dir, _ := strings.Cut(line, " ")
		if mod == "" || seen[mod] {
			continue
		}
		seen[mod] = true
		var found []string
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.IsDir() || !licenceFile.MatchString(e.Name()) {
				continue
			}
			text, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			found = append(found, licence(string(text)))
		}
		if len(found) == 0 {
			found = []string{"no licence file"}
		}
		t.Logf("%-60s %s", mod, strings.Join(found, ", "))
		for _, l := range found {
			if strings.Contains(l, "GPL") {
				t.Errorf("%s is %s", mod, l)
			}
		}
	}
}
