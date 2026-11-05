package proof

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestSignVerify(t *testing.T) {
	t.Parallel()
	alice, signers := key(t, "alice")
	mallory, _ := key(t, "mallory")
	r := Result{Check: "vet", Verdict: Passed, Key: "abc", Platform: "darwin/arm64", Tools: "sha256:00", Commit: "def",
		At: time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC), Dur: 3 * time.Second, Log: "sha256:11"}
	var err error
	if r.Sig, err = Sign(ctx(t), alice, Payload(r.Line())); err != nil {
		t.Fatal(err)
	}
	line := r.Line()
	if got, ok := Parse(line); !ok || got != r {
		t.Fatalf("parse %q: %+v %v", line, got, ok)
	}
	if who, err := Verify(ctx(t), []byte(signers), Payload(line), r.Sig); err != nil || who != "alice" {
		t.Errorf("good line: %q %v", who, err)
	}

	changed := strings.Replace(line, "PASSED", "FAILED", 1)
	if _, err := Verify(ctx(t), []byte(signers), Payload(changed), r.Sig); err == nil {
		t.Error("a changed payload verified")
	}

	// the same payload, signed in git's namespace: a commit signature
	// replayed as a result
	other := exec.Command("ssh-keygen", "-Y", "sign", "-n", "git", "-f", alice)
	other.Stdin = strings.NewReader(string(Payload(line)))
	out, err := other.Output()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(ctx(t), []byte(signers), Payload(line), unarmor(string(out))); err == nil {
		t.Error("a signature in namespace git verified")
	}

	sig, _ := Sign(ctx(t), mallory, Payload(line))
	if _, err := Verify(ctx(t), []byte(signers), Payload(line), sig); !errors.Is(err, ErrUnknownSigner) {
		t.Errorf("a key not in the signers file: %v", err)
	}
	if _, err := Verify(ctx(t), nil, Payload(line), r.Sig); !errors.Is(err, ErrUnknownSigner) {
		t.Errorf("no signers file: %v", err)
	}

	// the public key's path signs too: what user.signingkey holds
	if _, err := Sign(ctx(t), alice+".pub", Payload(line)); err != nil {
		t.Errorf("signing with the .pub path: %v", err)
	}
}

func unarmor(s string) string {
	var b strings.Builder
	for _, l := range strings.Split(s, "\n") {
		if l != "" && !strings.HasPrefix(l, "-----") {
			b.WriteString(l)
		}
	}
	return b.String()
}

func TestParseRefuses(t *testing.T) {
	t.Parallel()
	good := "v1 vet PASSED key=k platform=p tools=t commit=c at=2026-10-22T09:00:00Z dur=3s log=l sig=s"
	if _, ok := Parse(good); !ok {
		t.Fatal("good line refused")
	}
	for _, l := range []string{
		"", "v2" + good[2:], strings.Replace(good, "PASSED", "SKIPPED", 1),
		strings.Replace(good, "dur=3s", "dur=3", 1), strings.Replace(good, " sig=s", "", 1),
		strings.Replace(good, "key=k platform=p", "platform=p key=k", 1), good + " extra=1",
	} {
		if _, ok := Parse(l); ok {
			t.Errorf("parsed %q", l)
		}
	}
	if p := string(Payload(good)); p != "git-board-ci/v1\n"+strings.TrimSuffix(good, " sig=s") {
		t.Errorf("payload %q", p)
	}
}
