package issues

import (
	"encoding/binary"
	"errors"
	"slices"
	"strings"

	"github.com/guilbep/git-board/kernel"
)

// Row is what the view keeps of an issue for lists: what ls shows and
// filters on, and no text but the title.
type Row struct {
	Title      string
	Status     Status
	Labels     []string
	Author     kernel.ID
	Created    int64
	Updated    int64
	Closed     int64 // the last close's time, while closed; else 0
	Comments   int
	Commenters []kernel.ID // who commented, the author excepted, each once, in order
}

// index is what the view keeps of an issue: its row; its labels, state
// and author as terms; its title, body and comments as texts. A deleted
// one is hidden (ADR 0019).
func index(is *State) kernel.Doc[Row] {
	r := Row{Title: is.Title, Status: is.Status, Labels: is.Labels, Author: is.Author,
		Created: is.Created, Updated: is.Updated, Comments: len(is.Comments)}
	if is.Status == Closed {
		r.Closed = is.Changes["status"].Time
	}
	comments := make([]string, 0, len(is.Comments))
	for _, c := range is.Comments {
		comments = append(comments, c.Body)
		if c.Author != is.Author && !slices.Contains(r.Commenters, c.Author) {
			r.Commenters = append(r.Commenters, c.Author)
		}
	}
	terms := make([]string, 0, len(is.Labels)+2)
	terms = append(terms, is.Labels...)
	terms = append(terms, "is:"+string(is.Status), "author:"+string(is.Author))
	return kernel.Doc[Row]{
		Row:   r,
		Terms: terms,
		// the comments are one text, as v1's index keeps them: a word of
		// several tokens may match across two
		Texts:   []kernel.Text{{Field: "title", Body: is.Title}, {Field: "body", Body: is.Body}, {Field: "comments", Body: strings.Join(comments, "\n")}},
		Created: is.Created,
		Updated: is.Updated,
		Hidden:  is.Deleted,
	}
}

// rowCodec is a Row in bytes: varints and length-prefixed strings, read
// back without reflection. A list decodes every row.
type rowCodec struct{}

func (rowCodec) Append(b []byte, r *Row) []byte {
	str := func(s string) {
		b = binary.AppendUvarint(b, uint64(len(s)))
		b = append(b, s...)
	}
	str(r.Title)
	str(string(r.Status))
	b = binary.AppendUvarint(b, uint64(len(r.Labels)))
	for _, l := range r.Labels {
		str(l)
	}
	str(string(r.Author))
	b = binary.AppendVarint(b, r.Created)
	b = binary.AppendVarint(b, r.Updated)
	b = binary.AppendVarint(b, r.Closed)
	b = binary.AppendUvarint(b, uint64(r.Comments))
	b = binary.AppendUvarint(b, uint64(len(r.Commenters)))
	for _, c := range r.Commenters {
		str(string(c))
	}
	return b
}

var errRow = errors.New("a row cut short")

// Decode reads a row; its strings are pieces of one: a row is one
// allocation for its text, one for its labels.
func (rowCodec) Decode(src []byte, r *Row) error {
	whole := string(src)
	b := src
	bad := false
	uv := func() uint64 {
		v, n := binary.Uvarint(b)
		if n <= 0 {
			bad = true
			return 0
		}
		b = b[n:]
		return v
	}
	sv := func() int64 {
		v, n := binary.Varint(b)
		if n <= 0 {
			bad = true
			return 0
		}
		b = b[n:]
		return v
	}
	str := func() string {
		n := uv()
		if bad || n > uint64(len(b)) {
			bad = true
			return ""
		}
		at := len(src) - len(b)
		b = b[n:]
		return whole[at : at+int(n)]
	}
	r.Title = str()
	r.Status = Status(str())
	r.Labels = make([]string, min(uv(), uint64(len(b))))
	for i := range r.Labels {
		r.Labels[i] = str()
	}
	r.Author = kernel.ID(str())
	r.Created, r.Updated, r.Closed = sv(), sv(), sv()
	r.Comments = int(uv())
	if n := uv(); n > 0 {
		r.Commenters = make([]kernel.ID, min(n, uint64(len(b))))
		for i := range r.Commenters {
			r.Commenters[i] = kernel.ID(str())
		}
	}
	if bad {
		return errRow
	}
	return nil
}
