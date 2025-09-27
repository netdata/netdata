//go:build !cgo

// Package odbcbridge provides an optimized ODBC connection interface using CGO.
package odbcbridge

import (
	"database/sql"
	"database/sql/driver"
	"errors"
)

func init() {
	sql.Register("odbcbridge", &ODBCDriver{})
}

// ODBCDriver implements database/sql/driver.Driver
type ODBCDriver struct{}

// Open returns a new connection to the database
func (d *ODBCDriver) Open(dsn string) (driver.Conn, error) {
	return nil, errors.New("ODBC bridge requires CGO support")
}
