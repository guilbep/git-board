package proof

import (
	"context"
	"maps"
	"slices"
)

// Status is a check's verdict at a key, over every writer.
type Status struct {
	// Verdict is PASSED or FAILED: the latest FAILED when a writer's verdict
	// is FAILED, else the latest PASSED; "" when no line verifies.
	Verdict string `json:"verdict,omitempty"`
	// Proven: one writer's verdict is PASSED and none is FAILED.
	Proven bool    `json:"proven"`
	By     string  `json:"by,omitempty"`     // the writer of the line shown
	Signer string  `json:"signer,omitempty"` // its signer, as the signers file names it
	Result *Result `json:"result,omitempty"`
	// Unverified counts the lines dropped for a signature that doesn't
	// verify against the signers file.
	Unverified int `json:"unverified,omitempty"`
}

// Verdict applies ADR 0001, section 6, to a check's lines at key: in each
// writer's ref, the last line that verifies is that writer's verdict.
func Verdict(ctx context.Context, lines map[string][]string, signers []byte, check, key string) Status {
	var st Status
	failed := false
	for _, writer := range slices.Sorted(maps.Keys(lines)) {
		ls := lines[writer]
		for i := len(ls) - 1; i >= 0; i-- {
			r, ok := Parse(ls[i])
			if !ok || r.Check != check || r.Key != key {
				continue
			}
			signer, err := Verify(ctx, signers, Payload(ls[i]), r.Sig)
			if err != nil {
				st.Unverified++
				continue
			}
			// the line shown: a FAILED over any PASSED, else the latest
			show := st.Result == nil ||
				(r.Verdict == Failed && st.Verdict == Passed) ||
				(r.Verdict == st.Verdict && r.At.After(st.Result.At))
			if show {
				st.Verdict, st.By, st.Signer, st.Result = r.Verdict, writer, signer, &r
			}
			failed = failed || r.Verdict == Failed
			break
		}
	}
	st.Proven = st.Verdict == Passed && !failed
	return st
}
