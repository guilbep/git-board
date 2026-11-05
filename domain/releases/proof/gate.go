package proof

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

// Show is a file as revision r commits it; nil when r has no such
// file. A base's rules and signers file are read this way, never from the
// working tree.
func Show(ctx context.Context, dir, r, path string) ([]byte, error) {
	if err := rev(r); err != nil {
		return nil, err
	}
	if _, err := git(ctx, dir, nil, "cat-file", "-e", r+":"+path); err != nil {
		if _, rerr := git(ctx, dir, nil, "rev-parse", "--verify", "--quiet", r+"^{commit}"); rerr != nil {
			return nil, fmt.Errorf("no revision %s", r)
		}
		return nil, nil
	}
	return git(ctx, dir, nil, "show", r+":"+path)
}

// CheckStatus is a check's verdict at a commit.
type CheckStatus struct {
	Name string `json:"name"`
	Key  string `json:"key"`
	Status
}

// StatusAt is the verdict of a check at commit, over every writer's lines.
func StatusAt(ctx context.Context, dir, commit, name string, s Suite, signers []byte) (CheckStatus, error) {
	key, err := Key(ctx, dir, commit, name, s)
	if err != nil {
		return CheckStatus{}, err
	}
	lines, err := Lines(ctx, dir, key)
	if err != nil {
		return CheckStatus{}, err
	}
	return CheckStatus{Name: name, Key: key, Status: Verdict(ctx, lines, signers, name, key)}, nil
}

// GateResult is whether a commit may be marked ready for review.
type GateResult struct {
	Proven bool          `json:"proven"`
	Reason string        `json:"reason,omitempty"` // why not, or that nothing gates
	Checks []CheckStatus `json:"checks"`
}

// Base is what a base branch commits for its proofs: what runs and who
// may sign are the base's, never a branch's (ADR 0001, section 5). The
// application reads it from its own file at the base (app's
// AtBase); the domain reads no file of the application's (ADR 0005).
type Base struct {
	Branch string           // the base branch, as the gate's reason names it
	Tests  map[string]Suite // [cycle.tests.*]
	Checks map[string]Suite // [cycle.checks.*]: what gates
	// SignersFile is the signers file's path; Signers its content on the
	// base, nil when the base has none.
	SignersFile string
	Signers     []byte
}

// Gate tells whether every check of the base is proven at commit (ADR
// 0001, sections 6 and 9). Tests never gate; a check with where = "ci" is
// the forge's, M4 doesn't read it. No checks declared is open: a project
// that declares none has asked for no gate.
func (b Base) Gate(ctx context.Context, dir, commit string) (GateResult, error) {
	res := GateResult{Checks: []CheckStatus{}}
	var why []string
	for _, name := range slices.Sorted(maps.Keys(b.Checks)) {
		s := b.Checks[name]
		if s.Where == "ci" {
			continue
		}
		st, err := StatusAt(ctx, dir, commit, name, s, b.Signers)
		if err != nil {
			return GateResult{}, err
		}
		res.Checks = append(res.Checks, st)
		switch {
		case st.Proven:
		case st.Verdict == Failed:
			why = append(why, fmt.Sprintf("%s FAILED, signed by %s on %s", name, st.Signer, st.Result.At.Local().Format(time.DateTime)))
		default:
			why = append(why, fmt.Sprintf("%s not run at %s", name, short(commit)))
		}
	}
	switch {
	case len(res.Checks) == 0:
		res.Proven, res.Reason = true, "no checks declared on "+b.Branch
	case b.Signers == nil:
		res.Reason = fmt.Sprintf("no signers file %s on %s: no result can verify", b.SignersFile, b.Branch)
	case len(why) > 0:
		res.Reason = strings.Join(why, "; ")
	default:
		res.Proven = true
	}
	return res, nil
}

func short(sha string) string { return sha[:min(7, len(sha))] }
