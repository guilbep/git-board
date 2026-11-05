package pack

import (
	"encoding/json"
	"reflect"
	"testing"
)

// packs that read the same through the fast path and encoding/json, or
// that only encoding/json reads: either way, Decode reads them as
// encoding/json does.
var corpus = []string{
	`{"clock":1,"nonce":"ab","ops":[{"kind":"create","author":"","time":1,"data":{"title":"t","body":"b"}}]}`,
	`{"clock":2,"ops":[{"kind":"comment","author":"a","time":2,"origin":{"forge":"f","event":"e"},"data":{"body":"x\ny \"q\" é é 😀"}}]}`,
	`{"Clock":2,"OPS":[{"KIND":"c","Author":"a","TIME":2}]}`,
	`{"clock":2,"clock":3,"ops":[{"kind":"c","time":1}]}`,
	`{"clock":2,"ops":[{"kind":"c","kind":"d","time":1}]}`,
	`{"clock":2,"ops":[{"kind":"c","time":1,"data":null}]}`,
	`{"clock":2,"ops":null}`,
	`{"clock":2,"ops":[{"kind":"c","time":1.5}]}`,
	`{"clock":2,"ops":[{"kind":"c","time":1e3}]}`,
	`{"clock":-1,"ops":[]}`,
	`{"clock":01,"ops":[]}`,
	`{"clock":2,"ops":[{"kind":"c\u0000","time":1}]}`,
	"{\"clock\":2,\"ops\":[{\"kind\":\"c\xff\",\"time\":1}]}",
	"{\"clock\":2,\"ops\":[{\"kind\":\"c\x01\",\"time\":1}]}",
	`{"clock":2,"ops":[{"kind":"c","time":1,"extra":{"a":[1,2,{"b":null}],"c":true}}],"more":false}`,
	`{"clock":2,"ops":[{"kind":"c","time":1}]} trailing`,
	`{"clock":2,"ops":[{"kind":"c","time":1},]}`,
	`{"clock":2,"ops":[{"kind":5,"time":1}]}`,
	`{"clock":2,"ops":[{"kind":"c","time":1}]}`,
	`{"clock":9223372036854775808,"ops":[]}`,
	`  {"clock" : 2 , "ops" : [ { "kind" : "c" , "time" : 1 , "data" : { "x" : [ ] } } ] }  `,
	`{"clock":2,"ops":[{"kind":"c","time":1,"origin":null}]}`,
	`{"clock":2,"ops":[{"kind":"c","time":1,"data":{"meta":{"a":"1","A":"2"}}}]}`,
	`[]`, `nul`, ``, `{"clock":2,"ops":[{"kind":"c","time":-0}]}`,
}

func decodeSlow(data []byte) (Pack, error) {
	var p Pack
	if err := json.Unmarshal(data, &p); err != nil {
		return p, err
	}
	return p, check(p.Ops)
}

func TestDecodeAsEncodingJSON(t *testing.T) {
	t.Parallel()
	for _, c := range corpus {
		same(t, []byte(c))
	}
}

func same(t *testing.T, data []byte) {
	t.Helper()
	got, gerr := Decode(data)
	want, werr := decodeSlow(data)
	if (gerr == nil) != (werr == nil) {
		t.Fatalf("Decode(%q): error %v, encoding/json's %v", data, gerr, werr)
	}
	if gerr == nil && !reflect.DeepEqual(norm(got), norm(want)) {
		t.Fatalf("Decode(%q) =\n%+v\nencoding/json:\n%+v", data, got, want)
	}
}

// norm compares data by its bytes: a nil and an empty RawMessage alike.
func norm(p Pack) Pack {
	for i := range p.Ops {
		if len(p.Ops[i].Data) == 0 {
			p.Ops[i].Data = nil
		}
	}
	return p
}

func FuzzDecode(f *testing.F) {
	for _, c := range corpus {
		f.Add([]byte(c))
	}
	f.Fuzz(func(t *testing.T, data []byte) { same(t, data) })
}

// Fields reads what encoding/json reads into the same fields, or says it
// can't.
func TestFields(t *testing.T) {
	t.Parallel()
	type d struct {
		Title string            `json:"title"`
		Add   []string          `json:"add"`
		Meta  map[string]string `json:"meta"`
	}
	read := func(raw string) (d, bool) {
		var v d
		ok := Fields([]byte(raw), func(k string, r *Reader) error {
			switch {
			case equalFold(k, "title"):
				return r.String(&v.Title)
			case equalFold(k, "add"):
				return r.Strings(&v.Add)
			case equalFold(k, "meta"):
				return r.StringMap(&v.Meta)
			}
			return r.Skip()
		})
		return v, ok
	}
	for _, raw := range []string{
		`{"title":"a","add":["x","y"],"meta":{"k":"v"},"other":[1,{"z":null}]}`,
		`{"Title":"a","ADD":[],"meta":null,"title2":"z"}`,
		`{"title":null}`,
		`{"title":"a","title":"b"}`,
		`{"add":"x"}`,
		`{"title":"éé"}`,
		`{}`,
	} {
		got, ok := read(raw)
		var want d
		err := json.Unmarshal([]byte(raw), &want)
		if ok && (err != nil || !reflect.DeepEqual(got, want)) {
			t.Errorf("Fields(%s) = %+v; encoding/json: %+v, %v", raw, got, want, err)
		}
	}
}
