package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeTemplates gives the test repository a GitHub template, a GitHub
// issue form, its chooser's config.yml (not a template) and a GitLab one.
func writeTemplates(t *testing.T) {
	t.Helper()
	writeFile(t, ".github/ISSUE_TEMPLATE/bug_report.md", `---
name: Bug report
about: Something is broken
title: "[BUG] "
labels: regression, prio:p1
assignees:
  - guilbep
---

## Steps

1.
`)
	writeFile(t, ".github/ISSUE_TEMPLATE/config.yml", "blank_issues_enabled: false\n")
	writeFile(t, ".github/ISSUE_TEMPLATE/question.yml", `name: Question
title: "Q: "
labels: [question]
body:
  - type: markdown
    attributes:
      value: Thanks for asking.
  - type: textarea
    attributes:
      label: What do you want to know?
  - type: checkboxes
    attributes:
      label: Checked
      options:
        - label: I searched the issues
`)
	writeFile(t, ".gitlab/issue_templates/Feature.md", "/label ~\"needs triage\" ~feature\n\nWhat it should do.\n")
}

func TestNewFromATemplate(t *testing.T) {
	newTestRepo(t)
	writeConfig(t, fieldsConfig)
	writeTemplates(t)

	out, err := execute(t, []string{"new", "Login fails", "--template", "bug report", "prio=p0"})
	if err != nil {
		t.Fatalf("new --template: %v\n%s", err, out)
	}
	th := threadOf(t, strings.TrimSpace(out))
	slices.Sort(th.Labels)
	if th.Title != "Login fails" || th.Events[0].Body != "## Steps\n\n1." || !slices.Equal(th.Labels, []string{"assignee:guilbep", "prio:p0", "regression"}) {
		t.Errorf("from bug_report.md: %q %q %v", th.Title, th.Events[0].Body, th.Labels)
	}

	// GitLab's, by its file's name: the quick actions are labels
	out, err = execute(t, []string{"new", "Export", "-T", "feature", "-b", "Mine."})
	if err != nil {
		t.Fatalf("new -T feature: %v\n%s", err, out)
	}
	th = threadOf(t, strings.TrimSpace(out))
	slices.Sort(th.Labels)
	if th.Events[0].Body != "Mine." || !slices.Equal(th.Labels, []string{"feature", "needs triage"}) {
		t.Errorf("from Feature.md: %q %v", th.Events[0].Body, th.Labels)
	}

	// an issue form: the text GitHub makes of it
	out, err = execute(t, []string{"new", "How?", "-T", "question", "--force"})
	if err != nil {
		t.Fatalf("new -T question: %v\n%s", err, out)
	}
	if th = threadOf(t, strings.TrimSpace(out)); th.Events[0].Body != "### What do you want to know?\n\n### Checked\n\n- [ ] I searched the issues" {
		t.Errorf("from question.yml: %q", th.Events[0].Body)
	}

	_, err = execute(t, []string{"new", "X", "--template", "config"})
	if exitCode(err) != exitUsage || !strings.Contains(err.Error(), `"Bug report", "Question", "Feature"`) {
		t.Errorf("--template config = %v, want exit %d naming the templates", err, exitUsage)
	}
}

// promptAnswers makes stdin a terminal answering lines, until the test
// ends.
func promptAnswers(t *testing.T) {
	t.Helper()
	was := isTerminal
	isTerminal = func(any) bool { return true }
	t.Cleanup(func() { isTerminal = was })
}

func executeIn(t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	cmd := newRootCmd()
	var out, errOut strings.Builder
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestNewAsksOnATerminal(t *testing.T) {
	newTestRepo(t)
	writeConfig(t, fieldsConfig)
	writeTemplates(t)
	promptAnswers(t)

	out, asked, err := executeIn(t, "1\nLogin fails\n\n", "new")
	if err != nil {
		t.Fatalf("new, asked: %v\n%s", err, asked)
	}
	for _, want := range []string{"1  Bug report", "Template [0]: ", "Title: [BUG] ", "Description: e to write it in your editor, enter to keep the template's: "} {
		if !strings.Contains(asked, want) {
			t.Errorf("asked:\n%s\nwant %q", asked, want)
		}
	}
	th := threadOf(t, strings.TrimSpace(out))
	if th.Title != "[BUG] Login fails" || th.Events[0].Body != "## Steps\n\n1." || !slices.Contains(th.Labels, "regression") {
		t.Errorf("new, asked = %q %q %v", th.Title, th.Events[0].Body, th.Labels)
	}

	// no template, the description in the editor
	opened := editorWrites(t, "Typed in the editor.\n")
	out, asked, err = executeIn(t, "0\nPlain\ne\n", "new", "prio=p2")
	if err != nil {
		t.Fatalf("new, asked: %v\n%s", err, asked)
	}
	th = threadOf(t, strings.TrimSpace(out))
	if th.Title != "Plain" || th.Events[0].Body != "Typed in the editor." || *opened != "\n" || !slices.Equal(th.Labels, []string{"prio:p2"}) {
		t.Errorf("new, editor = %q %q %v (opened on %q)", th.Title, th.Events[0].Body, th.Labels, *opened)
	}

	// an empty title writes nothing
	before := len(listIDs(t))
	if _, _, err := executeIn(t, "0\n\n", "new"); exitCode(err) != exitUsage {
		t.Errorf("new, no title typed = %v, want exit %d", err, exitUsage)
	}
	// never asks with --no-prompt, nor GIT_BOARD_NO_PROMPT
	if _, asked, err := executeIn(t, "0\nNope\n\n", "new", "--no-prompt"); exitCode(err) != exitUsage || asked != "" {
		t.Errorf("new --no-prompt = %v, asked %q", err, asked)
	}
	t.Setenv("GIT_BOARD_NO_PROMPT", "1")
	if _, asked, err := executeIn(t, "0\nNope\n\n", "new"); exitCode(err) != exitUsage || asked != "" {
		t.Errorf("new, GIT_BOARD_NO_PROMPT = %v, asked %q", err, asked)
	}
	if after := len(listIDs(t)); after != before {
		t.Errorf("%d issues, want %d", after, before)
	}
}
