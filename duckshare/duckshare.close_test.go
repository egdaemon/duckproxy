package duckshare_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egdaemon/duckproxy/duckshare"
	"github.com/gofrs/flock"
	"github.com/stretchr/testify/require"
)

func TestClose(t *testing.T) {
	t.Run("holds the connect lock until the file is released", func(t *testing.T) {
		sockdir := t.TempDir()
		path := filepath.Join(t.TempDir(), "shared.db")

		owner := sql.OpenDB(duckshare.New(sockdir, path))
		_, err := owner.ExecContext(t.Context(), `SELECT 1`)
		require.NoError(t, err)

		sockets, err := filepath.Glob(filepath.Join(sockdir, "*.socket"))
		require.NoError(t, err)
		require.Len(t, sockets, 1)

		// a process connecting holds the lock, closing must wait for it instead of tearing the socket down.
		lock := flock.New(path + ".lock")
		locked, err := lock.TryLock()
		require.NoError(t, err)
		require.True(t, locked)
		defer lock.Unlock()

		closed := make(chan error, 1)
		go func() { closed <- owner.Close() }()

		require.Never(t, func() bool { return len(closed) > 0 }, 300*time.Millisecond, 10*time.Millisecond, "closed while another process held the connect lock")
		_, err = os.Stat(sockets[0])
		require.NoError(t, err, "the socket must remain until the lock is acquired")

		require.NoError(t, lock.Unlock())
		select {
		case err := <-closed:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("close did not finish after the lock was released")
		}

		_, err = os.Stat(sockets[0])
		require.ErrorIs(t, err, os.ErrNotExist)
	})
}
