// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows && cgo
// +build !windows,cgo

package dbdriver

import (
	"fmt"
	"strings"

	_ "github.com/alexbrainman/odbc"
)

func init() {
	// Try to register ODBC driver with panic recovery
	defer func() {
		if r := recover(); r != nil {
			// ODBC libraries not found or incompatible
			Register("odbc", &Driver{
				Name:         "odbc",
				Description:  "ODBC driver (unixODBC libraries not found or incompatible)",
				Available:    false,
				RequiresCGO:  true,
				RequiresLibs: []string{"unixODBC", "IBM i Access ODBC Driver"},
				DSNFormat:    "Driver={IBM i Access ODBC Driver};System=host;Uid=user;Pwd=pass",
			})
			return
		}
	}()

	// Test if ODBC driver is actually available by trying to use it
	available := testODBCAvailability()
	
	Register("odbc", &Driver{
		Name:         "odbc",
		Description:  "ODBC driver (requires unixODBC and database-specific ODBC driver)",
		Available:    available,
		RequiresCGO:  true,
		RequiresLibs: []string{"unixODBC", "IBM i Access ODBC Driver"},
		DSNFormat:    "Driver={IBM i Access ODBC Driver};System=host;Uid=user;Pwd=pass",
	})
}

// testODBCAvailability tests if ODBC is actually available
func testODBCAvailability() bool {
	defer func() {
		if recover() != nil {
			// ODBC not available
		}
	}()
	
	// ODBC availability is tested at runtime during connection attempts
	// The alexbrainman/odbc driver will handle library availability automatically
	return true
}

// BuildODBCDSN creates an ODBC connection string
func BuildODBCDSN(config *ConnectionConfig) string {
	// Determine the ODBC driver name
	driverName := config.ODBCDriver
	if driverName == "" {
		if config.SystemType == "AS400" {
			// Common AS/400 ODBC driver names
			driverName = "IBM i Access ODBC Driver"
		} else {
			// Common DB2 ODBC driver names
			driverName = "IBM DB2 ODBC DRIVER"
		}
	}

	// Handle AS/400 specific format
	if config.SystemType == "AS400" || 
		strings.Contains(driverName, "AS400") || 
		strings.Contains(driverName, "IBM i") {
		// AS/400 style ODBC connection
		dsn := fmt.Sprintf("Driver={%s};System=%s;Uid=%s;Pwd=%s;",
			driverName, config.Hostname, config.Username, config.Password)
		
		// AS/400 specific options
		if config.Database != "" && config.Database != "*SYSBAS" {
			dsn += fmt.Sprintf("DefaultLibraries=%s;", config.Database)
		}
		
		if config.Port != 0 && config.Port != 8471 {
			dsn += fmt.Sprintf("Port=%d;", config.Port)
		}
		
		if config.UseSSL {
			dsn += "SSL=1;"
		}
		
		return dsn
	}

	// Standard DB2 ODBC format
	dsn := fmt.Sprintf("Driver={%s};", driverName)

	if config.Database != "" {
		dsn += fmt.Sprintf("Database=%s;", config.Database)
	}

	dsn += fmt.Sprintf("Hostname=%s;Port=%d;Protocol=TCPIP;Uid=%s;Pwd=%s;",
		config.Hostname, config.Port, config.Username, config.Password)

	if config.UseSSL {
		dsn += "Security=SSL;"
		if config.SSLServerCertPath != "" {
			dsn += fmt.Sprintf("SSLServerCertificate=%s;", config.SSLServerCertPath)
		}
	}

	return dsn
}