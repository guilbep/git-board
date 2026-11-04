package store

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/guilbep/git-board/core/oplog"
)

// Front is a Markdown export's front matter (ADR 0002, section 5).
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

// ExportMarkdown writes each issue as dir/NNN-slug.md and gives the files
// it wrote: none when nothing changed. flat tells a label is a flat kind
// (bug, not kind:bug); nil for none. A file of an exported issue under
// another name, after a title change, is removed; any other file is left.
func (b *Backend) ExportMarkdown(dir string, flat func(label string) bool) ([]string, error) {
	issues, err := b.ListIssues()
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(issues))
	for i, is := range issues {
		ids[i] = is.ID
	}
	slices.Sort(ids)
	threads, err := b.Threads(ids)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	files := map[string][]byte{}  // name -> bytes
	nameOf := map[string]string{} // id -> name
	for _, th := range threads {  // by id, so a clash suffixes the later id
		name := mdName(th.Issue)
		if _, taken := files[name]; taken {
			name = strings.TrimSuffix(name, ".md") + "-" + th.ID[:7] + ".md"
		}
		data, err := renderMarkdown(th, flat)
		if err != nil {
			return nil, err
		}
		files[name], nameOf[th.ID] = data, name
	}

	old, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, err
	}
	for _, p := range old {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		f, _, err := ParseMarkdown(data)
		if err != nil {
			continue // not ours
		}
		if name, ok := nameOf[f.ID]; ok && name != filepath.Base(p) {
			if err := os.Remove(p); err != nil {
				return nil, err
			}
		}
	}

	var written []string
	for _, id := range ids {
		name := nameOf[id]
		p := filepath.Join(dir, name)
		if cur, err := os.ReadFile(p); err == nil && bytes.Equal(cur, files[name]) {
			continue
		}
		if err := os.WriteFile(p, files[name], 0o644); err != nil {
			return nil, err
		}
		written = append(written, name)
	}
	return written, nil
}

// mdName is NNN-slug.md: the bare number, else the id's first 7 digits;
// the title's slug, 50 characters at most.
func mdName(is Issue) string {
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

func renderMarkdown(th Thread, flat func(string) bool) ([]byte, error) {
	f := Front{ID: th.ID, Number: th.Number, Title: th.Title, State: th.Status,
		Created: th.CreatedAt.UTC().Truncate(time.Second), Updated: th.UpdatedAt.UTC().Truncate(time.Second)}
	if f.Number == "" {
		f.Number = th.BareNumber()
	}
	f.People.Author = nameOfPerson(th.Author)
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
		case "comment":
			fmt.Fprintf(&buf, "\n### %s · %s\n\n", nameOfPerson(e.Author), e.At.UTC().Format(time.RFC3339))
			if body := strings.TrimSpace(e.Body); body != "" {
				fmt.Fprintf(&buf, "%s\n\n", body)
			}
			fmt.Fprintf(&buf, "<!-- git-board comment %s -->\n", e.ID)
		}
	}
	return buf.Bytes(), nil
}

func nameOfPerson(p Person) string {
	switch {
	case p.Name != "":
		return p.Name
	case p.Login != "":
		return p.Login
	}
	return p.ID
}

// ParseMarkdown splits an exported file into its front matter and the
// rest. The import reads the rest too (parseMD, mdimport.go).
func ParseMarkdown(data []byte) (Front, string, error) {
	var f Front
	rest, ok := bytes.CutPrefix(data, []byte("---\n"))
	if !ok {
		return f, "", fmt.Errorf("no front matter")
	}
	front, body, ok := bytes.Cut(rest, []byte("\n---\n"))
	if !ok {
		return f, "", fmt.Errorf("front matter not closed")
	}
	if err := yaml.Unmarshal(front, &f); err != nil {
		return f, "", err
	}
	if !oplog.IsID(f.ID) {
		return f, "", fmt.Errorf("no issue id")
	}
	return f, string(body), nil
}
