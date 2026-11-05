package httpapi

import (
	"context"
	"errors"
	"net/http"

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

var errNoSync = errors.New("sync: true or false")
