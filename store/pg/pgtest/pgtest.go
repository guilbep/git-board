// Package pgtest gives a test a database of its own on the PostgreSQL
// server GIT_BOARD_TEST_POSTGRES names, a DSN whose user may create
// databases; with none set, the test is skipped.
package pgtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Env names the server.
const Env = "GIT_BOARD_TEST_POSTGRES"

// DSN makes a database for t, dropped when t ends, and answers its DSN.
func DSN(t testing.TB) string {
	t.Helper()
	admin := os.Getenv(Env)
	if admin == "" {
		t.Skip(Env + " is not set: no PostgreSQL to test against")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatalf("%s: %v", Env, err)
	}
	b := make([]byte, 6)
	rand.Read(b)
	name := "git_board_next_" + hex.EncodeToString(b)
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		conn.Close(ctx)
		t.Fatalf("creating a test database: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		conn.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		conn.Close(ctx)
	})
	cfg := conn.Config()
	dsn := fmt.Sprintf("host=%s port=%d user=%s dbname=%s sslmode=disable", cfg.Host, cfg.Port, cfg.User, name)
	if cfg.Password != "" {
		dsn += " password=" + cfg.Password
	}
	return dsn
}
