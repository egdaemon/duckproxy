// Package flightsql defines and registers usql's FlightSQL driver.
//
// See: https://github.com/apache/arrow/tree/main/go/arrow/flight/flightsql/driver
package flightsql

import (
	_ "github.com/apache/arrow-go/v18/arrow/flight/flightsql/driver" // DRIVER
	"github.com/egdaemon/duckproxy/usqlx/drivers"
)

func init() {
	drivers.Register("flightsql", drivers.Driver{})
}
