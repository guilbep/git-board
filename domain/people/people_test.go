package people

import (
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/guilbep/git-board/kernel"
)

// The person the spec folds (data-model.md › Person state), from its
// examples: create, set-name, set-email, add-account, and an account on
// gitlab.epfl.ch; remove-account of one it hasn't changes nothing.
func TestSpecPerson(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile("../../docs/spec/data-model.md")
	if err != nil {
		t.Fatal(err)
	}
	people := string(b[strings.Index(string(b), "## People"):strings.Index(string(b), "## Sync")])
	var ops []kernel.Op
	var want map[string]any
	for _, m := range regexp.MustCompile("(?s)```json\n(.*?)```").FindAllStringSubmatch(people, -1) {
		var op kernel.Op
		if json.Unmarshal([]byte(m[1]), &op) == nil && op.Kind != "" {
			ops = append(ops, op)
		} else if strings.Contains(m[1], `"accounts"`) {
			json.Unmarshal([]byte(m[1]), &want)
		}
	}
	if len(ops) != 5 || want == nil {
		t.Fatalf("data-model.md › People: %d operations and a state %v; want 5 and one", len(ops), want)
	}
	const id = "b7e41c09d2a85f36e1c4a0d7b9f2e5c83a6d1f40"
	epfl := kernel.Op{Kind: "add-account", Author: id, Time: 1792830500, Data: json.RawMessage(`{"server":"gitlab.epfl.ch","id":"2342","login":"guilbep"}`)}
	var log []kernel.Entry
	for i, op := range append(ops, epfl) {
		pack := strings.Repeat("c", 40)
		if i == 0 {
			pack = id // the first pack's hash is the person's id
		}
		log = append(log, kernel.Entry{ID: kernel.OpID(pack + ":" + string(rune('0'+i))), Op: op})
	}
	p, err := Kind.Fold(id, log)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	b, _ = json.Marshal(p)
	json.Unmarshal(b, &got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the fold is %v, the spec says %v", got, want)
	}
}

func TestFold(t *testing.T) {
	t.Parallel()
	const id = "1111111111111111111111111111111111111111"
	create := kernel.Entry{ID: id + ":0", Op: kernel.Op{Kind: "create", Time: 1, Data: json.RawMessage(`{"name":"Ann"}`)}}
	later := func(kind, data, author string) kernel.Entry {
		return kernel.Entry{ID: kernel.OpID(strings.Repeat("2", 40) + ":0"), Op: kernel.Op{Kind: kind, Author: author, Time: 2, Data: json.RawMessage(data)}}
	}
	for name, c := range map[string]struct {
		log []kernel.Entry
		bad string
	}{
		"an author-less later op":  {[]kernel.Entry{create, later("set-name", `{"name":"B"}`, "")}, "no author"},
		"an empty name":            {[]kernel.Entry{create, later("set-name", `{"name":""}`, id)}, "empty name"},
		"an account with no login": {[]kernel.Entry{create, later("add-account", `{"server":"s","id":"1"}`, id)}, "empty login"},
		"an unknown kind":          {[]kernel.Entry{create, later("frob", `{}`, id)}, "unknown operation"},
		"no create first":          {[]kernel.Entry{later("set-email", `{}`, id)}, "want create"},
		"a login followed":         {[]kernel.Entry{create, later("add-account", `{"server":"s","id":"1","login":"a"}`, id), later("add-account", `{"server":"s","id":"1","login":"b"}`, id)}, ""},
	} {
		p, err := Kind.Fold(id, c.log)
		switch {
		case c.bad == "" && err != nil:
			t.Errorf("%s: %v", name, err)
		case c.bad != "" && (err == nil || !strings.Contains(err.Error(), c.bad)):
			t.Errorf("%s: Fold = %v, want %q", name, err, c.bad)
		case c.bad == "" && (len(p.Accounts) != 1 || p.Accounts[0].Login != "b"):
			t.Errorf("%s: accounts %v", name, p.Accounts)
		}
	}
}

func TestWho(t *testing.T) {
	t.Parallel()
	p := Person{ID: "x", Name: "Ann", Email: " Ann@EPFL.ch ", Accounts: []Account{{"gitlab.epfl.ch", "7", "ann2"}, {"github.com", "17847282", "ann"}}}
	if got := p.Avatar(Forge); got != "https://avatars.githubusercontent.com/u/17847282?s=88" {
		t.Errorf("Avatar(forge) = %s", got)
	}
	p.Accounts = p.Accounts[:1]
	if got := p.Avatar(Gravatar); !strings.HasPrefix(got, "https://gravatar.com/avatar/") || !strings.HasSuffix(got, "?d=404&s=88") {
		t.Errorf("Avatar(gravatar) = %s", got)
	}
	if p.Avatar(None) != "" || p.Avatar(Forge) != "" {
		t.Error("none and forge without a GitHub account: no picture")
	}
	d := Directory{ByID: map[kernel.ID]Person{"x": p}}
	if w := d.Who("x"); w.Login != "ann2" || w.Name != "Ann" {
		t.Errorf("Who = %+v", w)
	}
	if w := d.Who("abcdef0123"); w.Name != "abcdef0" {
		t.Errorf("an unknown person = %+v, want shown by 7 digits", w)
	}
}
