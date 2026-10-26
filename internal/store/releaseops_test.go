package store

import (
	"encoding/json"
	"strings"
	"testing"
)

// A record without assets serializes as before this field: the records
// made before keep their IDs.
func TestReleaseCreateWithoutAssetsIsUnchanged(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(releaseData{Commit: "c", Previous: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "assets") {
		t.Errorf("a record without assets serializes them: %s", raw)
	}
}
