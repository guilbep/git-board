package ghpr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
)

// fake is GitHub's API answering with h, as the token "tok".
func fake(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization %q", got)
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return &Client{API: srv.URL, Token: "tok"}
}

func serveFile(t *testing.T, name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(data)
	}
}

// co2's six required checks on dev (testdata/rules.json)
var six = []string{"Backend Lint/Type Check", "Frontent Lint/Type Check", "Backend Tests", "Frontend Tests", "Backend Migration Smoke (db-create + db-migrate)", "Docs Build"}

// co2-calculator's PR #3082 as GitHub answered it on 2026-10-17
// (testdata/pr.json, its repository renamed o/r).
func co2PR(t *testing.T) *PR {
	t.Helper()
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string            `json:"query"`
			Variables map[string]string `json:"variables"`
		}
		if r.Method != http.MethodPost || r.URL.Path != "/graphql" {
			t.Errorf("%s %s, want POST /graphql", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if v := req.Variables; v["owner"] != "o" || v["name"] != "r" || v["head"] != "fix/3075-x" {
			t.Errorf("variables %v", v)
		}
		serveFile(t, "pr.json")(w, r)
	})
	p, err := c.Find(context.Background(), "o/r", "fix/3075-x")
	if err != nil || p == nil {
		t.Fatalf("Find: %v, %v", p, err)
	}
	return p
}

func TestFind(t *testing.T) {
	p := co2PR(t)
	if p.Number != 3082 || p.State != "OPEN" || p.Base != "dev" || p.Head != "c1ee2d98598eb5a0dadeb8a67a2fee1e3593eae3" || p.Author != "dependabot" {
		t.Errorf("PR %+v", p)
	}
	if p.Decision != "REVIEW_REQUIRED" || p.Mergeable != "MERGEABLE" || p.MergeState != "BLOCKED" {
		t.Errorf("decision %q, mergeable %q, merge state %q", p.Decision, p.Mergeable, p.MergeState)
	}
	if len(p.Checks) != 13 || !slices.Equal(p.Requested, []string{"co2-calculator"}) {
		t.Errorf("%d checks, requested %v", len(p.Checks), p.Requested)
	}
	i := slices.IndexFunc(p.Checks, func(c Check) bool { return c.Name == "Frontend Tests" })
	if i < 0 || p.Checks[i].State != "failure" || !strings.HasPrefix(p.Checks[i].URL, "https://github.com/o/r/actions/") {
		t.Errorf("Frontend Tests %+v", p.Checks)
	}
}

