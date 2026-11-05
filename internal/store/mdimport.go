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

	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/core/people"
)

// Importing a folder of Markdown (ADR 0002, section 5): the files
// ExportMarkdown writes, edited by hand or written new.
//
//   - A file whose id is an issue here: what differs from the issue becomes
//     its operations, as you: the title, the description, the state, the
//     fields, labels and assignees, a comment's text, and new comments.
//   - When the issue changed since the file's updated, a field that differs
//     is a conflict: the board's value stays, and the report names it. New
//     comments go in either way.
//   - A file with no id, or one this board doesn't have, is a new issue,
//     its author, comments and times as the file says; its new id is written
//     back into the file, so that a second import finds it.
//
// A comment is a heading "### <who> · <time>", its text, and the marker
// export writes; a heading of that form without a marker is a new comment.
// A Markdown file that isn't one of these is left alone.

// ImportReport is what an import did, by file name.
type ImportReport struct {
	Created   []string `json:"created"`   // new issues, their id written back into the file
	Updated   []string `json:"updated"`   // issues that took the file's changes
	Unchanged int      `json:"unchanged"` // issues the file says nothing new of
	Conflicts []string `json:"conflicts"` // "file: field", the board's value kept
	Skipped   []string `json:"skipped"`   // "file: why", not an issue file
}

// mdFile is one file read: its front matter, description and comments.
type mdFile struct {
	front    Front
	body     string
	comments []mdComment
}

type mdComment struct {
	id, who string // id "" for a comment written by hand
	at      time.Time
	body    string
}

// commentsMark is the line between the description and the comments.
const commentsMark = "<!-- git-board comments -->"

// commentHead is a comment's heading; commentMark its marker.
var (
	commentHead = regexp.MustCompile(`(?m)^### (.+?) · (\S+)[ \t]*$`)
	commentMark = regexp.MustCompile(`(?m)^<!-- git-board comment (\S+) -->[ \t]*$`)
)

// parseMD reads a file as ExportMarkdown writes it; its id may be missing.
func parseMD(data []byte) (mdFile, error) {
	var f mdFile
	rest, ok := bytes.CutPrefix(data, []byte("---\n"))
	if !ok {
		return f, fmt.Errorf("no front matter")
	}
	front, text, ok := bytes.Cut(rest, []byte("\n---\n"))
	if !ok {
		return f, fmt.Errorf("front matter not closed")
	}
	if err := yaml.Unmarshal(front, &f.front); err != nil {
		return f, err
	}
	if strings.TrimSpace(f.front.Title) == "" {
		return f, fmt.Errorf("no title")
	}
	s := string(text)
	// the description ends at the comments' line; a file without it (an
	// older export, or one written by hand) at its first comment heading
	var tail string
	if strings.HasPrefix(s, commentsMark) {
		tail = s[len(commentsMark):]
	} else if i := strings.Index(s, "\n"+commentsMark); i >= 0 {
		f.body, tail = strings.TrimSpace(s[:i]), s[i+1+len(commentsMark):]
	} else {
		end := len(s)
		if h := commentHead.FindStringIndex(s); h != nil {
			end = h[0]
		}
		f.body, tail = strings.TrimSpace(s[:end]), s[end:]
	}
	left := tail
	// each exported comment ends at its marker, so a "### " line in its
	// text stays its text; what follows the last marker is new comments
	for {
		m := commentMark.FindStringSubmatchIndex(left)
		if m == nil {
			break
		}
		chunk := left[:m[0]]
		c := mdComment{id: left[m[2]:m[3]]}
		if h := commentHead.FindStringSubmatchIndex(chunk); h != nil && strings.TrimSpace(chunk[:h[0]]) == "" {
			c.who = chunk[h[2]:h[3]]
			c.at, _ = time.Parse(time.RFC3339, chunk[h[4]:h[5]])
			chunk = chunk[h[1]:]
		}
		c.body = strings.TrimSpace(chunk)
		f.comments = append(f.comments, c)
		left = left[m[1]:]
	}
	heads := commentHead.FindAllStringSubmatchIndex(left, -1)
	for i, h := range heads {
		stop := len(left)
		if i+1 < len(heads) {
			stop = heads[i+1][0]
		}
		c := mdComment{who: left[h[2]:h[3]]}
		c.at, _ = time.Parse(time.RFC3339, left[h[4]:h[5]])
		c.body = strings.TrimSpace(left[h[1]:stop])
		f.comments = append(f.comments, c)
	}
	return f, nil
}

