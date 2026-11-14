# duckproxy

A database/sql driver and server that proxy a DuckDB database over a unix socket, plus a postgres wire frontend.

- `github.com/egdaemon/duckproxy/duckproxy` client driver (`sql.Open("duckproxy", socketPath)`)
- `github.com/egdaemon/duckproxy/duckserve` DuckDB-backed proxy server
- `github.com/egdaemon/duckproxy/duckgres` postgres wire protocol frontend
