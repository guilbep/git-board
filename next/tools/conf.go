// Package tools holds the tables of .git-board.toml the tools read: the
// GitHub Project bridge's [github-project], the uploads' [uploads], lint's
// [template]. Each moves to its tool when the tool lands (M6); until
// then they are declared here, so a file v1 reads, v2 reads.
package tools

import "github.com/guilbep/git-board/next/conf"

// GithubProject is [github-project]: the GitHub Project the bridge reads.
type GithubProject struct {
	Owner  string `toml:"owner"`
	Number int    `toml:"number"`
	Write  bool   `toml:"write"` // bridge sync writes to the Project
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
