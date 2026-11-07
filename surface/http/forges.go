package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/guilbep/git-board/app"
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
}

type forgeSyncRes struct {
	Name string `json:"name"`
	Sync bool   `json:"sync"`
}

func (s *Server) patchForge(ctx context.Context, r forgeReq) (forgeSyncRes, error) {
	if r.Sync == nil {
		return forgeSyncRes{}, invalid("sync", errNoSync)
	}
	if err := s.writeConfig(ctx, func() error { return board(ctx).SetForgeSync(r.Name, *r.Sync) }); err != nil {
		return forgeSyncRes{}, on("sync", err)
	}
	return forgeSyncRes{r.Name, *r.Sync}, nil
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
