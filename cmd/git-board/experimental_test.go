package main

import (
	"strings"
	"testing"
)

// The cycle's verify is hidden from the help and refuses, naming the
// switch, until [experimental] turns the cycle on (serial: it chdirs).
func TestVerifyHiddenUntilTheCycleIsOn(t *testing.T) {
	newTestRepo(t)

	help, err := execute(t, []string{"--help"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(help, "verify") {
		t.Errorf("the help lists verify:\n%s", help)
	}
	_, err = execute(t, []string{"verify"})
	if err == nil || !strings.Contains(err.Error(), "experimental feature, off") || !strings.Contains(err.Error(), "git config board.experimental cycle") {
		t.Errorf("off: err = %v, want the switch named", err)
	}

	// on, verify runs: whatever it says, not the switch
	writeConfig(t, testConfig+"\n[experimental]\nfeatures = [\"cycle\"]\n")
	_, err = execute(t, []string{"verify"})
	if err != nil && strings.Contains(err.Error(), "experimental feature") {
		t.Errorf("on: err = %v", err)
	}
}

// A broken [cycle] doesn't break the core commands while the cycle is off.
func TestBrokenCycleTableLeavesLsAlone(t *testing.T) {
	newTestRepo(t)
	writeConfig(t, testConfig+"\n[cycle]\nsteps = [\"review\", \"code\"]\n")
	out, err := execute(t, []string{"ls"})
	if err != nil {
		t.Fatalf("ls: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Fix invoice bug") {
		t.Errorf("ls:\n%s", out)
	}
}
