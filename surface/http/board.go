package httpapi

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/guilbep/git-board/app"
	"github.com/guilbep/git-board/conf"
	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/domain/people"
	"github.com/guilbep/git-board/kernel"
)

// routes registers every route of docs/rewrite/api.md built so far.
func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/events", s.handleEvents)
	Route(mux, "GET /api/board", s.boardInfo)

	Route(mux, "GET /api/issues", listIssues)
	Route(mux, "POST /api/issues", createIssue)
	Route(mux, "GET /api/issues/{id}", showIssue)
	Route(mux, "PATCH /api/issues/{id}", patchIssue)
	Route(mux, "DELETE /api/issues/{id}", deleteIssue)
	Route(mux, "GET /api/issues/{id}/log", issueLog)
	Route(mux, "POST /api/issues/{id}/comments", addComment)
	Route(mux, "PATCH /api/issues/{id}/comments/{comment}", editComment)
	Route(mux, "DELETE /api/issues/{id}/comments/{comment}", deleteComment)
	Route(mux, "POST /api/issues/{id}/undo", undo)
	Route(mux, "GET /api/issues/{id}/follow", following)
	Route(mux, "PUT /api/issues/{id}/follow", follow)

	Route(mux, "GET /api/labels", labels)
	Route(mux, "GET /api/fields", fields)
	Route(mux, "POST /api/fields/{name}/migrate", migrateField)
	Route(mux, "GET /api/people", listPeople)
	Route(mux, "GET /api/me", me)
	Route(mux, "GET /api/state", s.state)
	Route(mux, "PUT /api/me", s.setMe)

	Route(mux, "GET /api/inbox", inbox)
	Route(mux, "POST /api/inbox/seen", seenIssues)
	Route(mux, "PUT /api/inbox/settings", inboxSettings)

	Route(mux, "GET /api/config", getConfig)
	Route(mux, "PUT /api/config", s.putConfig)
	Route(mux, "PATCH /api/config", s.patchConfig)
	// a template's path or URL reads this machine's files or the network:
	// on loopback alone (initConfig)
	initMux := http.NewServeMux()
	Route(initMux, "POST /api/config/init", s.initConfig)
	mux.Handle("POST /api/config/init", markLoopback(initMux))

	Route(mux, "GET /api/datastore", datastore)
	// what writes a secret, or says where the board goes, answers on
	// loopback alone (localRoutes)
	local := http.NewServeMux()
	Route(local, "PUT /api/datastore", s.putDatastore)
	Route(local, "POST /api/datastore/copy", copyDatastore)
	Route(local, "POST /api/forges", s.addForge)
	Route(local, "POST /api/forges/auth", s.storeToken)
	Route(local, "POST /api/forges/auth/device", s.startDevice)
	Route(local, "GET /api/forges/auth/device", s.deviceStatus)
	Route(local, "POST /api/remotes/{name}", s.addRemote)
	for _, p := range localRoutes {
		mux.Handle(p, loopbackOnly(local))
	}

	Route(mux, "GET /api/sync", s.syncStatus)
	Route(mux, "POST /api/sync", s.startSync)
	Route(mux, "POST /api/remotes/{name}/{action}", remote)
	Route(mux, "POST /api/numbers", numbers)
	Route(mux, "PATCH /api/forges/{name}", s.patchForge)
	Route(mux, "DELETE /api/forges/{name}", s.removeForge)
	Route(mux, "GET /api/forges/credentials", credentials)
	Route(mux, "GET /api/forges/auth", forgeAuths)

	Route(mux, "GET /api/github-project", s.projectStatus)
	Route(mux, "PUT /api/github-project", s.configureProject)
	Route(mux, "POST /api/github-project/{action}", s.projectAction)

	Route(mux, "POST /api/markdown", markdown)

	Route(mux, "GET /api/releases", s.releases)
	Route(mux, "POST /api/releases", s.cut)
	Route(mux, "GET /api/releases/{tag}", s.release)
	Route(mux, "PATCH /api/releases/{tag}", s.patchRelease)
	Route(mux, "POST /api/releases/{tag}/comments", s.releaseComment)
	Route(mux, "POST /api/environments/{env}/deploy", s.deploy)
	Route(mux, "GET /api/uploads", uploadStatus)
	mux.HandleFunc("GET /api/media", media)
	mux.HandleFunc("GET /api/export/markdown", exportMarkdown)
	mux.HandleFunc("POST /api/uploads", s.upload)
	Route(mux, "GET /api/issues/{id}/cycle", issueCycle)
	Route(mux, "POST /api/issues/{id}/cycle/{action}", cycleAct)
	Route(mux, "GET /api/cycle", workRows)
	Route(mux, "GET /api/insights", insights)
}

