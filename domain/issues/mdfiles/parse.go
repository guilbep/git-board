package mdfiles

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// A file, as a hand may write it (datastores.md, Importing Markdown): its
// front matter, its description up to the comments' line, then its
// comments. A comment is a heading "### <who> · <time>", its text, and
// the marker the render writes; a heading of that form without a marker
// is a new comment.

// file is one file read.
type file struct {
	front    Front
	body     string
	comments []comment
}

type comment struct {
	id, who string // id "" for a comment written by hand
	at      time.Time
	body    string
}

// commentHead is a comment's heading; commentMark its marker.
var (
	commentHead = regexp.MustCompile(`(?m)^### (.+?) · (\S+)[ \t]*$`)
	commentMark = regexp.MustCompile(`(?m)^<!-- git-board comment (\S+) -->[ \t]*$`)
)

// parse reads a file as Render writes it; its id may be missing.
func parse(data []byte) (file, error) {
	var f file
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
	// older one, or one written by hand) at its first comment heading
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
	// each rendered comment ends at its marker, so a "### " line in its
	// text stays its text; what follows the last marker is new comments
	for {
		m := commentMark.FindStringSubmatchIndex(left)
		if m == nil {
			break
		}
		chunk := left[:m[0]]
		c := comment{id: left[m[2]:m[3]]}
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
		c := comment{who: left[h[2]:h[3]]}
		c.at, _ = time.Parse(time.RFC3339, left[h[4]:h[5]])
		c.body = strings.TrimSpace(left[h[1]:stop])
		f.comments = append(f.comments, c)
	}
	return f, nil
}

// labels are the file's labels as the board reads them: kind:bug for a
// field, assignee:alice for an assignee.
func (f file) labels() []string {
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
