// Package hive defines and registers usql's Apache Hive driver.
//
// See: https://github.com/sql-machine-learning/gohive
package hive

import (
	"github.com/xo/dburl"
	"github.com/egdaemon/duckproxy/usqlx/drivers"
	_ "sqlflow.org/gohive" // DRIVER
)

func init() {
	drivers.Register("hive", drivers.Driver{
		ForceParams: func(u *dburl.URL) {
			if u.User != nil && u.Query().Get("auth") == "" {
				drivers.ForceQueryParameters([]string{"auth", "PLAIN"})(u)
			}
		},
	})
}
