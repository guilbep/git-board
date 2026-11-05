package sync_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/guilbep/git-board/kernel"
	"github.com/guilbep/git-board/kernel/pack"
	"github.com/guilbep/git-board/store/mem"
	bsync "github.com/guilbep/git-board/sync"
	"github.com/guilbep/git-board/view"
)

var ctx = context.Background()

func op(kind string, data any) kernel.Op {
	raw, _ := json.Marshal(data)
	return kernel.Op{Kind: kind, Author: "4a9d2e7f1b0c6e38d5a2f9b4c7e1d0a36b8f2c95", Time: 1, Data: raw}
}

func board(t *testing.T) *kernel.Board {
	t.Helper()
	dir := t.TempDir()
	v, err := view.Open(ctx, filepath.Join(dir, "view.db"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := kernel.Open(ctx, mem.New(dir), v)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	return b
}

const (
	one = "1111111111111111111111111111111111111111"
	two = "2222222222222222222222222222222222222222"
	gb  = "3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5"
)

// The state rules (data-model.md, Sync state): the first link of an
// issue, and of a forge issue, holds; a seen counts for a linked issue
// and a known field; a posted once per comment and per forge comment;
// a text's last seen without sync's marker.
func TestRecords(t *testing.T) {
	t.Parallel()
	b := board(t)
	key := "github.com/acme/widgets"
	_, err := bsync.Kind.Create(ctx, b, bsync.StateID(key),
		op("seen", bsync.SeenData{Issue: one, Field: "title", Value: "before its link"}),
		op("link", bsync.LinkData{Issue: one, ForgeID: "101", Number: "1", URL: "https://github.com/acme/widgets/issues/1"}),
		op("link", bsync.LinkData{Issue: one, ForgeID: "102", Number: "2", URL: "https://github.com/acme/widgets/issues/2"}),
		op("link", bsync.LinkData{Issue: two, ForgeID: "101", Number: "1", URL: "https://github.com/acme/widgets/issues/1"}),
		op("seen", bsync.SeenData{Issue: one, Field: "title", Value: "Crash"}),
		op("seen", bsync.SeenData{Issue: one, Field: "colour", Value: "red"}),
		op("seen", bsync.SeenData{Issue: two, Field: "title", Value: "unlinked"}),
		op("seen", bsync.SeenData{Issue: one, Field: "body", Value: "Steps.\n\n<!-- git-board issue " + gb + " -->"}),
		op("posted", bsync.PostedData{Issue: one, Comment: gb + ":1", ForgeID: "9"}),
		op("posted", bsync.PostedData{Issue: one, Comment: gb + ":2", ForgeID: "9"}),
		op("run", bsync.RunData{Started: 5}),
	)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := bsync.Records(ctx, b, func(id kernel.ID, err error) { t.Errorf("%s: %v", id, err) })
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]bsync.Record{one: {{
		LinkData: bsync.LinkData{Issue: one, ForgeID: "101", Number: "1", URL: "https://github.com/acme/widgets/issues/1"},
		Forge:    bsync.StateID(key),
		Seen:     map[string]string{"title": "Crash", "body": "Steps."},
		Posted:   map[string]string{gb + ":1": "9"},
	}}}
	if !reflect.DeepEqual(recs, want) {
		t.Errorf("records:\n got %+v\nwant %+v", recs, want)
	}
}

// An entity that doesn't fold is left out, and told; the others read.
func TestRecordsBroken(t *testing.T) {
	t.Parallel()
	b := board(t)
	_, err := bsync.Kind.Create(ctx, b, bsync.StateID("github.com/acme/widgets"),
		op("link", bsync.LinkData{Issue: one, ForgeID: "101", Number: "1", URL: "https://github.com/acme/widgets/issues/1"}))
	if err != nil {
		t.Fatal(err)
	}
	data, err := pack.Encode(pack.Pack{Clock: 1, Ops: []kernel.Op{op("unlink", bsync.LinkData{Issue: one})}})
	if err != nil {
		t.Fatal(err)
	}
	broken := bsync.StateID("gitlab.com/acme/widgets")
	if _, err := b.Store().Add(ctx, "sync", string(broken), data, ""); err != nil {
		t.Fatal(err)
	}
	var told []kernel.ID
	recs, err := bsync.Records(ctx, b, func(id kernel.ID, err error) { told = append(told, id) })
	if err != nil || len(recs[one]) != 1 || !reflect.DeepEqual(told, []kernel.ID{broken}) {
		t.Errorf("records %v, told %v, err %v", recs, told, err)
	}
}
