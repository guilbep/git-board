package server

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
)

// bridge is a forge, as Settings › Sync shows it (whole.go).
type bridge struct {
	Name       string `json:"name"`
	Target     string `json:"target"`  // github, gitlab
	Project    string `json:"project"` // owner/repo, a GitLab path or id
	Where      string `json:"where"`   // the project's page
	Sync       bool   `json:"sync"`
	Discovered bool   `json:"discovered,omitempty"` // found in the git remotes, with no table
}

// settingsResponse is the settings page: how attachments and the GitHub
// Project are set up, and the cycle's stages (guessed, or the file's).
// The identity is GET /api/me, the forges GET /api/sync/all.
type settingsResponse struct {
	Uploads struct {
		Enabled   bool   `json:"enabled"`
		Endpoint  string `json:"endpoint,omitempty"`
		PublicURL string `json:"public_url,omitempty"`
		Prefix    string `json:"prefix,omitempty"`
	} `json:"uploads"`
	GithubProject config.GithubProject `json:"github_project"`
	ConfigPath    string               `json:"config_path"`
	Cycle         config.Stages        `json:"cycle"`
}

func (s *apiServer) handleSettings(w http.ResponseWriter, r *http.Request) {
	cfg := s.config()
	res := settingsResponse{GithubProject: cfg.GithubProject, ConfigPath: s.configPath(), Cycle: cfg.Stages()}
	if st := s.uploadStore(); st != nil {
		res.Uploads.Enabled, res.Uploads.Endpoint, res.Uploads.PublicURL, res.Uploads.Prefix = true, st.Endpoint, st.PublicURL, st.Prefix
	}
	writeJSON(w, http.StatusOK, res)
}

var errNoBridge = errors.New("no forge has that name")

// editForges changes .git-board.toml's forges, or its Project, the rest of
// the file untouched, and serves the result at once. beginWrite held.
func (s *apiServer) editForges(edit func(cfg *config.Config, text string) (string, error)) error {
	text, _, _, err := s.readConfigFile()
	if err != nil {
		return err
	}
	text, err = edit(s.config(), text)
	if err != nil {
		return err
	}
	cfg, res := checkConfig(text)
	if !res.OK {
		return fmt.Errorf("the settings wouldn't be valid: %s", cmp.Or(res.Message, fmt.Sprint(res.Error)))
	}
	if err := config.Save(s.configPath(), []byte(text)); err != nil {
		return err
	}
	s.setConfig(cfg)
	return nil
}

// handleBridgeSync turns a forge's sync on or off.
func (s *apiServer) handleBridgeSync(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Sync bool `json:"sync"`
	}
	if !decode(w, r, &req) {
		return
	}
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	err := s.editForges(func(cfg *config.Config, text string) (string, error) {
		f, ok := cfg.Forge(r.PathValue("name"))
		if !ok {
			return "", errNoBridge
		}
		f.Sync = req.Sync
		return config.WithForge(text, f), nil
	})
	if err != nil {
		writeBridgeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"sync": req.Sync})
}

func writeBridgeError(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	if errors.Is(err, errNoBridge) {
		code = http.StatusNotFound
	}
	writeError(w, code, err)
}

// handleCredentials lists the forges whose posting account has a token,
// the settings' or those found in the git remotes: forge, host and login,
// never a token.
func (s *apiServer) handleCredentials(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.b.Credentials(s.forges()))
}

// handleAddBridge adds a forge, sync on unless asked off, once the forge
// says the token is good and sees the project. The token, when pasted,
// goes to git's credential helper and is never sent back; else it is the
// one of an added forge (credential: its name), or the helper's for the
// host.
func (s *apiServer) handleAddBridge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Target     string `json:"target"`
		Name       string `json:"name"`
		URL        string `json:"url"`
		Credential string `json:"credential"`
		Token      string `json:"token"`
		Sync       *bool  `json:"sync"` // absent: on
	}
	if !decode(w, r, &req) {
		return
	}
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	var name string
	err := s.editForges(func(cfg *config.Config, text string) (string, error) {
		f, err := s.newForge(r.Context(), cfg, req.Target, req.Name, strings.TrimSpace(req.URL), req.Credential, strings.TrimSpace(req.Token))
		if err != nil {
			return "", err
		}
		f.Sync, name = req.Sync == nil || *req.Sync, f.Name
		if cfg.Discover {
			// a table turns the remotes' forges off: they get theirs
			if text, err = s.foundTables(r.Context(), text, f); err != nil {
				return "", err
			}
		}
		return config.WithForge(text, f), nil
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("the forge isn't added: %w", plainBridgeError(err)))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": name})
}

