// Package pgtest gives a test a fresh PostgreSQL database. The server is
// GIT_BOARD_TEST_POSTGRES, a DSN whose user may create databases; a test
// with none set is skipped.
package pgtest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// Env names the server.
const Env = "GIT_BOARD_TEST_POSTGRES"

// DSN makes a database for t, dropped when t ends, and answers its DSN.
func DSN(t testing.TB) string {
	t.Helper()
	cfg, name := create(t)
	dsn := fmt.Sprintf("host=%s port=%d user=%s dbname=%s sslmode=disable", cfg.Host, cfg.Port, cfg.User, name)
	if cfg.Password != "" {
		dsn += " password=" + cfg.Password
	}
	return dsn
}

// DSNCollated is DSN with a database whose collation isn't C: ICU's
// English, as many servers default to. It skips t on a server without ICU.
func DSNCollated(t testing.TB) string {
	t.Helper()
	cfg, name := create(t, "TEMPLATE template0 LOCALE_PROVIDER icu ICU_LOCALE 'en'")
	dsn := fmt.Sprintf("host=%s port=%d user=%s dbname=%s sslmode=disable", cfg.Host, cfg.Port, cfg.User, name)
	if cfg.Password != "" {
		dsn += " password=" + cfg.Password
	}
	return dsn
}

// URL is DSN as a postgres:// URL.
func URL(t testing.TB) string {
	t.Helper()
	cfg, name := create(t)
	u := url.URL{Scheme: "postgres", Host: net.JoinHostPort(cfg.Host, strconv.Itoa(int(cfg.Port))), Path: "/" + name, RawQuery: "sslmode=disable"}
	u.User = url.User(cfg.User)
	if cfg.Password != "" {
		u.User = url.UserPassword(cfg.User, cfg.Password)
	}
	return u.String()
}

// create makes the database, and answers the server's config and its name.
func create(t testing.TB, with ...string) (*pgx.ConnConfig, string) {
	t.Helper()
	admin := os.Getenv(Env)
	if admin == "" {
		t.Skip(Env + " is not set: no PostgreSQL to test against")
	}
	cfg, err := pgx.ParseConfig(admin)
	if err != nil {
		t.Fatalf("%s: %v", Env, err)
	}
	db := stdlib.OpenDB(*cfg)
	t.Cleanup(func() { db.Close() })
	b := make([]byte, 6)
	rand.Read(b)
	name := "git_board_test_" + hex.EncodeToString(b)
	if _, err := db.ExecContext(context.Background(), "CREATE DATABASE "+name+" "+strings.Join(with, " ")); err != nil {
		if len(with) > 0 {
			t.Skipf("creating a test database %s: %v", strings.Join(with, " "), err)
		}
		t.Fatalf("creating a test database: %v", err)
	}
	t.Cleanup(func() { drop(db, name) })
	return cfg, name
}

// drop removes the database, closing what still uses it.
func drop(db *sql.DB, name string) {
	_, _ = db.ExecContext(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
}
