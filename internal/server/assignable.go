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
	"sync"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
)

// assignablePath keeps, by forge name, who its forge said can be
// assigned at the last forge sync: the person boxes offer them.
var assignablePath = filepath.Join("git-board", "assignable.json")

// assignable asks a forge who can be assigned on its project, with its
// posting account's token.
// ponytail: one page of 100, paginate when a project has more
func (s *apiServer) assignable(ctx context.Context, f config.Forge) ([]string, error) {
	api, header, prefix := f.BaseURL()+"/api/v4/projects/"+url.PathEscape(f.Project)+"/users?per_page=100", "PRIVATE-TOKEN", "" // an id, or a found forge's path
	if f.Kind == "github" {
		base := f.BaseURL() + "/api/v3"
		if f.Host == "github.com" {
			base = cmp.Or(s.githubAPI, "https://api.github.com")
		}
		api, header, prefix = base+"/repos/"+f.Project+"/assignees?per_page=100", "Authorization", "Bearer "
	}
	token, err := s.b.Token(ctx, f, f.Login)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(header, prefix+token)
	resp, err := forgeHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s answered %s", f.Host, resp.Status)
	}
	var users []struct{ Login, Username string } // GitHub: login; GitLab: username
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
// all at once, once the run is done; keeps the answers, then counts them
// in the bridge's tally and tells the pages. A forge that doesn't answer
// is a warning: the run went well, the boxes keep the last list.
func (s *apiServer) fetchAssignable(job *forgeJob) {
	bridges := slices.DeleteFunc(slices.Clone(s.forges()), func(f config.Forge) bool { return !slices.Contains(job.Names, f.Name) })
	if len(bridges) == 0 {
		return
	}
	logins := make([][]string, len(bridges))
	errs := make([]error, len(bridges))
	var wg sync.WaitGroup
	for i, br := range bridges {
		wg.Go(func() { logins[i], errs[i] = s.assignable(context.Background(), br) })
	}
	wg.Wait()

	s.assignMu.Lock()
	saved, err := loadAssignable(s.b.Storage())
	if err != nil {
		saved = map[string][]string{}
	}
	kept := false
	for i, br := range bridges {
		if errs[i] == nil {
			saved[br.Name], kept = logins[i], true
		}
	}
	if kept {
		if err := saveAssignable(s.b.Storage(), saved); err != nil {
			log.Printf("keeping who can be assigned: %v", err)
		}
	}
	s.assignMu.Unlock()

	s.forgeMu.Lock()
	for i, br := range bridges {
		t := job.tally(br.Name)
		if errs[i] != nil {
			t.Warnings++
			t.Lines = append(t.Lines, "who can be assigned: "+errs[i].Error())
		} else {
			t.Assignable = len(logins[i])
			t.Lines = append(t.Lines, fmt.Sprintf("%d assignable people on %s", len(logins[i]), br.Where()))
		}
	}
	s.forgeMu.Unlock()
	s.events.publish(event{Kind: "sync"})
}

// people are who a person field offers: the git-bug identities and who
// the configured bridges' forges said can be assigned, once each, sorted.
func (s *apiServer) people() []string {
	var all []string
	if logins, err := s.b.Logins(); err == nil {
		all = logins
	}
	saved, _ := loadAssignable(s.b.Storage())
	for _, f := range s.forges() {
		all = append(all, saved[f.Name]...)
	}
	slices.SortFunc(all, func(a, b string) int { return cmp.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return slices.CompactFunc(all, strings.EqualFold)
}

func loadAssignable(fs store.Storage) (map[string][]string, error) {
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
func saveAssignable(fs store.Storage, m map[string][]string) error {
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
