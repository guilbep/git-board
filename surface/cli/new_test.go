package cli

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writeTemplates writes a repository's issue templates: GitHub's (a
// Markdown one, an issue form, its config) and GitLab's.
func writeTemplates(t *testing.T, dir string) {
	t.Helper()
	for name, text := range map[string]string{
		".github/ISSUE_TEMPLATE/bug.md":         "---\nname: Bug report\nabout: Something broke\ntitle: \"[BUG] \"\nlabels: regression, prio:p1\nassignees: [ada]\n---\n\n## Steps\n\n1.\n",
		".github/ISSUE_TEMPLATE/question.yml":   "name: Question\nlabels: [\"question\"]\nbody:\n  - type: markdown\n    attributes:\n      value: Thanks!\n  - type: textarea\n    attributes:\n      label: What do you want to know?\n  - type: checkboxes\n    attributes:\n      label: Checked\n      options:\n        - label: I searched the issues\n",
		".github/ISSUE_TEMPLATE/config.yml":     "blank_issues_enabled: false\n",
		".gitlab/issue_templates/Feature.md":    "Why:\n\n/label ~\"needs triage\" ~prio:p2\n/assign @ada\n",
		".gitlab/issue_templates/notes.txt":     "not a template\n",
		".github/ISSUE_TEMPLATE/broken.yml":     "name: [\n",
		".github/ISSUE_TEMPLATE/empty-form.yml": "name: Empty\n",
	} {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// prompted runs a command line with stdin a terminal answering stdin.
func prompted(t *testing.T, stdin string, args ...string) (out, asked string, code int) {
	t.Helper()
	was := isTerminal
	isTerminal = func(io.Reader) bool { return true }
	defer func() { isTerminal = was }()
	root := newRoot()
	var o, e strings.Builder
	root.SetIn(strings.NewReader(stdin))
	root.SetOut(&o)
	root.SetErr(&e)
	root.SetContext(ctx)
	err := runRoot(root, args)
	code = 0
	if err != nil {
		code = exitCode(err)
	}
	return o.String(), e.String(), code
}

func TestNewTemplates(t *testing.T) {
	dir, _ := newBoard(t)
	t.Chdir(dir)
	writeTemplates(t, dir)
	t.Setenv("GIT_BOARD_NO_PROMPT", "1")
	show := func(id string) (title, body string, labels []string) {
		t.Helper()
		out, errs, code := run(t, "show", strings.TrimSpace(id), "--json", "title,events,labels", "--jq", `.title, .events[0].body, (.labels | join(","))`)
		if code != 0 {
			t.Fatalf("show: %s", errs)
		}
		lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
		return lines[0], strings.Join(lines[1:len(lines)-1], "\n"), strings.Split(lines[len(lines)-1], ",")
	}
	out, errs, code := run(t, "new", "Crash", "--template", "bug report", "prio=p0")
	if code != 0 {
		t.Fatalf("new --template: %d %s", code, errs)
	}
	if title, body, labels := show(out); title != "Crash" || body != "## Steps\n\n1." || !slices.Equal(labels, []string{"assignee:ada", "prio:p0", "regression"}) {
		t.Errorf("from bug.md: %q %q %v", title, body, labels)
	}
	out, _, _ = run(t, "new", "Why", "-T", "feature", "-b", "mine")
	if _, body, labels := show(out); body != "mine" || !slices.Equal(labels, []string{"assignee:ada", "needs triage", "prio:p2"}) {
		t.Errorf("from Feature.md: %q %v", body, labels)
	}
	out, _, _ = run(t, "new", "Q", "-T", "question")
	if _, body, _ := show(out); body != "### What do you want to know?\n\n### Checked\n\n- [ ] I searched the issues" {
		t.Errorf("from question.yml: %q", body)
	}
	_, errs, code = run(t, "new", "X", "--template", "config")
	if code != exitUsage || !strings.Contains(errs, `there are "Bug report", "Question", "Feature"`) {
		t.Errorf("--template config = %d %s, want exit %d naming the templates", code, errs, exitUsage)
	}
}

func TestNewAsksOnATerminal(t *testing.T) {
	dir, _ := newBoard(t)
	t.Chdir(dir)
	writeTemplates(t, dir)
	out, asked, code := prompted(t, "1\nLogin fails\n\n", "new")
	if code != 0 {
		t.Fatalf("new, asked: %d\n%s", code, asked)
	}
	for _, want := range []string{"Templates:\n  1  Bug report\n  2  Question\n  3  Feature\n  0  none\n", "Template [0]: ", "Title: [BUG] ", "Description: e to write it in your editor, enter to keep the template's: "} {
		if !strings.Contains(asked, want) {
			t.Errorf("asked:\n%s\nwant %q", asked, want)
		}
	}
	got, _, _ := run(t, "show", strings.TrimSpace(out), "--json", "title,events", "--jq", ".title + \"|\" + .events[0].body")
	if got != "[BUG] Login fails|## Steps\n\n1.\n" {
		t.Errorf("new, asked = %q", got)
	}

	// no template, the description in the editor
	was := runEditor
	var opened string
	runEditor = func(_, initial string) (string, error) { opened = initial; return "Typed in the editor.\n", nil }
	defer func() { runEditor = was }()
	out, asked, code = prompted(t, "nope\n0\nPlain\ne\n", "new", "prio=p2")
	if code != 0 || !strings.Contains(asked, `"nope": a number from 0 to 3, or a name`) {
		t.Fatalf("new, asked: %d\n%s", code, asked)
	}
	got, _, _ = run(t, "show", strings.TrimSpace(out), "--json", "title,events,labels", "--jq", `.title + "|" + .events[0].body + "|" + (.labels | join(","))`)
	if got != "Plain|Typed in the editor.|prio:p2\n" || opened != "\n" {
		t.Errorf("new, editor = %q (opened on %q)", got, opened)
	}

	// an empty title writes nothing; --no-prompt and GIT_BOARD_NO_PROMPT never ask
	before, _, _ := run(t, "ls", "", "--json", "id", "--jq", "length")
	if _, _, code := prompted(t, "0\n\n", "new"); code != exitUsage {
		t.Errorf("new, no title typed = %d, want exit %d", code, exitUsage)
	}
	if _, asked, code := prompted(t, "0\nNope\n\n", "new", "--no-prompt"); code != exitUsage || asked != "" {
		t.Errorf("new --no-prompt = %d, asked %q", code, asked)
	}
	t.Setenv("GIT_BOARD_NO_PROMPT", "1")
	if _, asked, code := prompted(t, "0\nNope\n\n", "new"); code != exitUsage || asked != "" {
		t.Errorf("new, GIT_BOARD_NO_PROMPT = %d, asked %q", code, asked)
	}
	if after, _, _ := run(t, "ls", "", "--json", "id", "--jq", "length"); after != before {
		t.Errorf("%s issues, want %s", after, before)
	}
}
