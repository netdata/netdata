// SPDX-License-Identifier: GPL-3.0-or-later

package dbdriver

import "time"

// ConnectionConfig holds common connection parameters for database connections
type ConnectionConfig struct {
	// Connection type selection
	ConnectionType string // "auto", "db2", "odbc"
	PreferODBC     bool   // Prefer ODBC when auto-detecting
	SystemType     string // "AS400", "DB2", etc.

	// Universal DSN (if provided directly)
	DSN string

	// Component-based connection
	Database string
	Hostname string
	Port     int
	Username string
	Password string

	// SSL/TLS
	UseSSL            bool
	SSLCertPath       string
	SSLServerCertPath string

	// ODBC specific
	ODBCDriver string // Driver name for ODBC (e.g., "IBM i Access ODBC Driver")

	// Connection pool settings
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration

	// Context for operations
	Timeout time.Duration
}

// SetDefaults sets default values for connection configuration
func (c *ConnectionConfig) SetDefaults() {
	if c.Timeout == 0 {
		c.Timeout = 30 * time.Second
	}

	if c.MaxOpenConns == 0 {
		c.MaxOpenConns = 1
	}

	if c.ConnMaxLifetime == 0 {
		c.ConnMaxLifetime = 10 * time.Minute
	}

	// Set default ports based on system type
	if c.Port == 0 {
		switch c.SystemType {
		case "AS400":
			c.Port = 8471 // Default AS/400 DRDA port
		default:
			c.Port = 50000 // Default DB2 port
		}
	}

	// Set default database based on system type
	if c.Database == "" && c.SystemType == "AS400" {
		c.Database = "*SYSBAS"
	}
}