// none is a request with nothing to read.
type none struct{}

type boardRes struct {
	GitDir       string   `json:"git_dir"`
	Dir          string   `json:"dir"`
	Version      string   `json:"version"`
	Datastore    storeRes `json:"datastore"`
	Experimental []string `json:"experimental"`
	// FirstPull: this clone opened with no board, and the server pulled
	// origin's, or couldn't (docs/prd/clone-shows-board.md)
	FirstPull *app.FirstPull `json:"first_pull,omitempty"`
}

type storeRes struct {
	Name  string `json:"name"`
	Where string `json:"where"`
}

func (s *Server) boardInfo(ctx context.Context, _ none) (boardRes, error) {
	b := board(ctx)
	exp := b.Config.Features
	if exp == nil {
		exp = []string{}
	}
	return boardRes{GitDir: s.gitDir, Dir: b.Dir, Version: s.version, Experimental: exp, FirstPull: s.firstPulled(),
		Datastore: storeRes{Name: b.DatastoreName(), Where: app.Where(b.Kernel.Store())}}, nil
}

type labelsReq struct {
	Filter string `query:"filter"`
}

func labels(ctx context.Context, r labelsReq) (items[issues.LabelCount], error) {
	ls, err := board(ctx).Issues.LabelsIn(ctx, issues.ListReq{Filter: r.Filter})
	return items[issues.LabelCount]{Items: ls}, on("filter", err)
}

func fields(ctx context.Context, _ none) (map[string]issues.Described, error) {
	return board(ctx).Issues.Fields(ctx, time.Now())
}

type migrateReq struct {
	Name   string `path:"name"`
	DryRun bool   `query:"dry_run"`
}

type migrateRes struct {
	Plans []app.MigratePlan `json:"plans"`
	Error string            `json:"error,omitempty"` // the first forge's refusal; the plans say what landed
}

// migrateField is the forge-mapping migration (sync --migrate): with
// dry_run=1 its preview, per declared forge.
func migrateField(ctx context.Context, r migrateReq) (migrateRes, error) {
	plans, err := board(ctx).MigrateField(ctx, app.MigrateReq{Field: r.Name, DryRun: r.DryRun})
	if plans == nil {
		plans = []app.MigratePlan{}
	}
	if err != nil && app.Classify(err) == app.Usage {
		return migrateRes{}, err
	}
	res := migrateRes{Plans: plans}
	if err != nil {
		res.Error = app.NoCredentials(err.Error())
	}
	return res, nil
}

func listPeople(ctx context.Context, _ none) (items[people.Who], error) {
	ps, err := board(ctx).People.List(ctx)
	if ps == nil {
		ps = []people.Who{}
	}
	return items[people.Who]{Items: ps}, err
}

type meRes struct {
	Me      *people.Who `json:"me"`
	Avatars string      `json:"avatars"`
}

func me(ctx context.Context, _ none) (meRes, error) {
	b := board(ctx)
	res := meRes{Avatars: string(b.Issues.Avatars)}
	if res.Avatars == "" {
		res.Avatars = string(people.Forge)
	}
	w, ok, err := b.People.WhoAmI(ctx)
	if ok {
		res.Me = &w
	}
	return res, err
}

type setMeReq struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Email   string  `json:"email"`
	Avatars *string `json:"avatars"`
	// Global: write board.avatars at git's global level, so every
	// repository of the user reads it.
	Global bool `json:"global,omitempty"`
}

