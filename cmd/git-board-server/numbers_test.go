package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/git-bug/git-bug/repository/repotest"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// Creating an issue answers at once, even when origin hangs: the number
// comes after, off the request, and no other edit waits for it.
func TestNewIssueDoesNotWaitOnNumbering(t *testing.T) {
	remote := repotest.NewGoGitRepo(t, true)
	clone := repotest.NewGoGitRepo(t, false)
	repotest.AddRemote(t, clone, "origin", remote.GitDir)
	b, err := gitbugio.NewForRepo(clone)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	id, err := b.TestCache().Identities().NewFromGitUser()
	if err != nil {
		t.Fatal(err)
	}
	if err := b.TestCache().SetUserIdentity(id); err != nil {
		t.Fatal(err)
	}
	if err := b.InitNumbers("origin", "CO2", 1); err != nil {
		t.Fatal(err)
	}

	// origin now accepts connections and never answers
	hang, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var conns []net.Conn
	go func() {
		for {
			c, err := hang.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}
	}()
	if out, err := exec.Command("git", "-C", clone.GitDir, "config", "remote.origin.url", "git://"+hang.Addr().String()+"/x").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}

	s := &apiServer{cfg: &config.Config{}, b: b, me: "guilbep"}
	ts := httptest.NewServer(withCORS(newMux(s, http.NotFoundHandler())))
	t.Cleanup(func() {
		hang.Close()
		mu.Lock()
		for _, c := range conns {
			c.Close()
		}
		mu.Unlock()
		ts.Close()
		s.numberMu.Lock() // the numbering gave up
		s.numberMu.Unlock()
	})

	client := &http.Client{Timeout: 3 * time.Second}
	req, _ := http.NewRequest("POST", ts.URL+"/api/issues", strings.NewReader(`{"title":"Made while origin hangs"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("creating waited on the network: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d", resp.StatusCode)
	}
}
