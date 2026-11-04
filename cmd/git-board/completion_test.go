package main

import (
	"slices"
	"strings"
	"testing"
)

const completionConfig = `
[fields.status]
single = true
options = ["todo", "ready", "done"]

[fields.prio]
single = true
options = ["p0", "p1", "p2"]

[fields.sprint]
single = true

[sprints]
length_days = 14
start_weekday = "monday"

[[sprints.list]]
slug = "s1"
title = "Sprint 1"
start = 2020-01-01

[views.backlog]
filter = "is:open"
`

// complete asks the binary for the completions of args, as the shell
// does: the choices, without their descriptions, and the directive.
func complete(t *testing.T, args ...string) ([]string, string) {
	t.Helper()
	out, err := execute(t, append([]string{"__complete"}, args...))
	if err != nil {
		t.Fatalf("__complete %q: %v\n%s", args, err, out)
	}
	var choices []string
	directive := ""
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(line, ":") {
			directive = line
			continue
		}
		if strings.HasPrefix(line, "Completion ended") {
			continue
		}
		choice, _, _ := strings.Cut(line, "\t")
		choices = append(choices, choice)
	}
	return choices, directive
}

func TestCompletion(t *testing.T) {
	newTestRepo(t)
	writeConfig(t, completionConfig)
	id := openIssueID(t)

	out, err := execute(t, []string{"__complete", "show", ""})
	if err != nil || !strings.Contains(out, shortID(id)+"\tFix invoice bug\n") || !strings.Contains(out, "Write specs (closed)") {
		t.Errorf("show <TAB> = %v\n%s", err, out)
	}
	if got, _ := complete(t, "close", id[:3]); !slices.Equal(got, []string{shortID(id)}) {
		t.Errorf("close %s<TAB> = %q", id[:3], got)
	}
	if got, _ := complete(t, "show", id, ""); len(got) != 0 {
		t.Errorf("show <id> <TAB> = %q, want nothing", got)
	}

	for _, c := range []struct {
		args []string
		want []string
	}{
		{[]string{"set", id, "pr"}, []string{"prio="}},
		{[]string{"set", id, "prio=p"}, []string{"prio=p0", "prio=p1", "prio=p2"}},
		{[]string{"set", id, "status=-r"}, []string{"status=-ready"}},
		{[]string{"set", id, "sprint="}, []string{"sprint=@current", "sprint=@next", "sprint=@previous", "sprint=s1"}},
		{[]string{"set", "--filter", "is:open", "prio=p1"}, []string{"prio=p1"}},
		{[]string{"new", "A title", "status="}, []string{"status=todo", "status=ready", "status=done"}},
		{[]string{"new", "A tit"}, nil},
		{[]string{"move", id, "re"}, []string{"ready"}},
		{[]string{"ls", "is:"}, []string{"is:open", "is:closed"}},
		{[]string{"ls", "is:open -status:d"}, []string{"is:open -status:done"}},
		{[]string{"ls", "is:open pri"}, []string{"is:open prio:"}},
		{[]string{"ls", "--sort", "up"}, []string{"updated"}},
		{[]string{"ls", "--json=id,ti"}, []string{"id,title"}},
		{[]string{"view", "b"}, []string{"backlog"}},
		{[]string{"log", id, "--field", "st"}, []string{"status"}},
	} {
		if got, _ := complete(t, c.args...); !slices.Equal(got, c.want) {
			t.Errorf("%q<TAB> = %q, want %q", c.args, got, c.want)
		}
	}
	if got, _ := complete(t, "me", ""); len(got) != 1 {
		t.Errorf("me <TAB> = %q, want the one person", got)
	}

	// outside a repository: nothing, and no error
	t.Chdir(t.TempDir())
	if got, d := complete(t, "show", ""); len(got) != 0 || d != ":4" {
		t.Errorf("show <TAB> outside a repository = %q %s, want nothing", got, d)
	}
	if got, _ := complete(t, "set", "x", "prio="); len(got) != 0 {
		t.Errorf("set x prio=<TAB> outside a repository = %q, want nothing", got)
	}
}

func TestCompletionCommand(t *testing.T) {
	t.Parallel()
	cmd := newRootCmd()
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"completion", "bash"})
	if err := cmd.Execute(); err != nil || !strings.Contains(buf.String(), "__start_git-board") {
		t.Errorf("completion bash = %v\n%.200s", err, buf.String())
	}
}
