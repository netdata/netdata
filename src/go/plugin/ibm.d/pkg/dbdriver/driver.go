// SPDX-License-Identifier: GPL-3.0-or-later

package dbdriver

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
)

// Driver represents a database driver with its capabilities
type Driver struct {
	Name         string
	Description  string
	Available    bool
	RequiresCGO  bool
	RequiresLibs []string // Required system libraries
	DSNFormat    string   // Example DSN format
}

// Registry manages available database drivers
type Registry struct {
	mu      sync.RWMutex
	drivers map[string]*Driver
}

var defaultRegistry = &Registry{
	drivers: make(map[string]*Driver),
}

// DBConnection wraps database connection with driver info
type DBConnection struct {
	*sql.DB
	Driver     string
	DSN        string // Sanitized DSN for logging
	DriverInfo *Driver
}

// Register adds a driver to the registry
func Register(name string, driver *Driver) {
	defaultRegistry.mu.Lock()
	defer defaultRegistry.mu.Unlock()
	defaultRegistry.drivers[name] = driver
}

// GetAvailableDrivers returns list of available drivers
func GetAvailableDrivers() []string {
	defaultRegistry.mu.RLock()
	defer defaultRegistry.mu.RUnlock()

	var available []string
	for name, driver := range defaultRegistry.drivers {
		if driver.Available {
			available = append(available, name)
		}
	}
	return available
}

// IsDriverAvailable checks if a specific driver is available
func IsDriverAvailable(name string) bool {
	defaultRegistry.mu.RLock()
	defer defaultRegistry.mu.RUnlock()

	if driver, exists := defaultRegistry.drivers[name]; exists {
		return driver.Available
	}
	return false
}

// GetDriverInfo returns information about a specific driver
func GetDriverInfo(name string) *Driver {
	defaultRegistry.mu.RLock()
	defer defaultRegistry.mu.RUnlock()

	if driver, exists := defaultRegistry.drivers[name]; exists {
		return driver
	}
	return nil
}

// Connect creates a database connection using the best available driver
func Connect(ctx context.Context, config *ConnectionConfig) (*DBConnection, error) {
	// Determine which driver to use
	driverName, dsn, err := determineDriver(config)
	if err != nil {
		return nil, err
	}

	// Get driver info
	driverInfo := GetDriverInfo(driverName)
	if driverInfo == nil || !driverInfo.Available {
		return nil, fmt.Errorf("driver %s not available", driverName)
	}

	// Open database connection
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database with %s: %w", driverName, err)
	}

	// Configure connection pool
	if config.MaxOpenConns > 0 {
		db.SetMaxOpenConns(config.MaxOpenConns)
	}
	if config.MaxIdleConns > 0 {
		db.SetMaxIdleConns(config.MaxIdleConns)
	}
	if config.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(config.ConnMaxLifetime)
	}

	// Test connection
	pingCtx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database with %s: %w", driverName, err)
	}

	return &DBConnection{
		DB:         db,
		Driver:     driverName,
		DSN:        SanitizeDSN(dsn),
		DriverInfo: driverInfo,
	}, nil
}

// determineDriver selects the best driver based on configuration and availability
func determineDriver(config *ConnectionConfig) (driver, dsn string, err error) {
	available := GetAvailableDrivers()
	if len(available) == 0 {
		return "", "", fmt.Errorf("no database drivers available. Install IBM DB2 client or configure ODBC")
	}

	// If DSN provided, try to auto-detect
	if config.DSN != "" {
		driver, dsn = detectDriverFromDSN(config.DSN)
		if driver != "" && IsDriverAvailable(driver) {
			return driver, dsn, nil
		}
	}

	// Build based on connection type
	switch config.ConnectionType {
	case "db2":
		if !IsDriverAvailable("go_ibm_db") {
			return "", "", fmt.Errorf("IBM DB2 client driver requested but not available. Available drivers: %v", available)
		}
		return "go_ibm_db", BuildDB2DSN(config), nil

	case "odbc":
		if !IsDriverAvailable("odbc") {
			return "", "", fmt.Errorf("ODBC driver requested but not available. Available drivers: %v", available)
		}
		return "odbc", BuildODBCDSN(config), nil

	case "auto", "":
		// Auto-select best available driver
		// Prefer ODBC if configured or for AS/400
		if config.PreferODBC || config.SystemType == "AS400" {
			for _, d := range available {
				if d == "odbc" {
					return "odbc", BuildODBCDSN(config), nil
				}
			}
		}

		// Try IBM DB2 client
		for _, d := range available {
			if d == "go_ibm_db" {
				return "go_ibm_db", BuildDB2DSN(config), nil
			}
		}

		// Use whatever is available
		driver := available[0]
		if driver == "odbc" {
			return driver, BuildODBCDSN(config), nil
		}
		return driver, config.DSN, nil

	default:
		return "", "", fmt.Errorf("unknown connection type: %s", config.ConnectionType)
	}
}

// detectDriverFromDSN attempts to determine the driver from DSN format
func detectDriverFromDSN(dsn string) (driver, cleanDSN string) {
	// Check for DB2 format
	if containsDB2Keywords(dsn) {
		return "go_ibm_db", dsn
	}

	// Check for ODBC format
	if containsODBCKeywords(dsn) {
		return "odbc", dsn
	}

	// Unknown format
	return "", dsn
}
