package proof

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The verdicts, in the in-toto test-result vocabulary.
const (
	Passed = "PASSED"
	Failed = "FAILED"
)

// Result is one line of a note (ADR 0001, section 1):
//
//	v1 <check> PASSED|FAILED key=… platform=… tools=sha256:… commit=… at=… dur=…s log=sha256:… sig=…
type Result struct {
	Check    string        `json:"check"`
	Verdict  string        `json:"verdict"`
	Key      string        `json:"key"`
	Platform string        `json:"platform"`
	Tools    string        `json:"tools"`  // sha256:<hex> of the version commands' output
	Commit   string        `json:"commit"` // the commit checked out, for reading only
	At       time.Time     `json:"at"`
	Dur      time.Duration `json:"dur"`
	Log      string        `json:"log"` // sha256:<hex> of the run's output
	Sig      string        `json:"-"`
}

// fields are the key=value fields, in their order.
var fields = []string{"key", "platform", "tools", "commit", "at", "dur", "log", "sig"}

// Line is the result as a note's line.
func (r Result) Line() string {
	return fmt.Sprintf("v1 %s %s key=%s platform=%s tools=%s commit=%s at=%s dur=%ds log=%s sig=%s",
		r.Check, r.Verdict, r.Key, r.Platform, r.Tools, r.Commit, r.At.UTC().Format(time.RFC3339), int(r.Dur.Seconds()), r.Log, r.Sig)
}

// Payload is what a line's signature signs: "git-board-ci/v1", a newline,
// then the line up to " sig=".
func Payload(line string) []byte {
	if i := strings.LastIndex(line, " sig="); i >= 0 {
		line = line[:i]
	}
	return []byte("git-board-ci/v1\n" + line)
}

// Parse reads a line; false for one it can't, which a reader ignores.
func Parse(line string) (Result, bool) {
	f := strings.Split(line, " ")
	if len(f) != 3+len(fields) || f[0] != "v1" || f[1] == "" || (f[2] != Passed && f[2] != Failed) {
		return Result{}, false
	}
	v := map[string]string{}
	for i, name := range fields {
		val, ok := strings.CutPrefix(f[3+i], name+"=")
		if !ok {
			return Result{}, false
		}
		v[name] = val
	}
	at, err := time.Parse(time.RFC3339, v["at"])
	if err != nil {
		return Result{}, false
	}
	secs, ok := strings.CutSuffix(v["dur"], "s")
	n, err := strconv.Atoi(secs)
	if !ok || err != nil || v["key"] == "" || v["sig"] == "" {
		return Result{}, false
	}
	return Result{Check: f[1], Verdict: f[2], Key: v["key"], Platform: v["platform"], Tools: v["tools"], Commit: v["commit"],
		At: at, Dur: time.Duration(n) * time.Second, Log: v["log"], Sig: v["sig"]}, true
}