// foundTables writes a table for each forge found in the git remotes,
// GitLab's by its id, before f's: f's table would turn them off. It
// refuses f when a remote names it already, and a found forge whose id
// can't be read.
func (s *apiServer) foundTables(ctx context.Context, text string, f config.Forge) (string, error) {
	var left []string
	found := store.ResolveForges(ctx, s.forges(), func(g config.Forge, why error) {
		left = append(left, fmt.Sprintf("%s (%s %s): %v", g.Name, g.Host, g.Path, why))
	})
	if len(left) > 0 {
		return "", fmt.Errorf("the forges found in the remotes would be dropped, and one can't be written: %s", strings.Join(left, "; "))
	}
	for _, g := range found {
		if strings.EqualFold(g.Host+"/"+g.Project, f.Host+"/"+f.Project) {
			return "", fmt.Errorf("the git remotes name %s %s already: it is the forge %s", f.Host, f.Project, g.Name)
		}
		if g.Name == f.Name {
			return "", fmt.Errorf("a forge found in the remotes is named %s already", f.Name)
		}
		text = config.WithForge(text, g)
	}
	return text, nil
}

// newForge is the forge at the project's address, checked with the forge,
// its pasted token stored.
func (s *apiServer) newForge(ctx context.Context, cfg *config.Config, kind, name, address, credential, token string) (config.Forge, error) {
	if kind != "github" && kind != "gitlab" {
		return config.Forge{}, fmt.Errorf("forge %q: github or gitlab", kind)
	}
	u, err := url.Parse(address)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return config.Forge{}, fmt.Errorf("%q isn't a project's address, like https://github.com/owner/repo", address)
	}
	path, _, _ := strings.Cut(strings.Trim(u.Path, "/"), "/-/")
	f := config.Forge{Kind: kind, Host: u.Host, Project: strings.TrimSuffix(path, ".git")}
	if f.Name = name; f.Name == "" {
		f.Name = map[string]string{"github": "gh", "gitlab": "gl"}[kind]
		for n := 2; ; n++ {
			if _, taken := cfg.Forge(f.Name); !taken {
				break
			}
			f.Name = strings.TrimRight(f.Name, "0123456789") + strconv.Itoa(n)
		}
	}
	if _, taken := cfg.Forge(f.Name); taken {
		return config.Forge{}, fmt.Errorf("a forge is named %s already", f.Name)
	}
	if token == "" {
		login := ""
		if c, ok := cfg.Forge(credential); ok {
			login = c.Login
		}
		if token, err = s.b.Token(ctx, f, login); err != nil {
			return config.Forge{}, fmt.Errorf("no token: paste one: %w", err)
		}
		credential = "" // kept already
	} else {
		credential = "new"
	}
	if f.Login, f.Project, err = s.checkForge(ctx, f, token); err != nil {
		return config.Forge{}, err
	}
	if credential == "new" {
		if err := s.b.StoreToken(ctx, f, f.Login, token); err != nil {
			return config.Forge{}, err
		}
	}
	return f, nil
}

