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
	"path/filepath"
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
//
// Every part comes from board data, which anyone who can push the
// board's refs writes, and the name is joined to a folder: a number
// that isn't digits is passed over, and a name that still isn't one
// clean file name (an id that isn't hex) is refused.
func Name(is issues.Issue) (string, error) {
	n := is.BareNumber()
	if strings.Trim(n, "0123456789") != "" {
		n = ""
	}
	if n == "" {
		if !isID(is.ID) {
			return "", fmt.Errorf("issue %q: an id that isn't one names no file", is.ID)
		}
		n = is.ID[:7]
	}
	slug := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(is.Title), "-"), "-")
	if len(slug) > 50 {
		slug = strings.TrimRight(slug[:50], "-")
	}
	name := n + ".md"
	if slug != "" {
		name = n + "-" + slug + ".md"
	}
	if !oneName(name) {
		return "", fmt.Errorf("issue %s: %q isn't a file name", is.ID, name)
	}
	return name, nil
}

// oneName says name is one clean path element: no separator, not . or
// .., nothing filepath.Join would move out of its folder.
func oneName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`+"\x00") &&
		filepath.IsLocal(name) && filepath.Base(name) == name
}

// names are the files' names of issues sorted by id: a clash suffixes
// the later id.
func names(heads []issues.Head) ([]string, error) {
	out := make([]string, len(heads))
	taken := make(map[string]bool, len(heads))
	for i, h := range heads {
		name, err := Name(h.Issue)
		if err != nil {
			return nil, err
		}
		if taken[name] {
			name = strings.TrimSuffix(name, ".md") + "-" + h.ID[:7] + ".md"
		}
		taken[name], out[i] = true, name
	}
	return out, nil
}

// commentsMark is the line between the description and the comments.
const commentsMark = "<!-- git-board comments -->"

// Render is an issue's file.
func Render(th issues.Thread) ([]byte, error) { return RenderFlat(th, nil) }

// RenderFlat is an issue's file, flat telling a label is a flat kind (bug,
// not kind:bug), written as the kind field; nil for none (git board
// export).
func RenderFlat(th issues.Thread, flat func(label string) bool) ([]byte, error) {
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
		case flat != nil && flat(l):
			if f.Fields == nil {
				f.Fields = map[string][]string{}
			}
			f.Fields["kind"] = append(f.Fields["kind"], l)
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
