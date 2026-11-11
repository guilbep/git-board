package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/tools/ghproject"
)

// configure writes [github-project]; a run without one is a usage error,
// and nothing reads the network. It chdirs (-C): serial.
func TestProjectConfigure(t *testing.T) {
	dir, _ := newBoard(t)
	t.Chdir(dir) // and back after: -C leaves the process there
	if _, errs, code := run(t, "-C", dir, "project", "pull"); code != 2 || !strings.Contains(errs, "no [github-project]") {
		t.Errorf("pull without a Project: %d %q", code, errs)
	}
	out, errs, code := run(t, "-C", dir, "project", "configure", "--owner", "EPFL-ENAC", "--project", "40")
	if code != 0 || !strings.Contains(out, `owner="EPFL-ENAC" number=40`) {
		t.Fatalf("configure: %d %q %q", code, out, errs)
	}
	text, err := os.ReadFile(filepath.Join(dir, ".git-board.toml"))
	if err != nil || !strings.Contains(string(text), "[github-project]") || !strings.Contains(string(text), "number = 40") {
		t.Errorf("the file: %q, %v", text, err)
	}
	if _, errs, code := run(t, "-C", dir, "project", "configure", "--remove", "--owner", "x"); code != 2 {
		t.Errorf("--remove with --owner: %d %q", code, errs)
	}
	if _, _, code := run(t, "-C", dir, "project", "resolve", "1", "status"); code != 2 {
		t.Errorf("resolve without a side: %d", code)
	}
	if out, _, code := run(t, "-C", dir, "project", "configure", "--remove"); code != 0 || !strings.Contains(out, "removed") {
		t.Errorf("remove: %d %q", code, out)
	}
}

func TestPrintReport(t *testing.T) {
	t.Parallel()
	r := ghproject.Report{Applied: true, Skipped: 1, Plan: ghproject.Plan{
		Changes:   []ghproject.Change{{IssueID: "a", Number: 7, Title: "Seven", Added: []string{"status:qa"}, Removed: []string{"status:ready"}, Warnings: []ghproject.Warning{{Text: "w"}}}},
		Edits:     []ghproject.Edit{{IssueID: "a", Field: "prio", Value: "p1"}, {IssueID: "a", Field: "assignee", Kind: ghproject.EditAssignees, Add: []string{"bob"}}},
		Conflicts: []ghproject.Conflict{{IssueID: "a", Field: "est", Local: "2", Project: ""}},
		Unmatched: []ghproject.Unmatched{{Number: 9, Title: "Nine", Reason: "a draft"}},
	}}
	var out bytes.Buffer
	printReport(&out, r)
	want := `#7 Seven
  + status:qa
  - status:ready
  -> Project: prio = p1
  -> GitHub issue: assignee +bob
  ! conflict on est: git-board 2, Project (empty) (never agreed)
  ! w
unmatched #9 Nine: a draft

summary: 1 issue(s) matched, 1 with label changes, 2 Project write(s), 1 conflict(s), 1 warning(s), 1 unmatched

applied.
1 change(s) not written to the Project: set write = true under [github-project] in .git-board.toml.
1 conflict(s) held: git board project resolve <issue number> <field> --ours | --theirs
`
	if out.String() != want {
		t.Errorf("got\n%s\nwant\n%s", out.String(), want)
	}
}
