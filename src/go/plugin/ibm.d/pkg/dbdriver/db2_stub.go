// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !disable_ibm_direct_driver
// +build !disable_ibm_direct_driver

package dbdriver

// No imports needed for this stub

func init() {
	// Register IBM DB2 driver as not available (using ODBC instead)
	Register("go_ibm_db", &Driver{
		Name:         "go_ibm_db",
		Description:  "IBM DB2 client driver (disabled - using ODBC instead)",
		Available:    false,
		RequiresCGO:  true,
		RequiresLibs: []string{"IBM DB2 client libraries (not installed)"},
		DSNFormat:    "DATABASE=db;HOSTNAME=host;PORT=port;PROTOCOL=TCPIP;UID=user;PWD=pass",
	})
}

// BuildDB2DSN creates a DB2 connection string - redirects to ODBC format
func BuildDB2DSN(config *ConnectionConfig) string {
	// Convert to ODBC format instead of IBM DB2 format
	return BuildODBCDSN(config)
}