// forgeHTTP follows no redirect: a token goes to the forge's own host only.
var forgeHTTP = &http.Client{
	Timeout:       10 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// checkForge asks the forge whose token it is, and whether it sees the
// project: its login, and the project as the settings name it (GitLab's
// numeric id).
func (s *apiServer) checkForge(ctx context.Context, f config.Forge, token string) (login, project string, err error) {
	api, header, prefix := f.BaseURL()+"/api/v4", "PRIVATE-TOKEN", ""
	userPath, projectPath := "/user", "/projects/"+url.PathEscape(f.Project)
	if f.Kind == "github" {
		api, header, prefix = f.BaseURL()+"/api/v3", "Authorization", "Bearer "
		if f.Host == "github.com" {
			api = cmp.Or(s.githubAPI, "https://api.github.com")
		}
		projectPath = "/repos/" + f.Project
	}
	get := func(path string, out any) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, api+path, nil)
		if err != nil {
			return err
		}
		req.Header.Set(header, prefix+token)
		resp, err := forgeHTTP.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("GET %s: %s", path, resp.Status) // never the body: it may echo what was sent
		}
		return json.Unmarshal(data, out)
	}
	var user struct{ Login, Username string }
	if err := get(userPath, &user); err != nil {
		return "", "", err
	}
	var p struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	}
	if err := get(projectPath, &p); err != nil {
		return "", "", err
	}
	// as the forge spells it: links are keyed on it, and a discovered
	// forge of the same project reads the same (store.ResolveForges)
	project = cmp.Or(p.FullName, f.Project)
	if f.Kind == "gitlab" {
		project = strconv.FormatInt(p.ID, 10)
	}
	return cmp.Or(user.Login, user.Username), project, nil
}

// handleRemoveBridge removes a forge from the settings; its issues stay,
// and so does its token.
func (s *apiServer) handleRemoveBridge(w http.ResponseWriter, r *http.Request) {
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	err := s.editForges(func(cfg *config.Config, text string) (string, error) {
		if _, ok := cfg.Forge(r.PathValue("name")); !ok {
			return "", errNoBridge
		}
		return config.WithoutForge(text, r.PathValue("name")), nil
	})
	if err != nil {
		writeBridgeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{})
}

// githubLogin is GitHub's rule for a user's or an organization's name:
// letters, digits and single hyphens inside, 39 at most.
var githubLogin = regexp.MustCompile(`^[A-Za-z0-9](-?[A-Za-z0-9])*$`)

// handleGithubProject sets the GitHub Project, as `git board project
// configure` does; DELETE removes it. write = true stays for the same
// Project alone: another one is read until the file says otherwise. The
// last check was another Project's: it goes.
func (s *apiServer) handleGithubProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Owner  string `json:"owner"`
		Number int    `json:"number"`
	}
	if r.Method == http.MethodPut {
		if !decode(w, r, &req) {
			return
		}
		if len(req.Owner) > 39 || !githubLogin.MatchString(req.Owner) {
			writeError(w, http.StatusBadRequest, fmt.Errorf("owner %q isn't a GitHub user or organization", req.Owner))
			return
		}
		if req.Number <= 0 {
			writeError(w, http.StatusBadRequest, fmt.Errorf("number %d: a Project's number is 1 or more", req.Number))
			return
		}
	}
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	err := s.editForges(func(cfg *config.Config, text string) (string, error) {
		if req.Owner == "" {
			return config.WithoutGithubProject(text), nil
		}
		gp := cfg.GithubProject
		same := strings.EqualFold(gp.Owner, req.Owner) && gp.Number == req.Number
		return config.WithGithubProject(text, config.GithubProject{Owner: req.Owner, Number: req.Number, Write: gp.Write && same}), nil
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	s.last = nil
	writeJSON(w, http.StatusOK, s.status())
}

// plainBridgeError says in words what a forge answered a forge's check,
// the forge's own message kept at the end.
func plainBridgeError(err error) error {
	msg := err.Error()
	low := strings.ToLower(msg)
	var plain string
	switch {
	case strings.Contains(low, "401") || strings.Contains(low, "bad credentials") || strings.Contains(low, "unauthorized"):
		plain = "the forge refused the token: wrong, expired, or without the scope it needs (api, or repo)"
	case strings.Contains(low, "404") || strings.Contains(low, "doesn't exist") || strings.Contains(low, "non-existent"):
		plain = "the forge doesn't know this project, or the token can't see it"
	case strings.Contains(low, "no such host") || strings.Contains(low, "connection refused") || strings.Contains(low, "timeout"):
		plain = "git-board can't reach the forge at that address"
	default:
		return err
	}
	return fmt.Errorf("%s (%s)", plain, msg)
}
