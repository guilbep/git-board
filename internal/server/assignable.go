package server

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/git-bug/git-bug/repository"

	"github.com/guilbep/git-board/internal/gitbugio"
)

// assignablePath keeps, by bridge name, who its forge said can be
// assigned at the last forge sync: the person boxes offer them.
var assignablePath = filepath.Join("git-board", "assignable.json")

var assignableHTTP = &http.Client{Timeout: 30 * time.Second}

// assignable asks a bridge's forge who can be assigned on its project,
// with the token git-bug holds for that forge.
// ponytail: one page of 100, paginate when a project has more
func (s *apiServer) assignable(ctx context.Context, br gitbugio.Bridge) ([]string, error) {
	base := strings.TrimSuffix(br.BaseURL, "/")
	var api, host, header, prefix string
	switch br.Target {
	case "github":
		api = cmp.Or(s.githubAPI, "https://api.github.com") + "/repos/" + br.Owner + "/" + br.Project + "/assignees?per_page=100"
		host, header, prefix = "github.com", "Authorization", "Bearer "
	case "gitlab":
		api = base + "/api/v4/projects/" + br.Project + "/users?per_page=100"
		header = "PRIVATE-TOKEN"
	case "gitea":
		api = base + "/api/v1/repos/" + br.Owner + "/" + br.Project + "/assignees"
		header, prefix = "Authorization", "token "
	default:
		return nil, fmt.Errorf("%s can't say who can be assigned", br.Target)
	}
	if host == "" {
		u, err := url.Parse(base)
		if err != nil {
			return nil, err
		}
		host = u.Hostname()
	}
	token := s.b.ForgeToken(host)
	if token == "" {
		return nil, fmt.Errorf("no token for %s", host)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(header, prefix+token)
	resp, err := assignableHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s answered %s", host, resp.Status)
	}
	var users []struct{ Login, Username string } // GitHub and Gitea: login; GitLab: username
	if err := json.Unmarshal(data, &users); err != nil {
		return nil, err
	}
	logins := []string{}
	for _, u := range users {
		if l := cmp.Or(u.Login, u.Username); l != "" {
			logins = append(logins, l)
		}
	}
	return logins, nil
}

// fetchAssignable asks each bridge of a forge sync who can be assigned,
// keeps the answers, and counts them in the bridge's tally. A forge that
// doesn't answer is a warning: the run went well, the boxes keep the
// last list.
func (s *apiServer) fetchAssignable(job *forgeJob) {
	bridges, _ := s.b.Bridges()
	saved, err := loadAssignable(s.b.Storage())
	if err != nil {
		saved = map[string][]string{}
	}
	for _, br := range bridges {
		if !slices.Contains(job.Names, br.Name) {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		logins, err := s.assignable(ctx, br)
		cancel()
		s.forgeMu.Lock()
		t := job.Bridges[br.Name]
		if t == nil {
			t = &bridgeTally{Lines: []string{}}
			job.Bridges[br.Name] = t
		}
		if err != nil {
			t.Warnings++
			t.Lines = append(t.Lines, "who can be assigned: "+err.Error())
		} else {
			t.Assignable = len(logins)
			t.Lines = append(t.Lines, fmt.Sprintf("%d assignable people on %s", len(logins), br.Where))
			saved[br.Name] = logins
		}
		s.forgeMu.Unlock()
	}
	if err := saveAssignable(s.b.Storage(), saved); err != nil {
		log.Printf("keeping who can be assigned: %v", err)
	}
}

// people are who a person field offers: the git-bug identities and who
// the configured bridges' forges said can be assigned, once each, sorted.
func (s *apiServer) people() []string {
	var all []string
	if logins, err := s.b.Logins(); err == nil {
		all = logins
	}
	saved, _ := loadAssignable(s.b.Storage())
	bridges, _ := s.b.Bridges()
	for _, br := range bridges {
		all = append(all, saved[br.Name]...)
	}
	slices.SortFunc(all, func(a, b string) int { return cmp.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return slices.CompactFunc(all, strings.EqualFold)
}

func loadAssignable(fs repository.LocalStorage) (map[string][]string, error) {
	f, err := fs.Open(assignablePath)
	if errors.Is(err, os.ErrNotExist) {
		return map[string][]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m := map[string][]string{}
	if err := json.NewDecoder(f).Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

// saveAssignable writes through a temporary file and a rename: a reader
// gets the old list or the new one.
func saveAssignable(fs repository.LocalStorage, m map[string][]string) error {
	if err := fs.MkdirAll(filepath.Dir(assignablePath), 0o755); err != nil {
		return err
	}
	f, err := fs.Create(assignablePath + ".tmp")
	if err != nil {
		return err
	}
	if err := json.NewEncoder(f).Encode(m); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return fs.Rename(assignablePath+".tmp", assignablePath)
}
