package golden

import (
	"bufio"
	"context"
	"crypto/sha1"
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
)

// The PostgreSQL board is basic copied into PostgreSQL by v1's datastore
// copy (v1gen), kept as its tables' rows (tables.txt), and loaded into a
// database of its own at each checkout, on the server
// GIT_BOARD_TEST_POSTGRES names.

// PostgresEnv names the server: a DSN whose user may create databases.
const PostgresEnv = "GIT_BOARD_TEST_POSTGRES"

// ErrNoPostgres is a checkout of the PostgreSQL board with no server.
var ErrNoPostgres = errors.New(PostgresEnv + " names no PostgreSQL server")

// postgres is the datastore file's word for the PostgreSQL board.
const postgres = "postgres"

// tablesSQL is the board's tables (datastores.md, PostgreSQL), the
// contract: v1 reads them as made here.
const tablesSQL = `
CREATE SCHEMA git_board;
CREATE TABLE git_board.meta (key text PRIMARY KEY, value text NOT NULL);
CREATE TABLE git_board.entities (
	kind text NOT NULL, id text NOT NULL, version bigint NOT NULL,
	PRIMARY KEY (kind, id));
CREATE TABLE git_board.packs (
	kind text NOT NULL, id text NOT NULL, hash text NOT NULL, data bytea NOT NULL,
	PRIMARY KEY (kind, id, hash),
	FOREIGN KEY (kind, id) REFERENCES git_board.entities);
CREATE TABLE git_board.counter (
	one boolean PRIMARY KEY DEFAULT true CHECK (one), value bytea NOT NULL, version bigint NOT NULL);
`

// database is the checkout at dir's database name.
func database(dir string) string {
	h := sha1.Sum([]byte(dir))
	return "git_board_golden_" + hex.EncodeToString(h[:6])
}

// loadPostgres makes the checkout at dir's database and loads the board's
// rows in it; it answers its postgres:// URL, as board.datastore takes it.
func (b Board) loadPostgres(ctx context.Context, dir string) (string, error) {
	admin := os.Getenv(PostgresEnv)
	if admin == "" {
		return "", ErrNoPostgres
	}
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		return "", err
	}
	defer conn.Close(ctx)
	name := database(dir)
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		return "", err
	}
	cfg := conn.Config().Copy()
	cfg.Database = name
	db, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return "", err
	}
	defer db.Close(ctx)
	f, err := os.Open(filepath.Join(b.Dir, "tables.txt"))
	if err != nil {
		return "", err
	}
	defer f.Close()
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, tablesSQL); err != nil {
			return err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(nil, 64<<20)
		for sc.Scan() {
			f := strings.Fields(sc.Text())
			if len(f) == 0 || strings.HasPrefix(f[0], "#") {
				continue
			}
			var q string
			args := []any{}
			for _, v := range f[1:] {
				args = append(args, v)
			}
			switch {
			case f[0] == "meta" && len(f) == 3:
				q = "INSERT INTO git_board.meta (key, value) VALUES ($1, $2)"
			case f[0] == "entity" && len(f) == 4:
				q = "INSERT INTO git_board.entities (kind, id, version) VALUES ($1, $2, $3::bigint)"
			case f[0] == "pack" && len(f) == 5:
				q = "INSERT INTO git_board.packs (kind, id, hash, data) VALUES ($1, $2, $3, decode($4, 'hex'))"
			case f[0] == "counter" && len(f) == 3:
				q = "INSERT INTO git_board.counter (version, value) VALUES ($1::bigint, decode($2, 'hex'))"
			default:
				return fmt.Errorf("tables.txt: %q", sc.Text())
			}
			if _, err := tx.Exec(ctx, q, args...); err != nil {
				return err
			}
		}
		return sc.Err()
	})
	if err != nil {
		return "", fmt.Errorf("loading %s's tables: %w", b.Name, err)
	}
	u := url.URL{Scheme: "postgres", Host: net.JoinHostPort(cfg.Host, strconv.Itoa(int(cfg.Port))), Path: "/" + name, RawQuery: "sslmode=disable"}
	u.User = url.User(cfg.User)
	if cfg.Password != "" {
		u.User = url.UserPassword(cfg.User, cfg.Password)
	}
	return u.String(), nil
}

// Drop drops the database a checkout of the PostgreSQL board at dir
// made; nothing for another board.
func (b Board) Drop(ctx context.Context, dir string) error {
	if b.Datastore != postgres {
		return nil
	}
	admin := os.Getenv(PostgresEnv)
	if admin == "" {
		return nil
	}
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, "DROP DATABASE IF EXISTS "+database(dir)+" WITH (FORCE)")
	return err
}

// postgresIDs are the PostgreSQL board's issues, from its rows.
func (b Board) postgresIDs() ([]string, error) {
	data, err := os.ReadFile(filepath.Join(b.Dir, "tables.txt"))
	if err != nil {
		return nil, err
	}
	var ids []string
	for line := range strings.SplitSeq(string(data), "\n") {
		if f := strings.Fields(line); len(f) == 4 && f[0] == "entity" && f[1] == "issue" {
			ids = append(ids, f[2])
		}
	}
	return ids, nil
}
