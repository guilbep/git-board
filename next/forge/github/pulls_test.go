package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/guilbep/git-board/next/forge"
)

// TestPulls finds a branch's PR, the open one before a fork's and an
// older one, its gate from the base's rules, and opens, edits and merges
// as v1's ghpr did.
func TestPulls(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var writes []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		writes = append(writes, r.Method+" "+r.URL.Path+" "+string(body))
		mu.Unlock()
		switch {
		case r.URL.Path == "/graphql":
			var q struct{ Variables map[string]string }
			_ = json.Unmarshal(body, &q)
			if q.Variables["head"] != "feat/x" {
				_, _ = io.WriteString(w, `{"data":{"repository":{"pullRequests":{"nodes":[]}}}}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"repository":{"pullRequests":{"nodes":[
			  {"number":9,"state":"OPEN","headRepository":{"nameWithOwner":"fork/r"}},
			  {"number":8,"state":"CLOSED","headRepository":{"nameWithOwner":"o/r"}},
			  {"number":7,"url":"https://github.com/o/r/pull/7","state":"OPEN","title":"T","body":"a\r\nb","baseRefName":"main","headRefOid":"abc",
			   "headRepository":{"nameWithOwner":"O/R"},"author":{"login":"ann"},"reviewDecision":"REVIEW_REQUIRED","mergeable":"MERGEABLE","mergeStateStatus":"BLOCKED",
			   "reviewRequests":{"nodes":[{"requestedReviewer":{"login":"bob"}}]},
			   "commits":{"nodes":[{"commit":{"statusCheckRollup":{"contexts":{"nodes":[
			     {"__typename":"CheckRun","name":"lint","status":"COMPLETED","conclusion":"SKIPPED"},
			     {"__typename":"StatusContext","context":"ci","state":"PENDING"}]}}}}]}}]}}}}`)
		case r.URL.Path == "/repos/o/r/rules/branches/main":
			_, _ = io.WriteString(w, `[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"ci"},{"context":"e2e"}]}}]`)
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/pulls":
			_, _ = io.WriteString(w, `{"number":10,"html_url":"https://github.com/o/r/pull/10"}`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(srv.Close)
	ctx := t.Context()
	a, err := New(ctx, Options{Project: "o/r", API: srv.URL, Token: func(context.Context, forge.Account) (string, error) { return "t", nil }})
	if err != nil {
		t.Fatal(err)
	}
	pr, err := a.FindPR(ctx, "feat/x")
	if err != nil {
		t.Fatal(err)
	}
	if pr == nil || pr.Number != 7 || pr.Body != "a\nb" || pr.Author != "ann" || pr.CanMerge {
		t.Fatalf("found %+v", pr)
	}
	var names []string
	for _, c := range pr.Checks {
		names = append(names, c.Name+":"+c.State)
	}
	if !slices.Equal(names, []string{"ci:pending", "e2e:expected", "lint:success"}) {
		t.Fatalf("checks %q", names)
	}
	if !slices.Equal(pr.Waits, []string{"ci", "e2e", "an approval from bob"}) {
		t.Fatalf("waits %q", pr.Waits)
	}
	if none, err := a.FindPR(ctx, "other"); err != nil || none != nil {
		t.Fatalf("no PR: %+v, %v", none, err)
	}
	opened, err := a.OpenPR(ctx, "feat/x", "main", "CO2-1 T", "Refs: CO2-1")
	if err != nil || opened.Number != 10 || opened.State != "OPEN" {
		t.Fatalf("opened %+v, %v", opened, err)
	}
	if err := a.EditPR(ctx, 10, "New", "Body"); err != nil {
		t.Fatal(err)
	}
	if err := a.MergePR(ctx, 10, "abc"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	last := strings.Join(writes[len(writes)-3:], "\n")
	for _, want := range []string{`POST /repos/o/r/pulls {"base":"main","body":"Refs: CO2-1","head":"feat/x","title":"CO2-1 T"}`,
		`PATCH /repos/o/r/pulls/10 {"body":"Body","title":"New"}`, `PUT /repos/o/r/pulls/10/merge {"merge_method":"merge","sha":"abc"}`} {
		if !strings.Contains(last, want) {
			t.Errorf("no %s in\n%s", want, last)
		}
	}
}
