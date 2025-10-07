// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows && cgo
// +build !windows,cgo

package odbcwrapper

/*
// Force use of system unixODBC headers, not IBM headers
#cgo CFLAGS: -I/usr/include -DSQL_NOUNICODEMAP
#cgo LDFLAGS: -lodbc

// Include system unixODBC headers explicitly
#include "/usr/include/sql.h"
#include "/usr/include/sqlext.h"
#include <stdlib.h>
#include <string.h>

// Simple ODBC wrapper functions
SQLRETURN simple_alloc_env(SQLHENV *env) {
    return SQLAllocHandle(SQL_HANDLE_ENV, SQL_NULL_HANDLE, env);
}

SQLRETURN simple_set_version(SQLHENV env) {
    return SQLSetEnvAttr(env, SQL_ATTR_ODBC_VERSION, (SQLPOINTER)SQL_OV_ODBC3, 0);
}

SQLRETURN simple_data_sources(SQLHENV env, SQLUSMALLINT direction, char *dsn, SQLSMALLINT dsn_len, SQLSMALLINT *dsn_ret, char *desc, SQLSMALLINT desc_len, SQLSMALLINT *desc_ret) {
    return SQLDataSources(env, direction, (SQLCHAR*)dsn, dsn_len, dsn_ret, (SQLCHAR*)desc, desc_len, desc_ret);
}

void simple_free_env(SQLHENV env) {
    SQLFreeHandle(SQL_HANDLE_ENV, env);
}
*/
import "C"

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"unsafe"
)

// Simple ODBC driver implementation
type odbcDriver struct{}

func (d *odbcDriver) Open(dsn string) (driver.Conn, error) {
	return nil, fmt.Errorf("ODBC connections via DSN: %s (custom wrapper not implemented - use github.com/alexbrainman/odbc)", dsn)
}

func init() {
	sql.Register("odbc", &odbcDriver{})
}

// Drivers returns available ODBC data sources
func Drivers() ([]string, error) {
	var env C.SQLHENV

	// Allocate environment handle
	if ret := C.simple_alloc_env(&env); ret != C.SQL_SUCCESS {
		return nil, fmt.Errorf("failed to allocate environment handle")
	}
	defer C.simple_free_env(env)

	// Set ODBC version
	if ret := C.simple_set_version(env); ret != C.SQL_SUCCESS {
		return nil, fmt.Errorf("failed to set ODBC version")
	}

	var drivers []string
	direction := C.SQL_FETCH_FIRST

	for {
		var dsn [256]C.char
		var dsnLen C.SQLSMALLINT
		var desc [256]C.char
		var descLen C.SQLSMALLINT

		ret := C.simple_data_sources(env, C.SQLUSMALLINT(direction),
			&dsn[0], C.SQLSMALLINT(len(dsn)), &dsnLen,
			&desc[0], C.SQLSMALLINT(len(desc)), &descLen)

		if ret == C.SQL_NO_DATA {
			break
		}

		if ret != C.SQL_SUCCESS {
			break
		}

		driverName := C.GoString((*C.char)(unsafe.Pointer(&dsn[0])))
		drivers = append(drivers, driverName)
		direction = C.SQL_FETCH_NEXT
	}

	return drivers, nil
}

// Open wraps sql.Open for ODBC connections
func Open(dsn string) (*sql.DB, error) {
	return sql.Open("odbc", dsn)
}
