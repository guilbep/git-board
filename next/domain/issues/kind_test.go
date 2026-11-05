package issues

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/guilbep/git-board/next/kernel"
)

// specBlocks are the JSON blocks of docs/spec/data-model.md: the spec's
// examples are the fold's fixtures, so an edit of either fails here.
func specBlocks(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile("../../../docs/spec/data-model.md")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range regexp.MustCompile("(?s)```json\n(.*?)```").FindAllStringSubmatch(string(b), -1) {
		out = append(out, m[1])
	}
	return out
}

// specOp is the spec's example operation of kind, in its Issues section.
func specOp(t *testing.T, blocks []string, kind, has string) kernel.Op {
	t.Helper()
	for _, b := range blocks {
		var op kernel.Op
		if json.Unmarshal([]byte(b), &op) == nil && op.Kind == kind && strings.Contains(b, has) {
			return op
		}
	}
	t.Fatalf("no %s example in data-model.md", kind)
	return kernel.Op{}
}

// The issue the spec folds (data-model.md › Issue state): its operations,
// at the ids the spec gives them, fold into the state it shows.
func TestSpecIssue(t *testing.T) {
	t.Parallel()
	blocks := specBlocks(t)
	const id, comment = "3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5", "8c04e5b1d97f2a63c18e0b4d5f6a7928e3b1c0d4"
	entry := func(opID string, op kernel.Op) kernel.Entry { return kernel.Entry{ID: kernel.OpID(opID), Op: op} }
	log := []kernel.Entry{
		entry(id+":0", specOp(t, blocks, "create", "Search ignores closed issues")),
		entry(comment+":0", specOp(t, blocks, "comment", "It finds them with is:closed.")),
		entry("1d9e6b0a4c3f8e2d7b5a9c1e0f4d6b8a2c3e5f70:0", specOp(t, blocks, "edit-text", id)),
		entry("1d9e6b0a4c3f8e2d7b5a9c1e0f4d6b8a2c3e5f70:1", specOp(t, blocks, "set-title", "")),
		entry("1d9e6b0a4c3f8e2d7b5a9c1e0f4d6b8a2c3e5f70:2", specOp(t, blocks, "set-status", "")),
		entry("1d9e6b0a4c3f8e2d7b5a9c1e0f4d6b8a2c3e5f70:3", specOp(t, blocks, "change-labels", "")),
	}
	st, err := Kind.Fold(id, log)
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]any
	for _, b := range blocks {
		if strings.Contains(b, `"changes"`) && strings.Contains(b, `"id": "`+id+`"`) {
			if err := json.Unmarshal([]byte(b), &want); err != nil {
				t.Fatal(err)
			}
		}
	}
	if want == nil {
		t.Fatal("no issue state in data-model.md")
	}
	if got := asSpec(t, st); !reflect.DeepEqual(got, want) {
		g, _ := json.MarshalIndent(got, "", "  ")
		t.Fatalf("the fold is\n%s\nthe spec says\n%v", g, want)
	}
}

