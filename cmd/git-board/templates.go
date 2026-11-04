package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/guilbep/git-board/internal/config"
)

// issueTemplate is a forge's issue template, as the repository keeps it:
// GitHub's .github/ISSUE_TEMPLATE/*.md (front matter: name, about, title,
// labels, assignees) and its issue forms (*.yml), GitLab's
// .gitlab/issue_templates/*.md (named by the file, /label and /assign
// quick actions in the text).
type issueTemplate struct {
	Name      string
	Title     string
	Body      string
	Labels    []string
	Assignees []string
	Path      string
}

// templateDirs are where the forges keep issue templates, from the
// repository's root.
var templateDirs = []string{filepath.Join(".github", "ISSUE_TEMPLATE"), filepath.Join(".gitlab", "issue_templates")}

// loadTemplates reads the repository's issue templates, by file name in
// each directory; one it can't read is skipped.
func loadTemplates() []issueTemplate {
	root := filepath.Dir(config.Path())
	var res []issueTemplate
	for _, dir := range templateDirs {
		entries, err := os.ReadDir(filepath.Join(root, dir))
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			ext := strings.ToLower(filepath.Ext(name))
			if e.IsDir() || (ext != ".md" && ext != ".yml" && ext != ".yaml") || strings.EqualFold(strings.TrimSuffix(name, filepath.Ext(name)), "config") {
				continue
			}
			p := filepath.Join(root, dir, name)
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			var t issueTemplate
			var ok bool
			if ext == ".md" {
				t, ok = markdownTemplate(string(data)), true
			} else {
				t, ok = formTemplate(data)
			}
			if !ok {
				continue
			}
			if t.Name == "" {
				t.Name = strings.TrimSuffix(name, filepath.Ext(name))
			}
			t.Path = filepath.Join(dir, name)
			res = append(res, t)
		}
	}
	return res
}

// findTemplate is the template called name (its name or its file's,
// without case), or a usage error listing them.
func findTemplate(all []issueTemplate, name string) (issueTemplate, error) {
	for _, t := range all {
		file := strings.TrimSuffix(filepath.Base(t.Path), filepath.Ext(t.Path))
		if strings.EqualFold(t.Name, name) || strings.EqualFold(file, name) {
			return t, nil
		}
	}
	if len(all) == 0 {
		return issueTemplate{}, usageError{fmt.Errorf("--template %q: no issue templates in %s", name, strings.Join(templateDirs, " or "))}
	}
	names := make([]string, len(all))
	for i, t := range all {
		names[i] = fmt.Sprintf("%q", t.Name)
	}
	return issueTemplate{}, usageError{fmt.Errorf("--template %q: no such template; there are %s", name, strings.Join(names, ", "))}
}

// frontMatter is a GitHub template's head: labels and assignees a list
// or a comma-separated string.
type frontMatter struct {
	Name      string    `yaml:"name"`
	Title     string    `yaml:"title"`
	Labels    stringish `yaml:"labels"`
	Assignees stringish `yaml:"assignees"`
}

// stringish is a YAML list, or a string of comma-separated items.
type stringish []string

func (s *stringish) UnmarshalYAML(n *yaml.Node) error {
	var list []string
	if n.Kind == yaml.SequenceNode {
		if err := n.Decode(&list); err != nil {
			return err
		}
	} else {
		var one string
		if err := n.Decode(&one); err != nil {
			return err
		}
		list = strings.Split(one, ",")
	}
	for _, v := range list {
		if v = strings.TrimSpace(v); v != "" {
			*s = append(*s, v)
		}
	}
	return nil
}

var quickAction = regexp.MustCompile(`^/(label|labels|assign)\s+(.*)$`)
var quickArg = regexp.MustCompile(`[~@]("[^"]*"|\S+)`)

// markdownTemplate reads a .md template: its front matter when it has one
// (GitHub's), its /label and /assign lines (GitLab's), its text.
func markdownTemplate(text string) issueTemplate {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var t issueTemplate
	if rest, ok := strings.CutPrefix(text, "---\n"); ok {
		if head, body, ok := strings.Cut(rest, "\n---"); ok {
			var fm frontMatter
			if yaml.Unmarshal([]byte(head), &fm) == nil {
				t.Name, t.Title, t.Labels, t.Assignees = fm.Name, fm.Title, fm.Labels, fm.Assignees
				_, text, _ = strings.Cut(body, "\n")
			}
		}
	}
	var kept []string
	for _, line := range strings.Split(text, "\n") {
		m := quickAction.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			kept = append(kept, line)
			continue
		}
		for _, a := range quickArg.FindAllStringSubmatch(m[2], -1) {
			v := strings.Trim(a[1], `"`)
			if m[1] == "assign" {
				t.Assignees = append(t.Assignees, v)
			} else {
				t.Labels = append(t.Labels, v)
			}
		}
	}
	t.Body = trimText(strings.Join(kept, "\n"))
	return t
}

// form is a GitHub issue form, read as far as a text needs it.
type form struct {
	Name      string    `yaml:"name"`
	Title     string    `yaml:"title"`
	Labels    stringish `yaml:"labels"`
	Assignees stringish `yaml:"assignees"`
	Body      []struct {
		Type       string `yaml:"type"`
		Attributes struct {
			Label   string    `yaml:"label"`
			Value   string    `yaml:"value"`
			Options yaml.Node `yaml:"options"`
		} `yaml:"attributes"`
	} `yaml:"body"`
}

// formTemplate makes an issue form a text as GitHub writes the issue it
// files: a "### label" heading per field, markdown elements as they are.
// Its validations and its dropdown's choices aren't kept: a checkbox
// becomes "- [ ] its label".
func formTemplate(data []byte) (issueTemplate, bool) {
	var f form
	if err := yaml.Unmarshal(data, &f); err != nil || len(f.Body) == 0 {
		return issueTemplate{}, false
	}
	var parts []string
	for _, el := range f.Body {
		a := el.Attributes
		switch el.Type {
		case "markdown":
			continue // instructions to the one filing: GitHub doesn't keep them
		case "checkboxes":
			var opts []struct {
				Label string `yaml:"label"`
			}
			_ = a.Options.Decode(&opts)
			var lines []string
			for _, o := range opts {
				lines = append(lines, "- [ ] "+o.Label)
			}
			parts = append(parts, "### "+a.Label+"\n\n"+strings.Join(lines, "\n"))
		default:
			parts = append(parts, strings.TrimRight("### "+a.Label+"\n\n"+a.Value, "\n "))
		}
	}
	return issueTemplate{Name: f.Name, Title: f.Title, Labels: f.Labels, Assignees: f.Assignees, Body: strings.Join(parts, "\n\n")}, true
}

// labels are the labels a template gives a new issue: its labels,
// and an assignee:<login> each.
func (t issueTemplate) labels() []string {
	res := slices.Clone(t.Labels)
	for _, a := range t.Assignees {
		res = append(res, "assignee:"+strings.TrimPrefix(a, "@"))
	}
	return res
}

// prompter asks on a terminal: questions to w, answers line by line
// from r.
type prompter struct {
	r *bufio.Reader
	w io.Writer
}

// ask prints question and reads the answer, trimmed; io.EOF is an empty
// answer.
func (p prompter) ask(question string) (string, error) {
	fmt.Fprint(p.w, question)
	line, err := p.r.ReadString('\n')
	if err == io.EOF {
		err = nil
		fmt.Fprintln(p.w)
	}
	return strings.TrimSpace(line), err
}

