package server

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/git-bug/git-bug/util/process"
)

// Instance is a running server, as its file in the repository says
// (<git dir>/git-board/server.json): written once it listens, removed
// when it stops. Several repositories each have their own.
type Instance struct {
	URL string `json:"url"`
	PID int    `json:"pid"`
}

func instancePath(gitDir string) string {
	return filepath.Join(gitDir, "git-board", "server.json")
}

func writeInstance(gitDir string, inst Instance) error {
	data, err := json.Marshal(inst)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(instancePath(gitDir)), 0o755); err != nil {
		return err
	}
	return os.WriteFile(instancePath(gitDir), data, 0o644)
}

// ReadInstance is what the file of the repository at gitDir says, true
// when there is one.
func ReadInstance(gitDir string) (Instance, bool) {
	var inst Instance
	data, err := os.ReadFile(instancePath(gitDir))
	return inst, err == nil && json.Unmarshal(data, &inst) == nil
}

// Running is the server of the repository at gitDir, true when it runs:
// its file names a live process, whose server answers that repository.
// A file left by a server that died, or whose port now serves another
// repository, is ignored.
func Running(gitDir string) (Instance, bool) {
	inst, ok := ReadInstance(gitDir)
	if !ok || !process.IsRunning(inst.PID) {
		return inst, false
	}
	c := http.Client{Timeout: time.Second}
	resp, err := c.Get(inst.URL + "/api/repository")
	if err != nil {
		return inst, false
	}
	defer resp.Body.Close()
	var repo repositoryResponse
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&repo) != nil {
		return inst, false
	}
	return inst, sameDir(repo.GitDir, gitDir)
}

func sameDir(a, b string) bool {
	a, errA := filepath.EvalSymlinks(a)
	b, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && a == b
}

type repositoryResponse struct {
	GitDir string `json:"gitDir"`
}

// handleRepository answers which repository this server serves: how a
// "git board" tells its own board from another repository's.
func (s *apiServer) handleRepository(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, repositoryResponse{GitDir: s.gitDir})
}

// firstPort is where "git board" looks for a free port: a var for the
// tests.
var firstPort = 8080

// listenFree takes 127.0.0.1 on firstPort, else the next free port: one
// board per repository, each on its own port. 127.0.0.1 only, not
// localhost's [::1] too: [::1] alone free would leave http://localhost
// to whatever holds 127.0.0.1, another repository's board maybe.
func listenFree() (net.Listener, error) {
	var err error
	for p := firstPort; p < firstPort+100; p++ {
		var l net.Listener
		if l, err = net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p))); err == nil {
			return l, nil
		}
	}
	return nil, err
}
