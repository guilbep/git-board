package ghlabels

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/guilbep/git-board/internal/config"
)

// ENAC's template wants every EPFL-ENAC label as GitHub has it: nothing
// for Settings › Labels to call "Color differs".
func TestENACTemplateAgreesWithItsLabels(t *testing.T) {
	t.Parallel()
	var existing []Label
	data, err := os.ReadFile("testdata/enac-labels.json")
	if err == nil {
		err = json.Unmarshal(data, &existing)
	}
	if err != nil {
		t.Fatal(err)
	}
	s, err := config.Template("enac")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := config.Check([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	desired := Desired(cfg)
	for _, l := range existing {
		want, ok := lookup(desired, l.Name)
		if !ok {
			t.Errorf("%s: not in the template", l.Name)
		} else if s := State(want, existing); s != "same" {
			t.Errorf("%s: %s (want %s %q)", l.Name, s, want.Color, want.Description)
		}
	}
}