// asSpec is a state as data-model.md writes one.
func asSpec(t *testing.T, st State) map[string]any {
	t.Helper()
	type change struct {
		Author kernel.ID      `json:"author"`
		Time   int64          `json:"time"`
		Origin *kernel.Origin `json:"origin,omitempty"`
	}
	type comment struct {
		ID     kernel.OpID       `json:"id"`
		Author kernel.ID         `json:"author"`
		Time   int64             `json:"time"`
		Edited int64             `json:"edited"`
		Origin *kernel.Origin    `json:"origin,omitempty"`
		Body   string            `json:"body"`
		Meta   map[string]string `json:"meta,omitempty"`
	}
	s := struct {
		ID       kernel.ID         `json:"id"`
		Author   kernel.ID         `json:"author"`
		Created  int64             `json:"created"`
		Updated  int64             `json:"updated"`
		Origin   *kernel.Origin    `json:"origin,omitempty"`
		Title    string            `json:"title"`
		Body     string            `json:"body"`
		Status   Status            `json:"status"`
		Labels   []string          `json:"labels"`
		Comments []comment         `json:"comments"`
		Changes  map[string]change `json:"changes"`
	}{ID: st.ID, Author: st.Author, Created: st.Created, Updated: st.Updated, Origin: st.Origin, Title: st.Title,
		Body: st.Body, Status: st.Status, Labels: st.Labels, Comments: []comment{}, Changes: map[string]change{}}
	for _, c := range st.Comments {
		s.Comments = append(s.Comments, comment{c.ID, c.Author, c.Time, c.Edited, c.Origin, c.Body, c.Meta})
	}
	for k, c := range st.Changes {
		s.Changes[k] = change{c.Author, c.Time, c.Origin}
	}
	b, _ := json.Marshal(s)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func op(kind, data string) kernel.Op {
	return kernel.Op{Kind: kind, Author: "a", Time: 1, Data: json.RawMessage(data)}
}

// What the fold refuses (data-model.md › Issue state, its rules), and
// what it lets change nothing.
func TestFold(t *testing.T) {
	t.Parallel()
	const id = "1111111111111111111111111111111111111111"
	at := func(n int, o kernel.Op) kernel.Entry {
		return kernel.Entry{ID: kernel.OpID(strings.Repeat(string(rune('a'+n)), 40) + ":0"), Op: o}
	}
	create := at(0, op("create", `{"title":"t","body":"b"}`))
	comment := at(1, op("comment", `{"body":"c"}`))
	cid := string(comment.ID)
	for name, c := range map[string]struct {
		log  []kernel.Entry
		want error
	}{
		"no create first":       {[]kernel.Entry{comment}, errors.New("want create")},
		"a second create":       {[]kernel.Entry{create, at(1, op("create", `{}`))}, errors.New("second create")},
		"an unknown kind":       {[]kernel.Entry{create, at(1, op("frob", `{}`))}, ErrUnknownOp},
		"no author":             {[]kernel.Entry{create, at(1, kernel.Op{Kind: "comment", Time: 1})}, errors.New("no author")},
		"a bad status":          {[]kernel.Entry{create, at(1, op("set-status", `{"status":"done"}`))}, errors.New("bad status")},
		"an empty label":        {[]kernel.Entry{create, at(1, op("change-labels", `{"add":[""]}`))}, errors.New("bad label")},
		"a label with a line":   {[]kernel.Entry{create, at(1, op("change-labels", `{"remove":["a\nb"]}`))}, errors.New("bad label")},
		"an edit of nothing":    {[]kernel.Entry{create, at(1, op("edit-text", `{"target":"x:0","body":"y"}`))}, ErrBadTarget},
		"a pending target":      {[]kernel.Entry{create, at(1, op("set-meta", `{"target":"pending:0"}`))}, ErrBadTarget},
		"a delete of nothing":   {[]kernel.Entry{create, at(1, op("delete", `{"target":"x:0"}`))}, ErrBadTarget},
		"data that isn't data":  {[]kernel.Entry{create, at(1, op("set-title", `{"title":1}`))}, errors.New("cannot unmarshal")},
		"an edit of a deleted":  {[]kernel.Entry{create, comment, at(2, op("delete", `{"target":"`+cid+`"}`)), at(3, op("edit-text", `{"target":"`+cid+`","body":"z"}`)), at(4, op("set-meta", `{"target":"`+cid+`","meta":{"k":"v"}}`)), at(5, op("delete", `{"target":"`+cid+`"}`))}, nil},
		"labels apply in order": {[]kernel.Entry{create, at(1, op("change-labels", `{"add":["b","a"],"remove":["a"]}`)), at(2, op("change-labels", `{"add":["a"]}`))}, nil},
	} {
		st, err := Kind.Fold(id, c.log)
		switch {
		case c.want == nil && err != nil:
			t.Errorf("%s: %v", name, err)
		case c.want != nil && (err == nil || !errors.Is(err, c.want) && !strings.Contains(err.Error(), c.want.Error())):
			t.Errorf("%s: Fold = %v, want %v", name, err, c.want)
		case name == "an edit of a deleted" && (len(st.Comments) != 0 || len(st.Removed) != 1 || st.Removed[0].Body != "" || st.Changes["comment:"+cid] != (Change{})):
			t.Errorf("%s: %+v", name, st)
		case name == "labels apply in order" && strings.Join(st.Labels, ",") != "a,b":
			t.Errorf("%s: labels %v", name, st.Labels)
		}
	}
}

// A comment pulled twice (the same origin) is one comment, and either id
// names it; a delete of the issue hides it.
func TestDuplicateOriginAndDelete(t *testing.T) {
	t.Parallel()
	o := &kernel.Origin{Forge: "github.com/o/r", Event: "comment:5"}
	e := func(id string, kind, data string) kernel.Entry {
		x := op(kind, data)
		if kind == "comment" {
			x.Origin = o
		}
		return kernel.Entry{ID: kernel.OpID(id), Op: x}
	}
	st, err := Kind.Fold("i", []kernel.Entry{
		e("c:0", "create", `{"title":"t"}`),
		e("p1:0", "comment", `{"body":"first"}`),
		e("p2:0", "comment", `{"body":"second"}`),
		e("p3:0", "edit-text", `{"target":"p2:0","body":"edited"}`),
		e("p4:0", "delete", `{"target":"c:0"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Comments) != 1 || st.Comments[0].ID != "p1:0" || st.Comments[0].Body != "edited" || !st.Deleted {
		t.Fatalf("state = %+v", st)
	}
	if d := index(&st); !d.Hidden || d.Row.Comments != 1 {
		t.Fatalf("index = %+v", d)
	}
}

func TestRowCodec(t *testing.T) {
	t.Parallel()
	for _, r := range []Row{
		{},
		{Title: "café (#12)", Status: Closed, Labels: []string{"a", "prio:p1"}, Author: "x", Created: -5, Updated: 1 << 40, Closed: 7, Comments: 3, Commenters: []kernel.ID{"p", "q"}},
	} {
		var got Row
		b := rowCodec{}.Append(nil, &r)
		if err := (rowCodec{}).Decode(b, &got); err != nil {
			t.Fatal(err)
		}
		if r.Labels == nil {
			r.Labels = []string{}
		}
		if !reflect.DeepEqual(got, r) {
			t.Errorf("Decode(Append(%+v)) = %+v", r, got)
		}
		if len(b) > 0 {
			if err := (rowCodec{}).Decode(b[:len(b)-1], &got); err == nil {
				t.Errorf("a row cut short decodes: %+v", got)
			}
		}
	}
}

// An operation's data decodes as encoding/json decodes it.
func FuzzDecodeData(f *testing.F) {
	for _, s := range []string{`{"title":"t","body":"b"}`, `{"add":["a","b"],"remove":null}`, `{"meta":{"k":"v"},"Meta":{"x":"y"}}`,
		`{"target":"x:0","status":"open","extra":[1,{"a":null}]}`, `{"TITLE":"éé","add":[]}`, `{"title":5}`, ``, `null`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		got, gerr := decode(raw)
		var want data
		var werr error
		if len(raw) > 0 {
			werr = json.Unmarshal(raw, &want)
		}
		if (gerr == nil) != (werr == nil) || gerr == nil && !reflect.DeepEqual(got, want) {
			t.Fatalf("decode(%q) = %+v, %v; encoding/json: %+v, %v", raw, got, gerr, want, werr)
		}
	})
}
