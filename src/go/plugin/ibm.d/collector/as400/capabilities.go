// SPDX-License-Identifier: GPL-3.0-or-later

package as400

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SystemCapabilities tracks what features and tables are available on this IBM i system
type SystemCapabilities struct {
	// Table availability
	tables map[string]bool
	
	// Function availability  
	functions map[string]bool
	
	// System characteristics
	version        string
	model          string
	serialNumber   string
	
	// Feature flags derived from version/capabilities
	hasModernDiskStats    bool
	hasJobInfo           bool
	hasTableFunctions    bool
	hasNetworkStats      bool
	hasDatabaseStats     bool
	
	// Detection complete
	detected bool
}

// QueryVariant represents different query options for different system versions
type QueryVariant struct {
	Name        string   // Human readable name
	Query       string   // SQL query
	MinVersion  string   // Minimum version required (e.g., "V7R1") 
	Tables      []string // Required tables
	Functions   []string // Required functions
	Required    bool     // True if this metric is essential
	Handler     string   // Handler function name
}

// QueryDefinition holds all variants for a specific metric collection
type QueryDefinition struct {
	MetricName string
	Variants   []QueryVariant
	Fallback   *QueryVariant // Ultimate fallback if all variants fail
}

// CapabilityManager handles version detection and query selection
type CapabilityManager struct {
	capabilities *SystemCapabilities
	queries      map[string]*QueryDefinition
	db          *sql.DB
}

// NewCapabilityManager creates a new capability detection and query management system
func NewCapabilityManager(db *sql.DB) *CapabilityManager {
	cm := &CapabilityManager{
		capabilities: &SystemCapabilities{
			tables:    make(map[string]bool),
			functions: make(map[string]bool),
		},
		queries: make(map[string]*QueryDefinition),
		db:      db,
	}
	
	cm.defineQueries()
	return cm
}

// DetectCapabilities performs comprehensive system capability detection
func (cm *CapabilityManager) DetectCapabilities(ctx context.Context) error {
	if cm.capabilities.detected {
		return nil // Already detected
	}
	
	// Use a shorter timeout for the entire detection process
	detectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	
	// 1. Detect system characteristics
	if err := cm.detectSystemInfo(detectCtx); err != nil {
		// Non-fatal - continue with detection
		cm.capabilities.serialNumber = "Unknown"
		cm.capabilities.model = "Unknown"
	}
	
	// 2. Test table availability
	if err := cm.detectTableAvailability(detectCtx); err != nil {
		// Non-fatal - assume basic tables exist
	}
	
	// 3. Test function availability (if possible)
	cm.detectFunctionAvailability(detectCtx) // Non-fatal
	
	// 4. Set feature flags based on detected capabilities
	cm.setFeatureFlags()
	
	cm.capabilities.detected = true
	return nil
}

// detectSystemInfo gathers basic system information for version inference
func (cm *CapabilityManager) detectSystemInfo(ctx context.Context) error {
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	
	// Try to get system info from SYSTEM_STATUS_INFO
	// Using nullable fields to avoid NULL scan errors
	query := `SELECT 
		COALESCE(MACHINE_SERIAL_NUMBER, SERIAL_NUMBER, ''), 
		COALESCE(MACHINE_MODEL, '') 
	FROM QSYS2.SYSTEM_STATUS_INFO 
	FETCH FIRST 1 ROW ONLY`
	
	err := cm.db.QueryRowContext(queryCtx, query).Scan(
		&cm.capabilities.serialNumber,
		&cm.capabilities.model,
	)
	
	if err != nil {
		// Try even simpler query
		query2 := `SELECT 
			COALESCE(MACHINE_TYPE, ''), 
			COALESCE(MACHINE_MODEL, '') 
		FROM QSYS2.SYSTEM_STATUS_INFO 
		FETCH FIRST 1 ROW ONLY`
		
		err2 := cm.db.QueryRowContext(queryCtx, query2).Scan(
			&cm.capabilities.serialNumber,
			&cm.capabilities.model,
		)
		if err2 != nil {
			// Final fallback - just mark as unknown
			cm.capabilities.serialNumber = "Unknown"
			cm.capabilities.model = "Unknown"
			// Don't fail - continue with capability detection
		}
	}
	
	// Infer version based on available features (will be refined by table detection)
	cm.capabilities.version = "Unknown"
	
	return nil
}

// detectTableAvailability tests which monitoring tables exist
func (cm *CapabilityManager) detectTableAvailability(ctx context.Context) error {
	// List of tables to test (in order of importance)
	tables := []string{
		// Core tables (should always exist)
		"SYSTEM_STATUS_INFO",
		"MEMORY_POOL_INFO", 
		"SUBSYSTEM_INFO",
		"MESSAGE_QUEUE_INFO",
		"ASP_INFO",
		
		// Modern tables (V7R1+)
		"JOB_QUEUE_INFO",
		"SYSTEM_STATUS_INFO_BASIC",
		
		// Advanced tables (V7R3+)
		"DISK_STATUS",
		"JOB_INFO",
		
		// Legacy/Alternative tables
		"SYSDISKSTAT",
		"SYSTABLESTAT", 
		"NETSTAT_INTERFACE_INFO",
		"HARDWARE_RESOURCE_INFO",
		"SYSTEM_VALUE_INFO",
	}
	
	for _, table := range tables {
		cm.capabilities.tables[table] = cm.testTableExists(ctx, table)
	}
	
	return nil
}

