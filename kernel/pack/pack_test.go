package pack

import (
	"encoding/json"
	"strings"
	"testing"
)

// A pack's hash is git's blob id of its bytes.
func TestHash(t *testing.T) {
	t.Parallel()
	// git hash-object of "hello", and of nothing
	for data, want := range map[string]string{
		"hello": "b6fc4c620b67d95f953a5c1c1230aaab5db5a1b0",
		"":      "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391",
	} {
		if got := Hash([]byte(data)); got != want {
			t.Errorf("Hash(%q) = %s, want %s", data, got, want)
		}
	}
}

// Header reads what the decoder reads: keys in any case, the last of two
// equal keys, whatever spacing and nesting; the fast scan and the decoder
// never disagree.
func TestHeader(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		pack  string
		clock uint64
		empty bool
	}{
		{`{"clock":3,"ops":[]}`, 3, true},
		{`{"clock":1,"nonce":"ab","ops":[{"kind":"create","author":"","time":1,"data":{"title":"]}[{\"","body":"x"}}]}`, 1, false},
		{` { "ops" : [ ] , "clock" : 7 } `, 7, true},
		{`{"Clock":2,"OPS":[{"kind":"c"}]}`, 2, false},
		{`{"clock":2,"clock":5,"ops":[1]}`, 5, false},
		{`{"clock":4,"ops":null}`, 4, true},
		{`{"clock":4}`, 4, true},
		{`{"clock":6,"ops":[1]}`, 6, false},   // an escaped key: the decoder reads it
		{`{"clock":1e0,"ops":[1]}`, 0, false}, // not an integer: the decoder refuses it
		{`{"clock":2,"ops":[1],"x":{"y":[{"z":"}"}]}}`, 2, false},
	} {
		clock, empty, err := Header([]byte(c.pack))
		var h struct {
			Clock uint64            `json:"clock"`
			Ops   []json.RawMessage `json:"ops"`
		}
		jerr := json.Unmarshal([]byte(c.pack), &h)
		if (err == nil) != (jerr == nil) || err == nil && (clock != h.Clock || empty != (len(h.Ops) == 0)) {
			t.Errorf("Header(%s) = %d, %t, %v; the decoder reads %d, %d ops, %v", c.pack, clock, empty, err, h.Clock, len(h.Ops), jerr)
		}
		if err == nil && (clock != c.clock || empty != c.empty) {
			t.Errorf("Header(%s) = %d, %t; want %d, %t", c.pack, clock, empty, c.clock, c.empty)
		}
	}
	for _, bad := range []string{`not json`, `{"clock":`, `[1]`, `{"clock":1,"ops":[}`} {
		if _, _, err := Header([]byte(bad)); err == nil {
			t.Errorf("Header(%s) = nil error, want one", bad)
		}
	}
}

// Encode writes a pack compact, keys in the spec's order, a first one
// with a nonce; Decode refuses an operation without a kind or a time.
func TestEncodeDecode(t *testing.T) {
	t.Parallel()
	b, err := Encode(Pack{Clock: 2, Ops: []Op{{Kind: "comment", Author: "a", Time: 5, Data: json.RawMessage(`{"body":"x"}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"clock":2,"ops":[{"kind":"comment","author":"a","time":5,"data":{"body":"x"}}]}`; string(b) != want {
		t.Fatalf("Encode = %s, want %s", b, want)
	}
	first, err := Encode(Pack{Clock: 1, Ops: []Op{{Kind: "create", Time: 1}}})
	if err != nil || !strings.HasPrefix(string(first), `{"clock":1,"nonce":"`) || len(first) < 40 {
		t.Fatalf("a first pack = %s, %v; want a nonce", first, err)
	}
	for _, bad := range []string{`{"clock":1,"ops":[{"time":1}]}`, `{"clock":1,"ops":[{"kind":"x"}]}`} {
		if _, err := Decode([]byte(bad)); err == nil {
			t.Errorf("Decode(%s) = nil error", bad)
		}
	}
	if _, err := Encode(Pack{Clock: 2, Ops: []Op{{Kind: "x"}}}); err == nil {
		t.Error("Encode of an operation without a time: nil error")
	}
}

// The order is by clock, then pack hash, each pack's operations in their
// order, the same bytes once (storage.md, Order: the spec's example).
func TestOrder(t *testing.T) {
	t.Parallel()
	op := func(kind string) string { return `{"kind":"` + kind + `","author":"a","time":1}` }
	first := `{"clock":1,"nonce":"00","ops":[` + op("create") + `]}`
	a := `{"clock":2,"ops":[` + op("comment") + `]}`
	b := `{"clock":2,"ops":[` + op("set-title") + `,` + op("set-status") + `]}`
	merge := `{"clock":3,"ops":[]}`
	packs := [][]byte{[]byte(merge), []byte(a), []byte(first), []byte(b), []byte(a)}
	es, clock, err := Order(packs)
	if err != nil {
		t.Fatal(err)
	}
	ha, hb := Hash([]byte(a)), Hash([]byte(b))
	want := []string{"create", "comment", "set-title", "set-status"}
	if hb < ha {
		want = []string{"create", "set-title", "set-status", "comment"}
	}
	var got []string
	for _, e := range es {
		got = append(got, e.Kind)
	}
	if strings.Join(got, " ") != strings.Join(want, " ") || clock != 3 {
		t.Fatalf("Order = %v at clock %d, want %v at 3", got, clock, want)
	}
	if es[0].ID != Hash([]byte(first))+":0" {
		t.Fatalf("the create's id = %s, want <its pack's hash>:0", es[0].ID)
	}
	if _, _, err := Order([][]byte{[]byte(`{"clock":1,"ops":[{"kind":""}]}`)}); err == nil {
		t.Fatal("Order of a pack it refuses: nil error")
	}
}
