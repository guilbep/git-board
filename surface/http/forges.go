package httpapi

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/guilbep/git-board/app"
	"github.com/guilbep/git-board/kernel"
)

// The forges' tables and tokens (docs/rewrite/api.md, routes 33 to 36):
// each write is .git-board.toml's, as PATCH /api/config's, so the board
// opens again and the pages hear a config event; a token goes to git's
// credential helper, and is never answered nor logged.

type forgeNameRes struct {
	Name string `json:"name"`
}

func (s *Server) addForge(ctx context.Context, r app.NewForge) (Reply, error) {
	var name string
	err := s.writeConfig(ctx, func() (err error) {
		name, err = board(ctx).AddForge(ctx, r)
		return err
	})
	if err != nil {
		return Reply{}, on("url", err)
	}
	return Reply{Status: http.StatusCreated, Body: forgeNameRes{name}}, nil
}

type forgeReq struct {
	Name string `path:"name"`
	Sync *bool  `json:"sync"`
	Way  string `json:"way"` // when set: push, pull, or both
}

type forgeSyncRes struct {
	Name string `json:"name"`
	Sync bool   `json:"sync"`
	Way  string `json:"way"`
}

func (s *Server) patchForge(ctx context.Context, r forgeReq) (forgeSyncRes, error) {
	if r.Sync == nil && r.Way == "" {
		return forgeSyncRes{}, invalid("sync", errNoSync)
	}
	err := s.writeConfig(ctx, func() error {
		field := "sync"
		if r.Way != "" {
			field = "way"
		}
		return on(field, board(ctx).SetForge(ctx, r.Name, r.Sync, r.Way))
	})
	if err != nil {
		return forgeSyncRes{}, err
	}
	// the table as the board opened again reads it, not the request
	h := s.boards.get()
	defer s.boards.release(h)
	for _, f := range h.b.Config.Forges.List {
		if f.Name == r.Name {
			return forgeSyncRes{Name: f.Name, Sync: f.Sync, Way: f.Direction()}, nil
		}
	}
	return forgeSyncRes{}, on("sync", fmt.Errorf("%w: forge %q was removed meanwhile", kernel.ErrNotFound, r.Name))
}

func (s *Server) removeForge(ctx context.Context, r forgeReq) (Reply, error) {
	if err := s.writeConfig(ctx, func() error { return board(ctx).RemoveForge(r.Name) }); err != nil {
		return Reply{}, err
	}
	return Reply{Status: http.StatusNoContent}, nil
}

func credentials(ctx context.Context, _ none) (items[app.Credential], error) {
	cs, err := board(ctx).Credentials(ctx)
	return items[app.Credential]{Items: cs}, err
}

// forgeAuthReq is the read of the forges' tokens: check asks each forge
// whose each token is (the network), as forge status --check does.
type forgeAuthReq struct {
	Check bool `query:"check"`
}

// forgeAuths answers forge status: each forge, and for each of its
// accounts (the posting login, yours) whether a token is found and
// where. Never a token's value.
func forgeAuths(ctx context.Context, r forgeAuthReq) (items[app.ForgeStatus], error) {
	list, err := board(ctx).ForgeStatuses(ctx, r.Check)
	return items[app.ForgeStatus]{Items: list}, err
}

// newToken is a token to store: for a forge (by its name, as forge
// status lists it), for login ("" : yours on its host, else the forge's
// posting account), read from the body alone, never logged.
type newToken struct {
	Forge string `json:"forge"`
	Login string `json:"login"`
	Token string `json:"token"`
}

// tokenStored answers what was stored, where: the token itself never.
type tokenStored struct {
	Forge string `json:"forge"`
	Host  string `json:"host"`
	Login string `json:"login"`
}

// storeToken keeps a token in git's credential helper (forge login, ADR
// 0010): git credential approve with it on the input alone.
func (s *Server) storeToken(ctx context.Context, r newToken) (Reply, error) {
	if r.Token = strings.TrimSpace(r.Token); r.Token == "" {
		return Reply{}, invalid("token", errors.New("a token, on the body alone"))
	}
	f, who, err := board(ctx).LoginFor(ctx, r.Forge, r.Login)
	if err != nil {
		return Reply{}, on("forge", err)
	}
	if err := app.StoreToken(ctx, f, who, r.Token); err != nil {
		return Reply{}, on("token", err)
	}
	return Reply{Status: http.StatusCreated, Body: tokenStored{Forge: r.Forge, Host: f.Host, Login: who}}, nil
}

var errNoSync = errors.New("sync: true or false")

// deviceLogin is the server's device flow (routes 62, 63): one at a
// time, a new one ending the last. Its device code and its token stay in
// the server; the page sees the code to type, and the outcome.
type deviceLogin struct {
	mu     sync.Mutex
	cur    deviceRes
	cancel context.CancelFunc
}

// deviceRes is a device flow as the page sees it.
type deviceRes struct {
	Forge    string    `json:"forge,omitempty"`
	State    string    `json:"state"` // none, waiting, done, failed
	UserCode string    `json:"user_code,omitempty"`
	URL      string    `json:"verification_uri,omitempty"`
	Expires  time.Time `json:"expires_at,omitzero"`
	Login    string    `json:"login,omitempty"` // done: whose token was stored
	Error    string    `json:"error,omitempty"` // failed: why
}

// startDevice (62) starts a device flow for a forge: the person types
// user_code at verification_uri, and the server waits for the approval,
// beyond the request, then stores the token (63 says when).
func (s *Server) startDevice(ctx context.Context, r newToken) (Reply, error) {
	d, err := board(ctx).StartDevice(ctx, r.Forge)
	if err != nil {
		return Reply{}, on("forge", err)
	}
	wctx, cancel := context.WithCancel(s.base)
	s.device.mu.Lock()
	if s.device.cancel != nil {
		s.device.cancel()
	}
	s.device.cancel = cancel
	s.device.cur = deviceRes{Forge: r.Forge, State: "waiting", UserCode: d.UserCode, URL: d.URL, Expires: d.Expires}
	res := s.device.cur
	s.device.mu.Unlock()
	go func() {
		defer cancel()
		login, err := d.Wait(wctx)
		s.device.mu.Lock()
		defer s.device.mu.Unlock()
		if s.device.cur.UserCode != d.UserCode || wctx.Err() == context.Canceled && err != nil {
			return // a newer flow, or the server's end
		}
		if err != nil {
			s.device.cur.State, s.device.cur.Error = "failed", err.Error()
			return
		}
		s.device.cur.State, s.device.cur.Login = "done", login
		s.log.Info("a token was stored from the device flow", "forge", r.Forge, "login", login)
	}()
	return Reply{Status: http.StatusAccepted, Body: res}, nil
}

// deviceStatus (63) is the last device flow: state none when there was
// none.
func (s *Server) deviceStatus(context.Context, none) (deviceRes, error) {
	s.device.mu.Lock()
	defer s.device.mu.Unlock()
	return cmp.Or(s.device.cur, deviceRes{State: "none"}), nil
}
