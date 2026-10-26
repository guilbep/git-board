package store_test

import (
	"testing"

	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

func TestRefsFingerprintFollowsWrites(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	before := b.RefsFingerprint()
	id := storetest.NewIssue(t, b, "one")
	created := b.RefsFingerprint()
	if created == before {
		t.Fatal("a new issue left the fingerprint as it was")
	}
	if _, err := b.ApplyLabelChange(id, []string{"status:ready"}, nil); err != nil {
		t.Fatal(err)
	}
	if b.RefsFingerprint() == created {
		t.Error("a label change left the fingerprint as it was")
	}
	labelled := b.RefsFingerprint()
	if _, err := b.CreateRelease(store.ReleaseRecord{Commit: "5e09a71"}); err != nil {
		t.Fatal(err)
	}
	if b.RefsFingerprint() == labelled {
		t.Error("a new release left the fingerprint as it was")
	}
	if b.RefsFingerprint() != b.RefsFingerprint() {
		t.Error("the fingerprint isn't stable")
	}
}
