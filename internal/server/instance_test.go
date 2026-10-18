package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/git-bug/git-bug/repository"

	"github.com/guilbep/git-board/internal/gitbugio"
)

// answering is a server whose /api/repository answers gitDir, as Serve's.
func answering(t *testing.T, gitDir string) string {
	t.Helper()
	ts := httptest.NewServer(newMux(&apiServer{gitDir: gitDir}, http.NotFoundHandler()))
	t.Cleanup(ts.Close)
	return ts.URL
}

// deadPID is the pid of a process that has exited.
func deadPID(t *testing.T) int {
	t.Helper()
	c := exec.Command(os.Args[0], "-test.run=^$")
	if err := c.Run(); err != nil {
		t.Fatalf("running %s: %v", os.Args[0], err)
	}
	return c.Process.Pid
}

func TestRunningIgnoresADeadServer(t *testing.T) {
	gitDir := t.TempDir()
	if err := writeInstance(gitDir, Instance{URL: answering(t, gitDir), PID: deadPID(t)}); err != nil {
		t.Fatal(err)
	}
	if inst, ok := Running(gitDir); ok {
		t.Errorf("found %+v, whose process is gone", inst)
	}
}

// Its port now serves another repository's board.
func TestRunningIgnoresAnotherRepositorysServer(t *testing.T) {
	gitDir := t.TempDir()
	if err := writeInstance(gitDir, Instance{URL: answering(t, t.TempDir()), PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	if inst, ok := Running(gitDir); ok {
		t.Errorf("found %+v, which serves another repository", inst)
	}
}

func TestRunningFindsThisRepositorysServer(t *testing.T) {
	gitDir := t.TempDir()
	want := Instance{URL: answering(t, gitDir), PID: os.Getpid()}
	if err := writeInstance(gitDir, want); err != nil {
		t.Fatal(err)
	}
	if got, ok := Running(gitDir); !ok || got != want {
		t.Errorf("Running = %+v, %v; want %+v", got, ok, want)
	}
}

// One board per repository: a port another server holds is skipped.
func TestListenFreeTakesTheNextFreePort(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	port := taken.Addr().(*net.TCPAddr).Port
	defer func(p int) { firstPort = p }(firstPort)
	firstPort = port

	l, err := listenFree()
	if err != nil {
		t.Fatalf("listenFree: %v", err)
	}
	defer l.Close()
	if got := l.Addr().(*net.TCPAddr); got.Port <= port || got.IP.String() != "127.0.0.1" {
		t.Errorf("listening on %v, want 127.0.0.1 past the taken port %d", got, port)
	}
}

// While a board runs, its file in the repository says where; it goes
// when the board stops, the repository's lock too.
func TestServeTellsWhereItRunsUntilItStops(t *testing.T) {
	dir := t.TempDir()
	if _, err := repository.InitDefault(dir, "git-bug"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	gitDir, err := repository.FindGitDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func(p int) { firstPort = p }(firstPort)
	firstPort = free.Addr().(*net.TCPAddr).Port
	_ = free.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	urls, done := make(chan string, 1), make(chan error, 1)
	go func() { done <- Serve(ctx, "", "", func(url string) { urls <- url }) }()
	var url string
	select {
	case url = <-urls:
	case err := <-done:
		t.Fatalf("Serve: %v", err)
	}
	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Errorf("url %q, want this machine's 127.0.0.1", url)
	}
	if got, ok := Running(gitDir); !ok || got.URL != url || got.PID != os.Getpid() {
		t.Errorf("Running = %+v, %v; want %s, this process", got, ok, url)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if _, err := os.Stat(instancePath(gitDir)); !os.IsNotExist(err) {
		t.Errorf("the file stays after the board stopped: %v", err)
	}
	b, err := gitbugio.Open(dir)
	if err != nil {
		t.Fatalf("the repository stays locked: %v", err)
	}
	_ = b.Close()
}