// labels are the file's labels as the board reads them: kind:bug for a
// field, assignee:alice for an assignee.
func (f mdFile) labels() []string {
	var out []string
	for ns, vs := range f.front.Fields {
		for _, v := range vs {
			out = append(out, ns+":"+v)
		}
	}
	out = append(out, f.front.Labels...)
	for _, a := range f.front.People.Assignees {
		out = append(out, "assignee:"+a)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// ImportMarkdown reads every *.md file in dir into the board. On a
// Markdown board it holds the board's lock, and its files follow.
func (b *Backend) ImportMarkdown(dir string) (ImportReport, error) {
	m, unlock, err := b.mdLock()
	if err != nil {
		return ImportReport{}, fmt.Errorf("markdown: %w", err)
	}
	defer unlock()
	rep, err := b.importMarkdown(dir)
	if m != nil {
		if _, rerr := b.ExportMarkdown(m.Dir(), nil); err == nil {
			err = rerr
		}
	}
	return rep, err
}

// importMarkdown is ImportMarkdown, a Markdown board's lock held.
func (b *Backend) importMarkdown(dir string) (ImportReport, error) {
	rep := ImportReport{Created: []string{}, Updated: []string{}, Conflicts: []string{}, Skipped: []string{}}
	paths, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return rep, err
	}
	slices.Sort(paths)
	persons, err := b.personsByName()
	if err != nil {
		return rep, err
	}
	for _, p := range paths {
		name := filepath.Base(p)
		data, err := os.ReadFile(p)
		if err != nil {
			return rep, err
		}
		f, err := parseMD(data)
		if err != nil {
			rep.Skipped = append(rep.Skipped, name+": "+err.Error())
			continue
		}
		if b.isDeleted(f.front.ID) { // its file came back, from a branch: it stays deleted
			rep.Skipped = append(rep.Skipped, name+": its issue was deleted")
			continue
		}
		if f.front.ID != "" && oplog.IsID(f.front.ID) {
			if _, _, err := b.loadIssue(f.front.ID); err == nil {
				changed, conflicts, err := b.importInto(f, persons)
				if err != nil {
					return rep, fmt.Errorf("%s: %w", name, err)
				}
				for _, c := range conflicts {
					rep.Conflicts = append(rep.Conflicts, name+": "+c)
				}
				if changed {
					rep.Updated = append(rep.Updated, name)
				} else if len(conflicts) == 0 {
					rep.Unchanged++
				}
				continue
			}
		}
		id, err := b.importNew(f, persons)
		if err != nil {
			return rep, fmt.Errorf("%s: %w", name, err)
		}
		th, err := b.Thread(id)
		if err != nil {
			return rep, err
		}
		out, err := renderMarkdown(th, nil)
		if err != nil {
			return rep, err
		}
		if err := os.WriteFile(p, out, 0o644); err != nil {
			return rep, err
		}
		rep.Created = append(rep.Created, name)
	}
	return rep, nil
}

// personsByName are the persons by name and by login, to find a file's
// people; a name found nowhere makes a person (personOf).
func (b *Backend) personsByName() (map[string]string, error) {
	ps, err := b.Persons()
	if err != nil {
		return nil, err
	}
	slices.SortFunc(ps, func(x, y people.Person) int { return strings.Compare(x.ID, y.ID) })
	out := map[string]string{}
	put := func(k, id string) {
		if _, taken := out[k]; k != "" && !taken { // the lowest id keeps a name
			out[k] = id
		}
	}
	for _, p := range ps {
		put(p.ID, p.ID)
		put(p.Name, p.ID)
		for _, a := range p.Accounts {
			put(a.Login, p.ID)
		}
	}
	return out, nil
}

// personOf is the person a file names, made when there is none; "" is you.
func (b *Backend) personOf(names map[string]string, who string) (string, error) {
	if who == "" {
		return b.me()
	}
	if id, ok := names[who]; ok {
		return id, nil
	}
	p, err := people.Create(ctx, b.ds, people.NewCreate("", who, ""))
	if err != nil {
		return "", err
	}
	names[who] = p.ID
	return p.ID, nil
}

// importNew makes a new issue of a file, as it says: its author, its
// times, its comments.
func (b *Backend) importNew(f mdFile, names map[string]string) (string, error) {
	author, err := b.personOf(names, f.front.People.Author)
	if err != nil {
		return "", err
	}
	at := func(op oplog.Op, t time.Time) oplog.Op {
		if !t.IsZero() {
			op.Time = t.Unix()
		}
		return op
	}
	ops := []oplog.Op{at(issue.NewCreate(author, f.front.Title, f.body), f.front.Created)}
	if f.front.State == "closed" {
		ops = append(ops, at(issue.SetStatus(author, issue.Closed), f.front.Updated))
	}
	if add, _ := b.stored(f.labels(), nil, nil); len(add) > 0 {
		ops = append(ops, at(issue.ChangeLabels(author, add, nil), f.front.Created))
	}
	for _, c := range f.comments {
		who, err := b.personOf(names, c.who)
		if err != nil {
			return "", err
		}
		ops = append(ops, at(issue.NewComment(who, c.body), c.at))
	}
	is, err := issue.Create(ctx, b.ds, ops...)
	if err != nil {
		return "", err
	}
	b.wrote(is.ID)
	return is.ID, nil
}

// importInto applies a file's changes to its issue. A field the file
// changes, on an issue that changed since the file's updated, is a
// conflict: left as the board has it.
func (b *Backend) importInto(f mdFile, names map[string]string) (changed bool, conflicts []string, err error) {
	cur, err := b.GetIssue(f.front.ID)
	if err != nil {
		return false, nil, err
	}
	moved := cur.UpdatedAt.UTC().Truncate(time.Second).After(f.front.Updated.UTC())
	may := func(field string) bool {
		if moved {
			conflicts = append(conflicts, field+": changed here since the file's updated, the board's kept")
		}
		return !moved
	}
	_, err = b.write(f.front.ID, func(me string, is *issue.Issue, _ *oplog.Log) ([]oplog.Op, error) {
		var ops []oplog.Op
		if f.front.Title != is.Title && may("title") {
			ops = append(ops, issue.SetTitle(me, f.front.Title))
		}
		if f.body != strings.TrimSpace(is.Body) && may("description") {
			ops = append(ops, issue.EditText(me, is.BodyOp, f.body))
		}
		if (f.front.State == "open" || f.front.State == "closed") && f.front.State != string(is.Status) && may("state") {
			ops = append(ops, issue.SetStatus(me, issue.Status(f.front.State)))
		}
		for _, c := range f.comments {
			if c.id == "" {
				who, err := b.personOf(names, c.who)
				if err != nil {
					return nil, err
				}
				op := issue.NewComment(who, c.body)
				if !c.at.IsZero() {
					op.Time = c.at.Unix()
				}
				ops = append(ops, op)
				continue
			}
			i := slices.IndexFunc(is.Comments, func(x issue.Comment) bool { return x.ID == c.id })
			if i >= 0 && strings.TrimSpace(is.Comments[i].Body) != c.body && may("comment "+c.id) {
				ops = append(ops, issue.EditText(me, c.id, c.body))
			}
		}
		changed = len(ops) > 0
		return ops, nil
	})
	if err != nil {
		return false, conflicts, err
	}
	want, have := f.labels(), slices.Sorted(slices.Values(cur.Labels))
	var add, remove []string
	for _, l := range want {
		if !slices.Contains(have, l) {
			add = append(add, l)
		}
	}
	for _, l := range have {
		if !slices.Contains(want, l) {
			remove = append(remove, l)
		}
	}
	if (len(add) > 0 || len(remove) > 0) && may("labels") {
		// as ApplyLabelChange, under the lock the import holds already: its
		// do would wait on it for ever
		_, err := b.write(f.front.ID, func(me string, is *issue.Issue, _ *oplog.Log) ([]oplog.Op, error) {
			added, removed := b.stored(add, remove, is.Labels)
			added = slices.DeleteFunc(slices.Clone(added), func(l string) bool { return slices.Contains(is.Labels, l) })
			removed = slices.DeleteFunc(slices.Clone(removed), func(l string) bool { return !slices.Contains(is.Labels, l) })
			if len(added) == 0 && len(removed) == 0 {
				return nil, nil
			}
			return []oplog.Op{issue.ChangeLabels(me, added, removed)}, nil
		})
		if err != nil {
			return changed, conflicts, fmt.Errorf("changing labels on %s: %w", f.front.ID, err)
		}
		changed = true
	}
	return changed, conflicts, nil
}