// setMe changes who writes, or where pictures come from, then opens the
// board again: the requests from now write as them.
func (s *Server) setMe(ctx context.Context, r setMeReq) (meRes, error) {
	b := board(ctx)
	var err error
	switch {
	case r.ID != "" && r.Name != "":
		return meRes{}, invalid("id", errors.New("an id, or a name and an email: not both"))
	case r.ID != "":
		err = b.People.Adopt(ctx, r.ID)
	case r.Name != "":
		// a person the board has already is the request's: write as them ({id})
		if _, err = b.People.New(ctx, r.Name, r.Email); errors.Is(err, people.ErrExists) {
			return meRes{}, invalid("name", err)
		}
	case r.Avatars == nil:
		return meRes{}, invalid("id", errors.New("an id, a name and an email, or avatars"))
	}
	if err == nil && r.Avatars != nil {
		err = b.SetAvatars(ctx, *r.Avatars, r.Global)
	}
	if err != nil {
		return meRes{}, on("id", err)
	}
	if err := s.boards.reopen(ctx); err != nil {
		return meRes{}, err
	}
	s.publish(ctx, Event{Kind: "me", By: clientOf(ctx)})
	h := s.boards.get()
	defer s.boards.release(h)
	return me(context.WithValue(ctx, boardKey{}, h.b), none{})
}

func inbox(ctx context.Context, _ none) (issues.StatusReport, error) {
	return board(ctx).Issues.Status(ctx, "", false, time.Now())
}

type seenReq struct {
	Issues []string `json:"issues"`
}

func seenIssues(ctx context.Context, r seenReq) (struct{}, error) {
	err := board(ctx).Issues.Seen(ctx, r.Issues, "")
	return struct{}{}, on("issues", err)
}

type inboxSettingsReq struct {
	Views  []string `json:"views"`
	Fields bool     `json:"fields"`
}

func inboxSettings(ctx context.Context, r inboxSettingsReq) (struct{}, error) {
	err := board(ctx).Issues.FollowViews(ctx, r.Views, r.Fields, "")
	return struct{}{}, on("views", err)
}

// configRes is .git-board.toml as the settings read it.
type configRes struct {
	// SchemaVersion is the file's schema: 1 until the versioned schema
	// lands (built beside this, in rewrite/conf).
	SchemaVersion int             `json:"schema_version"`
	Path          string          `json:"path"`
	Version       string          `json:"version"`
	Exists        bool            `json:"exists"`
	Text          string          `json:"text"`
	Keys          []conf.KeyValue `json:"keys"`
	Problems      []problem       `json:"problems"`
}

type problem struct {
	Table   string `json:"table,omitempty"`
	Message string `json:"message"`
}

// configOf is a file's text as the settings read it.
func configOf(dir string, text []byte) configRes {
	res := configRes{SchemaVersion: 1, Path: app.ConfigPath(dir), Version: app.TextVersion(text), Exists: text != nil,
		Text: string(text), Keys: []conf.KeyValue{}, Problems: []problem{}}
	if keys, err := conf.ListKeys(string(text)); err == nil && keys != nil {
		res.Keys = keys
	}
	if err := app.CheckConfig(dir, text); err != nil {
		p := problem{Message: err.Error()}
		var cp conf.Problem
		if errors.As(err, &cp) {
			p.Table = cp.Table
		}
		res.Problems = append(res.Problems, p)
	}
	return res
}

func getConfig(ctx context.Context, _ none) (Reply, error) {
	dir := board(ctx).Dir
	text, _, err := app.ConfigText(dir)
	if err != nil {
		return Reply{}, err
	}
	c := configOf(dir, text)
	return Reply{ETag: c.Version, Body: c}, nil
}

type putConfigReq struct {
	IfMatch string `header:"If-Match"`
	DryRun  bool   `query:"dry_run"`
	Text    string `json:"text"`
}

func (s *Server) putConfig(ctx context.Context, r putConfigReq) (Reply, error) {
	dir := board(ctx).Dir
	// checked first, then through the code gate, then written
	if err := app.SaveConfig(dir, []byte(r.Text), version(r.IfMatch), true); err != nil {
		return Reply{}, on("text", err)
	}
	base, err := codeWrite(ctx, dir, version(r.IfMatch), []byte(r.Text), r.DryRun)
	if err != nil {
		return Reply{}, on("text", err)
	}
	if !r.DryRun {
		if err := s.writeConfig(ctx, func() error { return app.SaveConfig(dir, []byte(r.Text), base, false) }); err != nil {
			return Reply{}, on("text", err)
		}
	}
	return configReply(dir, []byte(r.Text)), nil
}

