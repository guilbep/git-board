// Package mdfiles is a Markdown folder's rendered files
// (docs/spec/datastores.md, The rendered files): each issue as a readable
// NNN-slug.md beside the folder's .board, rendered after each write, and
// read back in before the next one, a hand's edits becoming the writer's
// operations (Importing Markdown).
//
// The render is an observer of the kernel's writes, and follows the
// board: an issue is rendered again when its version, its name or what
// else its file shows (people, the counter) moved since its file was
// written. The import reads only the files whose stat moved since they
// were last written or read. Both keep what they know in this machine's
// folder (files.go), never the checkout's.
//
// Files, names and what is imported are v1's (internal/store/markdown.go
// and mdimport.go), byte for byte.
package mdfiles

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/domain/people"
)

// Front is a file's front matter (ADR 0002, section 5).
type Front struct {
	ID     string              `yaml:"id"`
	Number string              `yaml:"number,omitempty"`
	Title  string              `yaml:"title"`
	State  string              `yaml:"state"`
	Fields map[string][]string `yaml:"fields,omitempty"`
	Labels []string            `yaml:"labels,omitempty"`
	People struct {
		Author    string   `yaml:"author"`
		Assignees []string `yaml:"assignees,omitempty"`
	} `yaml:"people"`
	Created time.Time `yaml:"created"`
	Updated time.Time `yaml:"updated"`
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Name is an issue's file name, NNN-slug.md: the bare number, else the
// id's first 7 digits; the title's slug, 50 bytes at most. Two of one
// name: the later id's takes -<its first 7 digits> (names).
func Name(is issues.Issue) string {
	n := is.BareNumber()
	if n == "" {
		n = is.ID[:7]
	}
	slug := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(is.Title), "-"), "-")
	if len(slug) > 50 {
		slug = strings.TrimRight(slug[:50], "-")
	}
	if slug == "" {
		return n + ".md"
	}
	return n + "-" + slug + ".md"
}

// names are the files' names of issues sorted by id: a clash suffixes
// the later id.
func names(heads []issues.Head) []string {
	out := make([]string, len(heads))
	taken := make(map[string]bool, len(heads))
	for i, h := range heads {
		name := Name(h.Issue)
		if taken[name] {
			name = strings.TrimSuffix(name, ".md") + "-" + h.ID[:7] + ".md"
		}
		taken[name], out[i] = true, name
	}
	return out
}

// commentsMark is the line between the description and the comments.
const commentsMark = "<!-- git-board comments -->"

// Render is an issue's file.
func Render(th issues.Thread) ([]byte, error) {
	f := Front{ID: th.ID, Number: th.Number, Title: th.Title, State: th.Status,
		Created: th.CreatedAt.UTC().Truncate(time.Second), Updated: th.UpdatedAt.UTC().Truncate(time.Second)}
	if f.Number == "" {
		f.Number = th.BareNumber()
	}
	f.People.Author = nameOf(th.Author)
	for _, l := range th.Labels {
		ns, v, ok := strings.Cut(l, ":")
		switch {
		case ok && ns == "assignee":
			f.People.Assignees = append(f.People.Assignees, v)
		case ok:
			if f.Fields == nil {
				f.Fields = map[string][]string{}
			}
			f.Fields[ns] = append(f.Fields[ns], v)
		default:
			f.Labels = append(f.Labels, l)
		}
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(f); err != nil {
		return nil, err
	}
	buf.WriteString("---\n")
	for _, e := range th.Events {
		switch e.Kind {
		case "created":
			if body := strings.TrimSpace(e.Body); body != "" {
				fmt.Fprintf(&buf, "\n%s\n", body)
			}
			// where the comments start: a "### " line above it is the
			// description's own
			buf.WriteString("\n" + commentsMark + "\n")
		case "comment":
			fmt.Fprintf(&buf, "\n### %s · %s\n\n", nameOf(e.Author), e.At.UTC().Format(time.RFC3339))
			if body := strings.TrimSpace(e.Body); body != "" {
				fmt.Fprintf(&buf, "%s\n\n", body)
			}
			fmt.Fprintf(&buf, "<!-- git-board comment %s -->\n", e.ID)
		}
	}
	return buf.Bytes(), nil
}

// nameOf is a person as a file names them: their name, else their
// login, else their id.
func nameOf(p people.Who) string {
	switch {
	case p.Name != "":
		return p.Name
	case p.Login != "":
		return p.Login
	}
	return string(p.ID)
}
