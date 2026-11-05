package proof

import (
	"testing"
	"time"
)

func TestVerdict(t *testing.T) {
	t.Parallel()
	alice, aliceLine := key(t, "alice")
	bob, bobLine := key(t, "bob")
	mallory, _ := key(t, "mallory")
	signers := []byte(aliceLine + bobLine)
	at := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	line := func(keyFile, verdict string, min int) string {
		r := Result{Check: "vet", Verdict: verdict, Key: "k1", Platform: "p", Tools: "t", Commit: "c", At: at.Add(time.Duration(min) * time.Minute), Log: "l"}
		var err error
		if r.Sig, err = Sign(ctx(t), keyFile, Payload(r.Line())); err != nil {
			t.Fatal(err)
		}
		return r.Line()
	}
	pass, fail := line(alice, Passed, 1), line(alice, Failed, 2)
	bobPass, bobFail := line(bob, Passed, 3), line(bob, Failed, 3)
	forged := line(mallory, Passed, 9)

	for _, c := range []struct {
		name    string
		lines   map[string][]string
		verdict string
		proven  bool
		bad     int
	}{
		{"none", nil, "", false, 0},
		{"passed", map[string][]string{"a": {pass}}, Passed, true, 0},
		{"passed then failed", map[string][]string{"a": {pass, fail}}, Failed, false, 0},
		{"failed then passed", map[string][]string{"a": {fail, line(alice, Passed, 5)}}, Passed, true, 0},
		{"one writer passed, another failed", map[string][]string{"a": {pass}, "b": {bobFail}}, Failed, false, 0},
		{"two writers passed", map[string][]string{"a": {pass}, "b": {bobPass}}, Passed, true, 0},
		{"a forged line dropped", map[string][]string{"a": {fail, forged}}, Failed, false, 1},
		{"another key or check ignored", map[string][]string{"a": {pass, "v1 lint FAILED key=k1 platform=p tools=t commit=c at=2026-10-22T09:00:00Z dur=0s log=l sig=x"}}, Passed, true, 0},
	} {
		st := Verdict(ctx(t), c.lines, signers, "vet", "k1")
		if st.Verdict != c.verdict || st.Proven != c.proven || st.Unverified != c.bad {
			t.Errorf("%s: %+v", c.name, st)
		}
	}
	if st := Verdict(ctx(t), map[string][]string{"b": {bobPass}}, signers, "vet", "k1"); st.Signer != "bob" || st.By != "b" {
		t.Errorf("signer and writer: %+v", st)
	}
}
