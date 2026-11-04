package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// keysOf are the keys of each object in a JSON array, sorted.
func keysOf(t *testing.T, out string) [][]string {
	t.Helper()
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("not a JSON array: %v\n%s", err, out)
	}
	var res [][]string
	for _, r := range rows {
		var keys []string
		for k := range r {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		res = append(res, keys)
	}
	return res
}

func TestJSONFields(t *testing.T) {
	newTestRepo(t)
	id := openIssueID(t)

	full, err := execute(t, []string{"ls", "--json"})
	if err != nil {
		t.Fatalf("ls --json: %v\n%s", err, full)
	}
	if keys := keysOf(t, full); len(keys) != 1 || !slices.Contains(keys[0], "shown_title") || !slices.Contains(keys[0], "labels") {
		t.Errorf("ls --json keeps every field: %v", keys)
	}

	for _, args := range [][]string{
		{"ls", "--json", "title,id"},
		{"ls", "--json=title,id"},
		{"ls", "is:open", "--json", "id,title"},
		{"ls", "--json", "id,title", "is:open"},
	} {
		out, err := execute(t, args)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		if keys := keysOf(t, out); len(keys) != 1 || !slices.Equal(keys[0], []string{"id", "title"}) {
			t.Errorf("%v: keys %v\n%s", args, keys, out)
		}
	}
	// one known field after --json is the fields, not the filter
	out, err := execute(t, []string{"ls", "--json", "title"})
	if err != nil || !strings.Contains(out, `"title": "Fix invoice bug"`) || strings.Contains(out, `"id"`) {
		t.Errorf("ls --json title = %v\n%s", err, out)
	}
	// a field an issue lacks is null
	if out, _ := execute(t, []string{"ls", "--json", "number,id"}); !strings.Contains(out, `"number": null`) {
		t.Errorf("ls --json number,id:\n%s", out)
	}

	out, err = execute(t, []string{"ls", "--json", "id,nope"})
	if exitCode(err) != exitUsage || err == nil || !strings.Contains(err.Error(), `"nope"`) || !strings.Contains(err.Error(), "shown_title") {
		t.Errorf("ls --json id,nope = %v, want exit %d naming the fields", err, exitUsage)
	}

	out, err = execute(t, []string{"show", id, "--json", "title,events"})
	if err != nil {
		t.Fatalf("show --json title,events: %v\n%s", err, out)
	}
	var th map[string]any
	if err := json.Unmarshal([]byte(out), &th); err != nil || len(th) != 2 || th["title"] != "Fix invoice bug" || th["events"] == nil {
		t.Errorf("show --json title,events = %v\n%s", err, out)
	}
	if out, err := execute(t, []string{"log", id, "--json", "kind"}); err != nil || !strings.Contains(out, `"kind": "created"`) || strings.Contains(out, `"at"`) {
		t.Errorf("log --json kind = %v\n%s", err, out)
	}
	if out, err := execute(t, []string{"me", "--json", "name"}); err != nil || strings.TrimSpace(out) != "{\n  \"name\": \"guilbep\"\n}" {
		t.Errorf("me --json name = %v\n%s", err, out)
	}
	if out, err := execute(t, []string{"me", "--ls", "--json", "name,me"}); err != nil || !strings.Contains(out, `"me": true`) {
		t.Errorf("me --ls --json name,me = %v\n%s", err, out)
	}
	if out, err := execute(t, []string{"comment", id, "-b", "Seen", "--json", "ref"}); err != nil || !strings.Contains(out, `"ref": "`) || strings.Contains(out, `"issue"`) {
		t.Errorf("comment --json ref = %v\n%s", err, out)
	}
}

// A wrong field is refused before anything is written.
func TestJSONFieldsCheckedFirst(t *testing.T) {
	newTestRepo(t)
	before := len(listIDs(t))
	if _, err := execute(t, []string{"new", "Never written", "--json", "id,nope"}); exitCode(err) != exitUsage {
		t.Errorf("new --json id,nope = %v, want exit %d", err, exitUsage)
	}
	if after := len(listIDs(t)); after != before {
		t.Errorf("new --json id,nope wrote an issue: %d issues, was %d", after, before)
	}
	out, err := execute(t, []string{"new", "Written", "--json", "title,status"})
	if err != nil || strings.TrimSpace(out) != "{\n  \"status\": \"open\",\n  \"title\": \"Written\"\n}" {
		t.Errorf("new --json title,status = %v\n%s", err, out)
	}
}

