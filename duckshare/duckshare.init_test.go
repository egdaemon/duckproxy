package duckshare_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/egdaemon/duckproxy/duckshare"
	"github.com/stretchr/testify/require"
)

func TestWithInit(t *testing.T) {
	t.Run("restores the initialization when another process takes the file over", func(t *testing.T) {
		sockdir := t.TempDir()
		path := filepath.Join(t.TempDir(), "shared.db")
		attach := fmt.Sprintf("ATTACH IF NOT EXISTS '%s' AS other", filepath.Join(t.TempDir(), "other.db"))

		owner := sql.OpenDB(duckshare.New(sockdir, path, duckshare.WithInit(attach)))
		_, err := owner.ExecContext(t.Context(), `CREATE TABLE other.t (id BIGINT)`)
		require.NoError(t, err)

		// a second connector stands in for another process, it is served by the owner.
		other := sql.OpenDB(duckshare.New(sockdir, path, duckshare.WithInit(attach)))
		defer other.Close()
		_, err = other.ExecContext(t.Context(), `INSERT INTO other.t VALUES (1)`)
		require.NoError(t, err)

		// the owner goes away, the other takes the file over with a new instance.
		require.NoError(t, owner.Close())

		var n int
		require.NoError(t, other.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM other.t`).Scan(&n))
		require.Equal(t, 1, n)
	})
}
