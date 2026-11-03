package lint

import (
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/config"
)

// A file names the template it came from, and its version: lint lists
// what the template has that the file lacks, and a newer template.
func TestTemplateDrift(t *testing.T) {
	t.Parallel()
	enac, err := config.Template("enac")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := config.Check([]byte(enac))
	if err != nil {
		t.Fatal(err)
	}
	if got := checkTemplate(cfg); len(got) != 0 {
		t.Errorf("the template itself drifts: %v", got)
	}

	// an old file: version 0, no Could, no est
	old := strings.Replace(enac, "version = 1", "version = 0", 1)
	old = strings.Replace(old, `"Should", "Could", "Wont"`, `"Should", "Wont"`, 1)
	old = strings.Replace(old, "[fields.est]\nsingle = true\nnumeric = true\nunit = \"h\"\noptions = [\"0.5\", \"1\", \"2\", \"4\", \"8\", \"16\"]\n", "", 1)
	cfg, _, err = config.Check([]byte(old))
	if err != nil {
		t.Fatal(err)
	}
	var msgs []string
	for _, f := range checkTemplate(cfg) {
		if f.Check != CheckTemplate {
			t.Errorf("check %q", f.Check)
		}
		msgs = append(msgs, f.Message)
	}
	want := []string{
		"this file is from template enac version 0; enac is at version 1",
		"moscow: template enac has Could, this file doesn't",
		"est: template enac has this field, this file doesn't",
	}
	if !slices.Equal(msgs, want) {
		t.Errorf("findings\n%q\nwant\n%q", msgs, want)
	}

	// with the Project named, its fields' values are the Project's: a
	// MoSCoW without Could isn't drift
	cfg, _, err = config.Check([]byte(strings.Replace(old, "version = 0", "version = 1", 1) + "\n[github-project]\nowner = \"EPFL-ENAC\"\nnumber = 53\n"))
	if err != nil {
		t.Fatal(err)
	}
	msgs = nil
	for _, f := range checkTemplate(cfg) {
		msgs = append(msgs, f.Message)
	}
	if want := []string{"est: template enac has this field, this file doesn't"}; !slices.Equal(msgs, want) {
		t.Errorf("with the Project: %q, want %q", msgs, want)
	}

	// no [template], or one that isn't built in: nothing to compare to
	for _, src := range []string{"", "[template]\nname = \"ours\"\n"} {
		cfg, _, err := config.Check([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		if got := checkTemplate(cfg); len(got) != 0 {
			t.Errorf("%q: %v", src, got)
		}
	}
}
