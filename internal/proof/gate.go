package proof

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/internal/config"
)

// AtBase is the config and the signers file as the base branch has them
// committed: what runs and who may sign are the base's, never a branch's
// (ADR 0001, section 5). No .git-board.toml there is an empty config; no
// signers file, nil signers.
func AtBase(ctx context.Context, dir, base string) (*config.Config, []byte, error) {
	if err := rev(base); err != nil {
		return nil, nil, err
	}
	cfg := &config.Config{}
	if data, err := show(ctx, dir, base, ".git-board.toml"); err != nil {
		return nil, nil, err
	} else if data != nil {
		if cfg, _, err = config.Check(data); err != nil {
			return nil, nil, fmt.Errorf(".git-board.toml on %s: %w", base, err)
		}
	}
	signers, err := show(ctx, dir, base, cfg.Cycle.SignersFile())
	return cfg, signers, err
}

// show is a file at a revision; nil when the revision has no such file.
func show(ctx context.Context, dir, r, path string) ([]byte, error) {
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
func StatusAt(ctx context.Context, dir, commit, name string, s config.Suite, signers []byte) (CheckStatus, error) {
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

// Gate tells whether every [cycle.checks.*] of the base branch's config is
// proven at commit (ADR 0001, sections 6 and 9). Tests never gate; a check
// with where = "ci" is the forge's, M4 doesn't read it. No checks declared
// is open: a project that declares none has asked for no gate.
func Gate(ctx context.Context, dir, base, commit string) (GateResult, error) {
	cfg, signers, err := AtBase(ctx, dir, base)
	if err != nil {
		return GateResult{}, err
	}
	res := GateResult{Checks: []CheckStatus{}}
	var why []string
	for _, name := range slices.Sorted(maps.Keys(cfg.Cycle.Checks)) {
		s := cfg.Cycle.Checks[name]
		if s.Where == "ci" {
			continue
		}
		st, err := StatusAt(ctx, dir, commit, name, s, signers)
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
		res.Proven, res.Reason = true, "no checks declared on "+base
	case signers == nil:
		res.Reason = fmt.Sprintf("no signers file %s on %s: no result can verify", cfg.Cycle.SignersFile(), base)
	case len(why) > 0:
		res.Reason = strings.Join(why, "; ")
	default:
		res.Proven = true
	}
	return res, nil
}

func short(sha string) string { return sha[:min(7, len(sha))] }