func TestJQAndTemplate(t *testing.T) {
	newTestRepo(t)

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"ls", "--jq", ".[].title"}, "Fix invoice bug\n"},
		{[]string{"ls", "is:closed", "--jq", ".[] | {title, status}"}, `{"status":"closed","title":"Write specs"}` + "\n"},
		{[]string{"ls", "--json", "title", "--jq", ".[0]"}, `{"title":"Fix invoice bug"}` + "\n"},
		{[]string{"ls", "--jq", "length"}, "1\n"},
		{[]string{"ls", "--template", `{{range .}}{{.title}}: {{join ", " .labels}}{{"\n"}}{{end}}`}, "Fix invoice bug: status:ready\n"},
		{[]string{"ls", "--template", `{{range .}}{{truncate 6 .title}}{{end}}`}, "Fix..."},
		{[]string{"ls", "is:closed", "--template", `{{join "," (pluck "title" .)}}`}, "Write specs"},
	} {
		out, err := execute(t, c.args)
		if err != nil || out != c.want {
			t.Errorf("%q = %v\n%q, want %q", c.args, err, out, c.want)
		}
	}

	for _, args := range [][]string{
		{"ls", "--jq", ".[] |"},
		{"ls", "--template", "{{.title"},
		{"ls", "--jq", ".", "--template", "x"},
		{"new", "No template", "--template", "x"}, // new --template is for an issue template
	} {
		if _, err := execute(t, args); exitCode(err) != exitUsage {
			t.Errorf("%q = %v, want exit %d", args, err, exitUsage)
		}
	}
	if _, err := execute(t, []string{"show", openIssueID(t), "--md", "--jq", "."}); exitCode(err) != exitUsage {
		t.Errorf("show --md --jq = %v, want exit %d", err, exitUsage)
	}
}

func TestJSONFieldArgs(t *testing.T) {
	t.Parallel()
	root := &cobra.Command{Use: "git-board"}
	ls := &cobra.Command{Use: "ls", Run: func(*cobra.Command, []string) {}}
	addJSON(ls, "render as JSON", []string{"id", "title"}, true)
	root.AddCommand(ls, &cobra.Command{Use: "init", Run: func(*cobra.Command, []string) {}})
	for _, c := range []struct{ in, want []string }{
		{[]string{"ls", "--json", "id,title"}, []string{"ls", "--json=id,title"}},
		{[]string{"ls", "--json", "id,nope"}, []string{"ls", "--json=id,nope"}}, // refused later, by name
		{[]string{"ls", "--json", "title"}, []string{"ls", "--json=title"}},
		{[]string{"ls", "--json", "login"}, []string{"ls", "--json", "login"}}, // not a field: the filter
		{[]string{"ls", "--json"}, []string{"ls", "--json"}},
		{[]string{"ls", "--", "--json", "id,title"}, []string{"ls", "--", "--json", "id,title"}},
		{[]string{"init", "--json", "id,title"}, []string{"init", "--json", "id,title"}}, // no --json
	} {
		if got := jsonFieldArgs(root, c.in); !slices.Equal(got, c.want) {
			t.Errorf("jsonFieldArgs(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTimeAgo(t *testing.T) {
	t.Parallel()
	for d, want := range map[time.Duration]string{
		10 * time.Second:     "less than a minute ago",
		time.Minute:          "1 minute ago",
		5 * time.Hour:        "5 hours ago",
		3 * 24 * time.Hour:   "3 days ago",
		70 * 24 * time.Hour:  "2 months ago",
		800 * 24 * time.Hour: "2 years ago",
	} {
		if got := timeAgo(d); got != want {
			t.Errorf("timeAgo(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestJSONFieldsOf(t *testing.T) {
	t.Parallel()
	got := jsonFields(meRow{})
	if want := []string{"id", "name", "login", "avatar_url", "me"}; !slices.Equal(got, want) {
		t.Errorf("jsonFields(meRow) = %q, want %q", got, want)
	}
}
