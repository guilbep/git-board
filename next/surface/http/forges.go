package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/guilbep/git-board/next/app"
)

// The forges' tables and tokens (docs/rewrite/api.md, routes 33 to 36):
// each write is .git-board.toml's, as PATCH /api/config's, so the board
// opens again and the pages hear a config event; a token goes to git's
// credential helper, and is never answered nor logged.

type forgeNameRes struct {
	Name string `json:"name"`
}

func (s *Server) addForge(ctx context.Context, r app.NewForge) (Reply, error) {
	name, err := board(ctx).AddForge(ctx, r)
	if name != "" {
		if rerr := s.forgesWritten(ctx); rerr != nil && err == nil {
			err = rerr
		}
	}
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
	if err := board(ctx).SetForgeSync(r.Name, *r.Sync); err != nil {
		return forgeSyncRes{}, on("sync", err)
	}
	return forgeSyncRes{r.Name, *r.Sync}, s.forgesWritten(ctx)
}

func (s *Server) removeForge(ctx context.Context, r forgeReq) (Reply, error) {
	if err := board(ctx).RemoveForge(r.Name); err != nil {
		return Reply{}, err
	}
	return Reply{Status: http.StatusNoContent}, s.forgesWritten(ctx)
}

func credentials(ctx context.Context, _ none) (items[app.Credential], error) {
	cs, err := board(ctx).Credentials(ctx)
	return items[app.Credential]{Items: cs}, err
}

// forgesWritten opens the board again on its file written, and tells
// the pages.
func (s *Server) forgesWritten(ctx context.Context) error {
	if err := s.boards.reopen(ctx); err != nil {
		return err
	}
	s.publish(ctx, Event{Kind: "config", By: clientOf(ctx)})
	return nil
}

var errNoSync = errors.New("sync: true or false")
