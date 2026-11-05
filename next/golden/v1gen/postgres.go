package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/guilbep/git-board/internal/store"
)

// errSkipped is a board that can't be made here: its folder stays as it is.
var errSkipped = errors.New("skipped")

// postgres is basic copied into PostgreSQL by v1's datastore copy, as
// docs/rewrite/compat.md's M0 suggested: the board's tables, dumped a row
// a line (tables.txt), which next/golden loads into a database of its
// own at each checkout. The server is GIT_BOARD_TEST_POSTGRES; without
// it, the board is left as it is.
func postgres(ctx context.Context, dir string) (meta, error) {
	admin := os.Getenv("GIT_BOARD_TEST_POSTGRES")
	if admin == "" {
		return meta{}, fmt.Errorf("%w: GIT_BOARD_TEST_POSTGRES names no server", errSkipped)
	}
	m, err := basic(ctx, dir)
	if err != nil {
		return meta{}, err
	}
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		return meta{}, err
	}
	defer conn.Close(ctx)
	b := make([]byte, 6)
	rand.Read(b)
	name := "git_board_v1gen_" + hex.EncodeToString(b)
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		return meta{}, err
	}
	defer conn.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	cfg := conn.Config()
	u := url.URL{Scheme: "postgres", Host: net.JoinHostPort(cfg.Host, strconv.Itoa(int(cfg.Port))), Path: "/" + name, RawQuery: "sslmode=disable"}
	u.User = url.User(cfg.User)
	if cfg.Password != "" {
		u.User = url.UserPassword(cfg.User, cfg.Password)
	}
	v1, err := store.Open(dir)
	if err != nil {
		return meta{}, err
	}
	_, err = v1.CopyGitTo(ctx, u.String())
	v1.Close()
	if err != nil {
		return meta{}, err
	}
	db := cfg.Copy()
	db.Database = name
	c, err := pgx.ConnectConfig(ctx, db)
	if err != nil {
		return meta{}, err
	}
	defer c.Close(ctx)
	var dump strings.Builder
	dump.WriteString("# git_board's rows, as v1's datastore copy wrote basic's: a row a line, bytes in hex\n")
	for _, q := range []struct{ name, sql string }{
		{"meta", "SELECT key, value FROM git_board.meta ORDER BY key"},
		{"entity", "SELECT kind, id, version::text FROM git_board.entities ORDER BY kind, id"},
		{"pack", "SELECT kind, id, hash, encode(data, 'hex') FROM git_board.packs ORDER BY kind, id, hash"},
		{"counter", "SELECT version::text, encode(value, 'hex') FROM git_board.counter"},
	} {
		rows, err := c.Query(ctx, q.sql)
		if err != nil {
			return meta{}, err
		}
		for rows.Next() {
			vals, err := rows.Values()
			if err != nil {
				rows.Close()
				return meta{}, err
			}
			dump.WriteString(q.name)
			for _, v := range vals {
				dump.WriteString(" " + fmt.Sprint(v))
			}
			dump.WriteString("\n")
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return meta{}, err
		}
	}
	tables := filepath.Join(filepath.Dir(dir), "tables.txt")
	if err := os.WriteFile(tables, []byte(dump.String()), 0o644); err != nil {
		return meta{}, err
	}
	m.tables, m.datastore = tables, "postgres"
	return m, nil
}