// testTableExists checks if a specific table exists and is accessible
func (cm *CapabilityManager) testTableExists(ctx context.Context, tableName string) bool {
	queryCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	
	// Try to prepare the statement only - faster than executing
	query := fmt.Sprintf("SELECT 1 FROM QSYS2.%s FETCH FIRST 1 ROW ONLY", tableName)
	
	stmt, err := cm.db.PrepareContext(queryCtx, query)
	if err != nil {
		return false
	}
	stmt.Close()
	return true
}

// detectFunctionAvailability tests table functions (best effort)
func (cm *CapabilityManager) detectFunctionAvailability(ctx context.Context) {
	// Try to detect table functions - these often don't work on older systems
	functions := []string{
		"ACTIVE_JOB_INFO",
		"IFS_OBJECT_STATISTICS",
	}
	
	for _, function := range functions {
		cm.capabilities.functions[function] = cm.testFunctionExists(ctx, function)
	}
}

// testFunctionExists checks if a table function is available
func (cm *CapabilityManager) testFunctionExists(ctx context.Context, functionName string) bool {
	queryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	
	// Try a simple function call
	var query string
	switch functionName {
	case "ACTIVE_JOB_INFO":
		query = "SELECT COUNT(*) FROM TABLE(QSYS2.ACTIVE_JOB_INFO()) FETCH FIRST 1 ROW ONLY"
	case "IFS_OBJECT_STATISTICS":
		query = "SELECT COUNT(*) FROM TABLE(QSYS2.IFS_OBJECT_STATISTICS(START_PATH_NAME => '/')) FETCH FIRST 1 ROW ONLY"
	default:
		return false
	}
	
	var count int
	err := cm.db.QueryRowContext(queryCtx, query).Scan(&count)
	return err == nil
}

// setFeatureFlags derives feature availability from detected capabilities
func (cm *CapabilityManager) setFeatureFlags() {
	caps := cm.capabilities
	
	// Modern disk statistics
	caps.hasModernDiskStats = caps.tables["DISK_STATUS"]
	
	// Job information
	caps.hasJobInfo = caps.tables["JOB_INFO"]
	
	// Table functions
	caps.hasTableFunctions = caps.functions["ACTIVE_JOB_INFO"] || caps.functions["IFS_OBJECT_STATISTICS"]
	
	// Network statistics
	caps.hasNetworkStats = caps.tables["NETSTAT_INTERFACE_INFO"]
	
	// Database statistics
	caps.hasDatabaseStats = caps.tables["SYSTABLESTAT"]
	
	// Infer approximate version based on feature availability
	if caps.hasModernDiskStats && caps.hasJobInfo {
		caps.version = "V7R3+"
	} else if caps.tables["JOB_QUEUE_INFO"] && caps.tables["SYSTEM_STATUS_INFO_BASIC"] {
		caps.version = "V7R1+"
	} else {
		caps.version = "V6R1+"
	}
}

// GetBestQuery returns the best available query variant for a metric
func (cm *CapabilityManager) GetBestQuery(metricName string) (*QueryVariant, error) {
	definition, exists := cm.queries[metricName]
	if !exists {
		return nil, fmt.Errorf("metric %s not defined", metricName)
	}
	
	// Try each variant in order until we find one that's supported
	for _, variant := range definition.Variants {
		if cm.isVariantSupported(&variant) {
			return &variant, nil
		}
	}
	
	// Fall back to ultimate fallback if available
	if definition.Fallback != nil {
		if cm.isVariantSupported(definition.Fallback) {
			return definition.Fallback, nil
		}
	}
	
	if definition.Variants[0].Required {
		return nil, fmt.Errorf("no supported variant found for required metric %s", metricName)
	}
	
	return nil, nil // Optional metric not available
}

// isVariantSupported checks if a query variant can run on this system
func (cm *CapabilityManager) isVariantSupported(variant *QueryVariant) bool {
	// Check required tables
	for _, table := range variant.Tables {
		if !cm.capabilities.tables[table] {
			return false
		}
	}
	
	// Check required functions
	for _, function := range variant.Functions {
		if !cm.capabilities.functions[function] {
			return false
		}
	}
	
	return true
}

// GetCapabilities returns the detected system capabilities
func (cm *CapabilityManager) GetCapabilities() *SystemCapabilities {
	return cm.capabilities
}

// GetSystemInfo returns basic system information
func (cm *CapabilityManager) GetSystemInfo() (version, model, serial string) {
	caps := cm.capabilities
	return caps.version, caps.model, caps.serialNumber
}

// HasTable checks if a specific table is available
func (cm *CapabilityManager) HasTable(tableName string) bool {
	return cm.capabilities.tables[tableName]
}

// HasFunction checks if a specific function is available
func (cm *CapabilityManager) HasFunction(functionName string) bool {
	return cm.capabilities.functions[functionName]
}

// LogCapabilities outputs detected capabilities for debugging
func (cm *CapabilityManager) LogCapabilities() map[string]interface{} {
	caps := cm.capabilities
	
	availableTables := make([]string, 0)
	for table, available := range caps.tables {
		if available {
			availableTables = append(availableTables, table)
		}
	}
	
	availableFunctions := make([]string, 0)
	for function, available := range caps.functions {
		if available {
			availableFunctions = append(availableFunctions, function)
		}
	}
	
	return map[string]interface{}{
		"version":           caps.version,
		"model":            caps.model,
		"serial_number":    caps.serialNumber,
		"available_tables": availableTables,
		"available_functions": availableFunctions,
		"features": map[string]bool{
			"modern_disk_stats": caps.hasModernDiskStats,
			"job_info":         caps.hasJobInfo,
			"table_functions":  caps.hasTableFunctions,
			"network_stats":    caps.hasNetworkStats,
			"database_stats":   caps.hasDatabaseStats,
		},
	}
}