// pulls answers the query with these PRs, newest first.
func pulls(nodes ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"data":{"repository":{"pullRequests":{"nodes":[%s]}}}}`, strings.Join(nodes, ","))
	}
}

func node(number int, state, repo string) string {
	head := `null`
	if repo != "" {
		head = fmt.Sprintf(`{"nameWithOwner":%q}`, repo)
	}
	return fmt.Sprintf(`{"number":%d,"state":%q,"headRepository":%s}`, number, state, head)
}

// A fork's branch of the same name is another PR; the open one comes
// before a closed one.
func TestFindSameRepoOpenFirst(t *testing.T) {
	for _, tc := range []struct {
		nodes []string
		want  int
	}{
		{[]string{node(9, "CLOSED", "o/r"), node(8, "OPEN", "fork/r"), node(7, "OPEN", "O/R"), node(6, "OPEN", "")}, 7},
		{[]string{node(9, "CLOSED", "o/r"), node(8, "OPEN", "fork/r"), node(5, "MERGED", "o/r")}, 9},
		{nil, 0},
	} {
		p, err := fake(t, pulls(tc.nodes...)).Find(context.Background(), "o/r", "fix/1-x")
		if err != nil {
			t.Fatal(err)
		}
		got := 0
		if p != nil {
			got = p.Number
		}
		if got != tc.want {
			t.Errorf("Find = #%d, want #%d (0: none)", got, tc.want)
		}
	}
}

func TestRequired(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/repos/o/r/rules/branches/dev" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		serveFile(t, "rules.json")(w, r)
	})
	got, err := c.Required(context.Background(), "o/r", "dev")
	if err != nil || !slices.Equal(got, six) {
		t.Errorf("Required = %q, %v", got, err)
	}
}

func names(checks []Check) []string {
	var res []string
	for _, c := range checks {
		res = append(res, c.Name)
	}
	return res
}

func TestGate(t *testing.T) {
	p := co2PR(t)
	Gate(p, six)
	if p.CanMerge || p.Checking {
		t.Errorf("can merge %v, checking %v", p.CanMerge, p.Checking)
	}
	if got := names(p.Checks[:6]); !slices.Equal(got, six) {
		t.Errorf("first checks %q, want the required ones in the rules' order", got)
	}
	for i, c := range p.Checks {
		if c.Required != (i < 6) {
			t.Errorf("check %d %+v", i, c)
		}
	}
	if want := []string{"Frontent Lint/Type Check", "Frontend Tests", "an approval from co2-calculator"}; !slices.Equal(p.Waits, want) {
		t.Errorf("Waits = %q, want %q", p.Waits, want)
	}
}

// A required check that never reported isn't in GitHub's rollup: it
// still shows, expected, and the merge waits for it.
func TestGateMissingRequired(t *testing.T) {
	p := &PR{State: "OPEN", Base: "dev", Mergeable: "MERGEABLE", MergeState: "BLOCKED",
		Checks: []Check{{Name: "Lint", State: "success"}, {Name: "Docs Build", State: "success"}}}
	Gate(p, []string{"Docs Build", "E2E"})
	if got := names(p.Checks); !slices.Equal(got, []string{"Docs Build", "E2E", "Lint"}) {
		t.Errorf("checks %q", got)
	}
	if e2e := p.Checks[1]; e2e.State != "expected" || !e2e.Required {
		t.Errorf("E2E %+v", e2e)
	}
	if !slices.Equal(p.Waits, []string{"E2E"}) || p.CanMerge {
		t.Errorf("Waits %q, can merge %v", p.Waits, p.CanMerge)
	}
}

func TestGateClean(t *testing.T) {
	for _, state := range []string{"CLEAN", "HAS_HOOKS", "UNSTABLE"} {
		p := &PR{State: "OPEN", Base: "dev", Mergeable: "MERGEABLE", MergeState: state, Decision: "APPROVED",
			Checks: []Check{{Name: "Tests", State: "success"}, {Name: "Audit", State: "failure"}}}
		Gate(p, []string{"Tests"})
		if !p.CanMerge || len(p.Waits) != 0 {
			t.Errorf("%s: can merge %v, waits %q", state, p.CanMerge, p.Waits)
		}
	}
	merged := &PR{State: "MERGED", Mergeable: "UNKNOWN", MergeState: "UNKNOWN"}
	if Gate(merged, nil); merged.CanMerge || merged.Checking || len(merged.Waits) != 0 {
		t.Errorf("a merged PR %+v", merged)
	}
}

func TestGateChecking(t *testing.T) {
	p := &PR{State: "OPEN", Base: "dev", Mergeable: "UNKNOWN", MergeState: "UNKNOWN"}
	Gate(p, nil)
	if !p.Checking || p.CanMerge {
		t.Errorf("checking %v, can merge %v", p.Checking, p.CanMerge)
	}
}

func TestGateWords(t *testing.T) {
	for _, tc := range []struct {
		pr   PR
		want string
	}{
		{PR{Decision: "CHANGES_REQUESTED", MergeState: "BLOCKED", Reviews: []Review{{Login: "alice", State: "APPROVED"}, {Login: "bob", State: "CHANGES_REQUESTED"}}}, "changes bob requested"},
		{PR{Mergeable: "CONFLICTING", MergeState: "DIRTY"}, "conflicts with dev resolved"},
		{PR{MergeState: "BEHIND"}, "an update from dev"},
		{PR{Draft: true, MergeState: "DRAFT"}, "ready for review (a draft)"},
		{PR{MergeState: "BLOCKED"}, "GitHub's rules"},
		{PR{Decision: "REVIEW_REQUIRED", MergeState: "BLOCKED"}, "an approval"},
		{PR{Decision: "REVIEW_REQUIRED", MergeState: "BLOCKED", Requested: []string{"alice", "co2-calculator"}}, "an approval from alice or co2-calculator"},
	} {
		p := tc.pr
		p.State, p.Base = "OPEN", "dev"
		if p.Mergeable == "" {
			p.Mergeable = "MERGEABLE"
		}
		Gate(&p, nil)
		if !slices.Equal(p.Waits, []string{tc.want}) || p.CanMerge {
			t.Errorf("%+v: waits %q, want %q", tc.pr, p.Waits, tc.want)
		}
	}
}

func TestOpen(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if r.Method != http.MethodPost || r.URL.Path != "/repos/o/r/pulls" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		want := map[string]string{"title": "CO2-3075 Fix it", "head": "fix/3075-x", "base": "dev", "body": "CO2-3075\n\nCloses #3071"}
		if fmt.Sprint(body) != fmt.Sprint(want) {
			t.Errorf("body %v, want %v", body, want)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"number": 9, "html_url": "https://github.com/o/r/pull/9"}`)
	})
	p, err := c.Open(context.Background(), "o/r", "fix/3075-x", "dev", "CO2-3075 Fix it", "CO2-3075\n\nCloses #3071")
	if err != nil || p.Number != 9 || p.URL != "https://github.com/o/r/pull/9" {
		t.Errorf("Open = %+v, %v", p, err)
	}
}

func TestMerge(t *testing.T) {
	refuse := false
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPut || r.URL.Path != "/repos/o/r/pulls/9/merge" || string(data) != `{"merge_method":"merge","sha":"abc"}` {
			t.Errorf("%s %s %s", r.Method, r.URL.Path, data)
		}
		if refuse {
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"message":"Head branch was modified. Review and try the merge again.","documentation_url":"https://docs.github.com"}`)
			return
		}
		_, _ = io.WriteString(w, `{"merged":true}`)
	})
	if err := c.Merge(context.Background(), "o/r", 9, "abc"); err != nil {
		t.Fatal(err)
	}
	refuse = true
	if err := c.Merge(context.Background(), "o/r", 9, "abc"); err == nil || !strings.Contains(err.Error(), "Head branch was modified. Review and try the merge again.") {
		t.Errorf("a refused merge: %v", err)
	}
}

func TestGraphQLError(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"repository":null},"errors":[{"type":"NOT_FOUND","message":"Could not resolve to a Repository with the name 'o/r'."}]}`)
	})
	if _, err := c.Find(context.Background(), "o/r", "x"); err == nil || !strings.Contains(err.Error(), "Could not resolve to a Repository") {
		t.Errorf("Find: %v", err)
	}
}
