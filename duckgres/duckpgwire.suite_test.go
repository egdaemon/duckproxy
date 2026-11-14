// Package duckgres_test holds shared helpers for the black-box tests
// exercising the public sql.Open/pgx-client surface, as opposed to the
// internal package's tests of unexported functions.
package duckgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/egdaemon/duckproxy/duckgres"
	"github.com/egdaemon/duckproxy/duckserve"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

// startTestServer wires up the two-hop topology duckgres runs on: a real
// duckproxy.Server backed by a temp-file DuckDB, and a duckgres.Server in
// front of it whose db is sql.Open("duckproxy", ...) pointed at that
// server's socket -- not a direct duckdb connection. It returns the
// duckgres socket's directory and duckproxy's underlying *sql.DB (for
// inspecting real DuckDB connection pool stats, e.g. in the concurrency
// test).
func startTestServer(t *testing.T, opts ...duckgres.Option) (dir string, duckdbDB *sql.DB) {
	t.Helper()

	dir = t.TempDir()
	dbPath := filepath.Join(dir, "test.duckdb")
	duckproxySocketPath := filepath.Join(dir, "duckproxy.sock")
	pgSocketPath := filepath.Join(dir, ".s.PGSQL.5432")

	duckdbDB, err := sql.Open("duckdb", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { duckdbDB.Close() })

	ctx, cancel := context.WithCancel(context.Background())

	duckproxySrv := duckserve.New(duckdbDB)
	duckproxyErrCh := make(chan error, 1)
	go func() { duckproxyErrCh <- duckserve.ListenUnix(ctx, duckproxySocketPath, duckproxySrv) }()
	waitForSocket(t, duckproxySocketPath)

	duckproxyClientDB, err := sql.Open("duckproxy", duckproxySocketPath)
	require.NoError(t, err)
	t.Cleanup(func() { duckproxyClientDB.Close() })

	srv := duckgres.New(duckproxyClientDB, opts...)
	pgErrCh := make(chan error, 1)
	go func() { pgErrCh <- duckgres.ListenUnix(ctx, pgSocketPath, srv) }()
	waitForSocket(t, pgSocketPath)

	t.Cleanup(func() {
		cancel()
		<-pgErrCh
		<-duckproxyErrCh
	})

	return dir, duckdbDB
}

func waitForSocket(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("socket %s did not appear in time", path)
}

func connect(t *testing.T, dir string, simple bool) *pgx.Conn {
	t.Helper()

	cfg, err := pgx.ParseConfig(fmt.Sprintf("host=%s port=5432 database=test sslmode=disable", dir))
	require.NoError(t, err)
	if simple {
		cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	}

	conn, err := pgx.ConnectConfig(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close(context.Background()) })
	return conn
}