type patchConfigReq struct {
	IfMatch string                     `header:"If-Match"`
	DryRun  bool                       `query:"dry_run"`
	Set     map[string]json.RawMessage `json:"set"`
}

func (s *Server) patchConfig(ctx context.Context, r patchConfigReq) (Reply, error) {
	dir := board(ctx).Dir
	set := map[string]any{}
	for k, raw := range r.Set {
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return Reply{}, invalid(k, err)
		}
		set[k] = tomlish(v)
	}
	// the file it would write, from the file as it is now (pinned to its
	// version), passes the code gate before it is written
	_, cur, err := app.ConfigText(dir)
	if err != nil {
		return Reply{}, err
	}
	base := cmp.Or(version(r.IfMatch), cur)
	text, err := app.SetConfigKeys(dir, set, base, true)
	if err != nil {
		return Reply{}, on("set", err)
	}
	if base, err = codeWrite(ctx, dir, base, text, r.DryRun); err != nil {
		return Reply{}, on("set", err)
	}
	if !r.DryRun {
		if err := s.writeConfig(ctx, func() (err error) {
			text, err = app.SetConfigKeys(dir, set, base, false)
			return err
		}); err != nil {
			return Reply{}, on("set", err)
		}
	}
	return configReply(dir, text), nil
}

// initConfigReq names the template the file starts from: "" the default,
// else the CLI's --template (a built-in name, a file, or a URL).
type initConfigReq struct {
	Template string `json:"template,omitempty"`
}

// initRes answers the file written, and what reading the forge said.
type initRes struct {
	Path  string   `json:"path"`
	Lines []string `json:"lines"`
}

// initConfig writes the default .git-board.toml, as `git board init`
// does: the template, seeded with origin's labels and its Project's
// fields when it can read them. It never overwrites one: a file there
// already is an error, 409.
func (s *Server) initConfig(ctx context.Context, r initConfigReq) (Reply, error) {
	// A client beyond localhost names no path or URL: it never chooses what
	// the server reads. "" is the global file's template, which the
	// machine's owner chose, as git board init uses it.
	if strings.ContainsAny(r.Template, `/\.:`) && !fromLoopback(ctx) {
		return Reply{}, on("template", forbidden(errors.New("a template's path or URL answers on localhost only: a built-in name answers anywhere")))
	}
	var res initRes
	err := s.writeConfig(ctx, func() (err error) {
		res.Path, res.Lines, err = app.InitConfig(ctx, board(ctx).Dir, r.Template)
		return err
	})
	if errors.Is(err, app.ErrConfigExists) {
		err = &app.Error{Class: app.Changed, Err: err} // 409: init never overwrites
	}
	if err != nil {
		return Reply{}, on("template", err)
	}
	return Reply{Status: http.StatusCreated, Body: res}, nil
}

// tomlish is a JSON value as TOML writes it: a whole number an integer.
func tomlish(v any) any {
	switch x := v.(type) {
	case float64:
		if x == float64(int64(x)) {
			return int64(x)
		}
	case []any:
		for i := range x {
			x[i] = tomlish(x[i])
		}
	case map[string]any:
		for k := range x {
			x[k] = tomlish(x[k])
		}
	}
	return v
}

// configReply answers the file.
func configReply(dir string, text []byte) Reply {
	c := configOf(dir, text)
	return Reply{ETag: c.Version, Body: c}
}

type markdownReq struct {
	Body string `json:"body"`
}

func markdown(ctx context.Context, r markdownReq) (map[string]string, error) {
	return map[string]string{"html": render(r.Body, board(ctx).MediaURL(ctx))}, nil
}

// gitDir is the repository's git directory, absolute: what a `git
// board` compares to find its own board running.
func gitDir(ctx context.Context, dir string) string {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--absolute-git-dir").Output()
	if err != nil {
		return ""
	}
	return filepath.Clean(strings.TrimSpace(string(out)))
}

func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }

func errNoRoute(r *http.Request) error {
	return fmt.Errorf("no route %s %s (docs/rewrite/api.md)", r.Method, r.URL.Path)
}

func cut(s, sep string) (string, string, bool) { return strings.Cut(s, sep) }

type kernelChange = kernel.Change

func appWhere(b *app.Board) string { return app.Where(b.Kernel.Store()) }
