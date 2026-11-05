package httpapi

import (
	"cmp"
	"context"
	"fmt"
	"reflect"

	"github.com/BurntSushi/toml"

	"github.com/guilbep/git-board/next/app"
)

// codeKeys are the keys of .git-board.toml that hold a command, or say
// what a run builds, publishes, deploys or trusts (ADR 0013, Amendment
// 2026-11-05): a write changing one passes the code gate (runsCode).
// [release] as a whole: build is a shell line (sh -c), binaries are
// built with go build, images with docker build (a Dockerfile's RUN),
// the chart with helm, deploy names the config repos the overlays are
// pushed to, checks the command a merge must pass. [cycle.tests] and
// [cycle.checks] are argvs that `git board verify` runs, and
// cycle.signers says whose signed results prove them.
var codeKeys = [][]string{{"release"}, {"cycle", "tests"}, {"cycle", "checks"}, {"cycle", "signers"}}

// codeChanged is the first code key whose value differs between two
// texts of .git-board.toml, "" when none does. An old text that doesn't
// parse holds none; a new one that doesn't, every one.
func codeChanged(old, new []byte) string {
	var a, b map[string]any
	if _, err := toml.Decode(string(old), &a); err != nil {
		a = nil
	}
	_, errB := toml.Decode(string(new), &b)
	for _, k := range codeKeys {
		va, vb := dig(a, k), dig(b, k)
		if errB != nil || !reflect.DeepEqual(va, vb) {
			name := k[0]
			for _, p := range k[1:] {
				name += "." + p
			}
			return name
		}
	}
	return ""
}

func dig(m map[string]any, path []string) any {
	var v any = m
	for _, p := range path {
		t, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = t[p]
	}
	return v
}

// codeWrite passes a config write of text through the code gate when it
// changes a code key, and answers the version the write must start from:
// base, or the file's current one, so the file checked is the file
// written over. A dry run writes nothing, and passes.
func codeWrite(ctx context.Context, dir, base string, text []byte, dry bool) (string, error) {
	cur, v, err := app.ConfigText(dir)
	if err != nil {
		return "", err
	}
	if dry {
		return base, nil
	}
	if base != "" && base != v {
		return base, nil // stale: the write refuses it
	}
	if key := codeChanged(cur, text); key != "" {
		if err := runsCode(ctx, fmt.Sprintf("a command of .git-board.toml (%s) is set", key)); err != nil {
			return "", err
		}
	}
	return cmp.Or(base, v), nil
}
