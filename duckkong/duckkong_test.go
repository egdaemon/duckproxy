package duckkong

import (
	"bytes"
	"database/sql"
	"log"
	"path/filepath"
	"testing"

	"github.com/egdaemon/duckproxy/usqlx/drivers"
	"github.com/egdaemon/duckproxy/usqlx/drivers/completer"
	"github.com/stretchr/testify/require"
	"github.com/xo/dburl"
)

func TestCompletion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.duckdb")

	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `CREATE TABLE serve_sessions (id INTEGER); CREATE TABLE lan_peers (id INTEGER)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	registry.Lock()
	registry.open = DefaultOpener
	registry.Unlock()
	register()

	u := &dburl.URL{Driver: driver, DSN: path}
	udb, err := drivers.Open(t.Context(), u, nil, nil)
	require.NoError(t, err)
	defer udb.Close()

	for _, tc := range []struct {
		name, line, expected string
	}{
		{"completes table names", "SELECT * from serve_ses", "sions"},
		{"completes the tables of a meta command", `\d lan_`, "peers"},
		{"completes after a complete table name", "SELECT * FROM serve_sessions", ""},
		{"completes the start of a line", "", ""},
		{"completes after from", "SELECT * FROM ", ""},
		{"completes after where", "SELECT * FROM lan_peers WHERE ", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var errs bytes.Buffer
			c := drivers.NewCompleter(t.Context(), u, udb, nil, completer.WithLogger(log.New(&errs, "", 0)))
			require.NotNil(t, c)

			suggestions, _ := c.Do([]rune(tc.line), len(tc.line))
			require.Empty(t, errs.String(), "the completer must only query what duckdb supports")

			completed := make([]string, 0, len(suggestions))
			for _, s := range suggestions {
				completed = append(completed, string(s))
			}

			if tc.expected != "" {
				require.Contains(t, completed, tc.expected)
			}
		})
	}
}
