package server

import (
	"errors"
	"net/http"
	"reflect"

	"github.com/BurntSushi/toml"
)

// The code gate (ADR 0013, Amendment 2026-11-05): whoever reaches the port
// writes the board, which is accepted; they must not run code as the user.
// A request that runs code, or changes what code a later run runs, passes
// localWrite: refused when the server listens beyond loopback, and asking
// the board's own page otherwise. Those are the releases' cut, promotion,
// adoption, deploy and rollback, their setup, and a write of
// .git-board.toml that changes a key holding a command (codeKeys).

// refuseCode answers 403 when r may not run code, what saying what it
// would do ("releases are cut"), and tells whether it did.
func (s *apiServer) refuseCode(w http.ResponseWriter, r *http.Request, what string) bool {
	code, reason := s.localWrite(r, what)
	if code != "" {
		writeCoded(w, http.StatusForbidden, code, errors.New(reason))
	}
	return code != ""
}

// refuseCodeWrite is refuseCode for a write of .git-board.toml from old
// to new: only one that changes a code key passes the gate.
func (s *apiServer) refuseCodeWrite(w http.ResponseWriter, r *http.Request, old, new string) bool {
	return codeChanged(old, new) != "" && s.refuseCode(w, r, "commands of .git-board.toml are set")
}

// codeKeys are the keys of .git-board.toml that hold a command, or say
// what a run builds, deploys or trusts: [release] whole (build is run with
// sh -c, binaries with go build, images with docker build, the chart with
// helm, deploy the config repos pushed to, checks a merge's command),
// [cycle.tests] and [cycle.checks] (argvs a run runs) and cycle.signers.
var codeKeys = [][]string{{"release"}, {"cycle", "tests"}, {"cycle", "checks"}, {"cycle", "signers"}}

// codeChanged is the first code key whose value differs between two
// texts of .git-board.toml, "" when none does. An old text that doesn't
// parse holds none; a new one that doesn't, every one.
func codeChanged(old, new string) string {
	var a, b map[string]any
	if _, err := toml.Decode(old, &a); err != nil {
		a = nil
	}
	_, errB := toml.Decode(new, &b)
	for _, k := range codeKeys {
		if errB != nil || !reflect.DeepEqual(dig(a, k), dig(b, k)) {
			return k[0]
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
