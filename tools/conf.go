// Package tools holds the tables of .git-board.toml the tools read: the
// GitHub Project bridge's [github-project], the uploads' [uploads], lint's
// [template]. Each moves to its tool when the tool lands (M6); until
// then they are declared here, so a file v1 reads, v2 reads.
package tools

import (
	"slices"

	"github.com/guilbep/git-board/conf"
)

// GithubProject is [github-project]: the GitHub Project the bridge reads.
type GithubProject struct {
	Owner  string `toml:"owner"`
	Number int    `toml:"number"`
	Write  bool   `toml:"write"` // bridge sync writes to the Project
	// Fields are the board's fields the Project holds as its columns:
	// their home on GitHub is the Project, not a label (ADR 0022). project
	// configure writes them; a table without the key holds
	// DefaultProjectFields, and the Project page says so.
	Fields []string `toml:"fields"`
	// Unmapped are the Project's values the team chose to leave out, by
	// field (category for the Category): a sync skips them, and warns of
	// nothing.
	Unmapped map[string][]string `toml:"unmapped"`
}

// DefaultProjectFields are the columns a Project holds when its table
// names none: the ENAC Project's own.
var DefaultProjectFields = []string{"status", "prio", "mod", "est", "sprint", "moscow", "milestone"}

// Columns are the fields the Project holds: its table's, else the default.
func (g GithubProject) Columns() []string {
	if g.Fields != nil {
		return g.Fields
	}
	return DefaultProjectFields
}

// Holds says field's home on GitHub is the Project, by this table: a
// Project is named, and the field is one of its columns.
func (g GithubProject) Holds(field string) bool {
	return g.Number != 0 && slices.Contains(g.Columns(), field)
}

// Uploads is [uploads]: where files attached to issues go. The keys stay
// in the environment.
type Uploads struct {
	Endpoint  string `toml:"endpoint"`
	PublicURL string `toml:"public_url"`
	Prefix    string `toml:"prefix"`
	Region    string `toml:"region"`
}

// Template is [template]: the built-in template the file came from.
type Template struct {
	Name    string `toml:"name"`
	Version int    `toml:"version"`
}

// Tables are the tools' tables on a file.
type Tables struct {
	GithubProject *conf.Handle[GithubProject]
	Uploads       *conf.Handle[Uploads]
	Template      *conf.Handle[Template]
}

// Declare declares the tools' tables on f.
func Declare(f *conf.File) Tables {
	return Tables{
		GithubProject: conf.Declare(f, conf.Table[GithubProject]{Name: "github-project"}),
		Uploads:       conf.Declare(f, conf.Table[Uploads]{Name: "uploads"}),
		Template:      conf.Declare(f, conf.Table[Template]{Name: "template"}),
	}
}
