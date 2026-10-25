package forge

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// git's credential helper is asked never to prompt, and one that hangs is
// given up on at CredentialTimeout: no token. Serial: it sets the
// environment and swaps the timeout.
func TestCredentialNeverWaits(t *testing.T) {
	helper := filepath.Join(t.TempDir(), "helper")
	script := `#!/bin/sh
[ "$1" = get ] || exit 0
case "$(cat)" in
*host=slow*) sleep 10 ;;
*) [ "$GCM_INTERACTIVE" = never ] && [ "$GIT_TERMINAL_PROMPT" = 0 ] && echo password=quiet ;;
esac
`
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	// An empty helper first drops the machine's own, keychains included.
	for k, v := range map[string]string{"GIT_CONFIG_COUNT": "2",
		"GIT_CONFIG_KEY_0": "credential.helper", "GIT_CONFIG_VALUE_0": "",
		"GIT_CONFIG_KEY_1": "credential.helper", "GIT_CONFIG_VALUE_1": helper} {
		t.Setenv(k, v)
	}
	saved := CredentialTimeout
	t.Cleanup(func() { CredentialTimeout = saved })
	CredentialTimeout = 2 * time.Second
	if got, err := Password(t.Context(), "https", "fast.example.test", "ann"); got != "quiet" || err != nil {
		t.Fatalf("got %q, %v", got, err)
	}
	start := time.Now()
	if _, err := Password(t.Context(), "https", "slow.example.test", "ann"); !errors.Is(err, ErrNoToken) {
		t.Fatalf("err = %v, want ErrNoToken", err)
	}
	if d := time.Since(start); d > 6*time.Second {
		t.Fatalf("waited %v for a helper that hangs", d)
	}
}
