package duckshare_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/egdaemon/duckproxy/duckshare"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestConnector(t *testing.T) {
	t.Run("serves the database over postgres while open", func(t *testing.T) {
		sockdir := t.TempDir()
		pgdir := t.TempDir()
		pgsocket := filepath.Join(pgdir, ".s.PGSQL.5432")

		db := sql.OpenDB(duckshare.New(sockdir, filepath.Join(t.TempDir(), "shared.db"), duckshare.WithPostgresSocket(pgsocket)))
		_, err := db.ExecContext(t.Context(), `CREATE TABLE t (id VARCHAR, n BIGINT)`)
		require.NoError(t, err)
		_, err = db.ExecContext(t.Context(), `INSERT INTO t VALUES ($1, $2)`, "a", 2299)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			_, err := os.Stat(pgsocket)
			return err == nil
		}, 5*time.Second, 10*time.Millisecond, "the postgres socket must exist while the database is open")

		cfg, err := pgx.ParseConfig(fmt.Sprintf("host=%s port=5432 sslmode=disable", pgdir))
		require.NoError(t, err)
		conn, err := pgx.ConnectConfig(t.Context(), cfg)
		require.NoError(t, err)

		var n int64
		require.NoError(t, conn.QueryRow(t.Context(), `SELECT n FROM t WHERE id = $1`, "a").Scan(&n))
		require.Equal(t, int64(2299), n)
		require.NoError(t, conn.Close(t.Context()))

		require.NoError(t, db.Close())
		_, err = os.Stat(pgsocket)
		require.ErrorIs(t, err, os.ErrNotExist, "the postgres socket must be removed when the database closes")
	})

	t.Run("processes share the file", func(t *testing.T) {
		sockdir := t.TempDir()
		path := filepath.Join(t.TempDir(), "shared.db")

		owner := sql.OpenDB(duckshare.New(sockdir, path))
		defer owner.Close()
		_, err := owner.ExecContext(t.Context(), `CREATE TABLE t (id VARCHAR)`)
		require.NoError(t, err)

		sharer := sql.OpenDB(duckshare.New(sockdir, path))
		defer sharer.Close()

		_, err = owner.ExecContext(t.Context(), `INSERT INTO t VALUES ($1)`, "a")
		require.NoError(t, err)
		_, err = sharer.ExecContext(t.Context(), `INSERT INTO t VALUES ($1)`, "b")
		require.NoError(t, err)

		for _, db := range []*sql.DB{owner, sharer} {
			var n int
			require.NoError(t, db.QueryRowContext(t.Context(), `SELECT count(*) FROM t`).Scan(&n))
			require.Equal(t, 2, n)
		}
	})

	t.Run("recovers when the owner goes away", func(t *testing.T) {
		sockdir := t.TempDir()
		path := filepath.Join(t.TempDir(), "shared.db")

		owner := sql.OpenDB(duckshare.New(sockdir, path))
		_, err := owner.ExecContext(t.Context(), `CREATE TABLE t (id VARCHAR)`)
		require.NoError(t, err)

		sharer := sql.OpenDB(duckshare.New(sockdir, path))
		defer sharer.Close()

		_, err = owner.ExecContext(t.Context(), `INSERT INTO t VALUES ($1)`, "a")
		require.NoError(t, err)
		// the sharer now holds a connection through the owner.
		var n int
		require.NoError(t, sharer.QueryRowContext(t.Context(), `SELECT count(*) FROM t`).Scan(&n))

		require.NoError(t, owner.Close())

		require.NoError(t, sharer.QueryRowContext(t.Context(), `SELECT count(*) FROM t`).Scan(&n))
		require.Equal(t, 1, n)

		// the sharer owns the file now so the next process goes through it.
		next := sql.OpenDB(duckshare.New(sockdir, path))
		defer next.Close()
		_, err = next.ExecContext(t.Context(), `INSERT INTO t VALUES ($1)`, "b")
		require.NoError(t, err)
		require.NoError(t, sharer.QueryRowContext(t.Context(), `SELECT count(*) FROM t`).Scan(&n))
		require.Equal(t, 2, n)
	})

	t.Run("concurrent opens all succeed", func(t *testing.T) {
		sockdir := t.TempDir()
		path := filepath.Join(t.TempDir(), "shared.db")

		setup := sql.OpenDB(duckshare.New(sockdir, path))
		defer setup.Close()
		_, err := setup.ExecContext(t.Context(), `CREATE TABLE t (id VARCHAR)`)
		require.NoError(t, err)

		const n = 8
		dbs := make([]*sql.DB, n)
		errs := make([]error, n)

		var wg sync.WaitGroup
		for i := range n {
			wg.Go(func() {
				dbs[i] = sql.OpenDB(duckshare.New(sockdir, path))
				_, errs[i] = dbs[i].ExecContext(t.Context(), `INSERT INTO t VALUES ($1)`, string(rune('a'+i)))
			})
		}
		wg.Wait()

		for i := range n {
			require.NoError(t, errs[i])
			defer dbs[i].Close()
		}

		var got int
		require.NoError(t, setup.QueryRowContext(t.Context(), `SELECT count(*) FROM t`).Scan(&got))
		require.Equal(t, n, got)
	})

	t.Run("replaces a stale socket", func(t *testing.T) {
		sockdir := t.TempDir()
		path := filepath.Join(t.TempDir(), "shared.db")

		first := sql.OpenDB(duckshare.New(sockdir, path))
		require.NoError(t, first.PingContext(t.Context()))

		entries, err := os.ReadDir(sockdir)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		socket := filepath.Join(sockdir, entries[0].Name())

		require.NoError(t, first.Close())
		_, err = os.Stat(socket)
		require.ErrorIs(t, err, os.ErrNotExist, "the socket is removed when the owner closes")

		// a crash leaves the socket behind.
		require.NoError(t, os.WriteFile(socket, nil, 0600))
		db := sql.OpenDB(duckshare.New(sockdir, path))
		require.NoError(t, db.PingContext(t.Context()))
		require.NoError(t, db.Close())
	})

	t.Run("gives up waiting when the context ends", func(t *testing.T) {
		ctx, done := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer done()
		<-ctx.Done()

		db := sql.OpenDB(duckshare.New(t.TempDir(), filepath.Join(t.TempDir(), "shared.db")))
		defer db.Close()
		require.Error(t, db.PingContext(ctx))
	})

	t.Run("lock path override is honoured", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "shared.db")
		lock := path + ".lock"

		db := sql.OpenDB(duckshare.New(t.TempDir(), path, duckshare.WithLockPath(lock)))
		defer db.Close()
		require.NoError(t, db.PingContext(t.Context()))

		_, err := os.Stat(lock)
		require.NoError(t, err)
		_, err = os.Stat(path + ".duckproxy.lock")
		require.ErrorIs(t, err, os.ErrNotExist)
	})
}
