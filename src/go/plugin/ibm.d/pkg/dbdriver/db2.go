// SPDX-License-Identifier: GPL-3.0-or-later

//go:build disable_ibm_direct_driver
// +build disable_ibm_direct_driver

package dbdriver

import (
	"fmt"

	_ "github.com/ibmdb/go_ibm_db"
)

func init() {
	// Try to register DB2 driver
	// The import will panic if libraries are missing, so we catch it
	defer func() {
		if r := recover(); r != nil {
			// DB2 client libraries not found
			Register("go_ibm_db", &Driver{
				Name:         "go_ibm_db",
				Description:  "IBM DB2 client driver (requires IBM DB2 client libraries)",
				Available:    false,
				RequiresCGO:  true,
				RequiresLibs: []string{"libdb2.so", "libdb2.dll", "libdb2.dylib"},
				DSNFormat:    "DATABASE=db;HOSTNAME=host;PORT=port;PROTOCOL=TCPIP;UID=user;PWD=pass",
			})
		}
	}()

	// If we get here, driver loaded successfully
	Register("go_ibm_db", &Driver{
		Name:         "go_ibm_db",
		Description:  "IBM DB2 client driver",
		Available:    true,
		RequiresCGO:  true,
		RequiresLibs: []string{"libdb2.so", "libdb2.dll", "libdb2.dylib"},
		DSNFormat:    "DATABASE=db;HOSTNAME=host;PORT=port;PROTOCOL=TCPIP;UID=user;PWD=pass",
	})
}

// BuildDB2DSN creates a DB2 connection string
func BuildDB2DSN(config *ConnectionConfig) string {
	// Handle AS/400 specific format if system type is AS400
	if config.SystemType == "AS400" {
		// AS/400 uses different database naming
		database := config.Database
		if database == "" {
			database = "*SYSBAS" // Default AS/400 database
		}
		
		dsn := fmt.Sprintf("DATABASE=%s;HOSTNAME=%s;PORT=%d;PROTOCOL=TCPIP;UID=%s;PWD=%s",
			database, config.Hostname, config.Port, config.Username, config.Password)
		
		if config.UseSSL {
			dsn += ";SECURITY=SSL"
			if config.SSLServerCertPath != "" {
				dsn += fmt.Sprintf(";SSLServerCertificate=%s", config.SSLServerCertPath)
			}
		}
		
		return dsn
	}
	
	// Standard DB2 format
	dsn := fmt.Sprintf("DATABASE=%s;HOSTNAME=%s;PORT=%d;PROTOCOL=TCPIP;UID=%s;PWD=%s",
		config.Database, config.Hostname, config.Port, config.Username, config.Password)

	if config.UseSSL {
		dsn += ";SECURITY=SSL"
		if config.SSLServerCertPath != "" {
			dsn += fmt.Sprintf(";SSLServerCertificate=%s", config.SSLServerCertPath)
		}
	}

	return dsn
}