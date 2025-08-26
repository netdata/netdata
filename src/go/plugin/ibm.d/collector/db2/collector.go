// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package db2

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"

	// Import database drivers
	_ "github.com/netdata/netdata/go/plugins/plugin/ibm.d/pkg/dbdriver"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("db2", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *DB2 {
	return &DB2{
		Config: Config{
			DSN:           "",
			Timeout:       confopt.Duration(time.Second * 2),
			UpdateEvery:   5,
			MaxDbConns:    1,
			MaxDbLifeTime: confopt.Duration(time.Minute * 10),

			// Instance collection defaults (will be set in Init based on version)
			// CollectDatabaseMetrics:   nil,
			// CollectBufferpoolMetrics: nil,
			// CollectTablespaceMetrics: nil,
			// CollectConnectionMetrics: nil,
			// CollectLockMetrics:       nil,
			// CollectTableMetrics:      nil,
			// CollectIndexMetrics:      nil,

			// Cardinality limits
			MaxDatabases:   10,
			MaxBufferpools: 20,
			MaxTablespaces: 100,
			MaxConnections: 200,
			MaxTables:      50,
			MaxIndexes:     100,

			// Backup history
			BackupHistoryDays: 30,

			// New performance monitoring defaults
			CollectMemoryMetrics:  true,
			CollectWaitMetrics:    true,
			CollectTableIOMetrics: true,
		},

		charts:      baseCharts.Copy(),
		once:        &sync.Once{},
		mx:          &metricsData{},
		databases:   make(map[string]*databaseMetrics),
		bufferpools: make(map[string]*bufferpoolMetrics),
		tablespaces: make(map[string]*tablespaceMetrics),
		connections: make(map[string]*connectionMetrics),
		tables:      make(map[string]*tableMetrics),
		indexes:     make(map[string]*indexMetrics),
		memoryPools: make(map[string]*memoryPoolMetrics),
		memorySets:  make(map[string]*memorySetInstanceMetrics),
		prefetchers: make(map[string]*prefetcherInstanceMetrics),

		// Initialize resilience tracking
		disabledMetrics:  make(map[string]bool),
		disabledFeatures: make(map[string]bool),
	}
}

type Config struct {
	Vnode         string           `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery   int              `yaml:"update_every,omitempty" json:"update_every"`
	DSN           string           `yaml:"dsn" json:"dsn"`
	Timeout       confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	MaxDbConns    int              `yaml:"max_db_conns,omitempty" json:"max_db_conns"`
	MaxDbLifeTime confopt.Duration `yaml:"max_db_life_time,omitempty" json:"max_db_life_time"`

	// Instance collection settings
	CollectDatabaseMetrics   *bool `yaml:"collect_database_metrics,omitempty" json:"collect_database_metrics"`
	CollectBufferpoolMetrics *bool `yaml:"collect_bufferpool_metrics,omitempty" json:"collect_bufferpool_metrics"`
	CollectTablespaceMetrics *bool `yaml:"collect_tablespace_metrics,omitempty" json:"collect_tablespace_metrics"`
	CollectConnectionMetrics *bool `yaml:"collect_connection_metrics,omitempty" json:"collect_connection_metrics"`
	CollectLockMetrics       *bool `yaml:"collect_lock_metrics,omitempty" json:"collect_lock_metrics"`
	CollectTableMetrics      *bool `yaml:"collect_table_metrics,omitempty" json:"collect_table_metrics"`
	CollectIndexMetrics      *bool `yaml:"collect_index_metrics,omitempty" json:"collect_index_metrics"`

	// Cardinality limits
	MaxDatabases   int `yaml:"max_databases,omitempty" json:"max_databases"`
	MaxBufferpools int `yaml:"max_bufferpools,omitempty" json:"max_bufferpools"`
	MaxTablespaces int `yaml:"max_tablespaces,omitempty" json:"max_tablespaces"`
	MaxConnections int `yaml:"max_connections,omitempty" json:"max_connections"`
	MaxTables      int `yaml:"max_tables,omitempty" json:"max_tables"`
	MaxIndexes     int `yaml:"max_indexes,omitempty" json:"max_indexes"`

	// Backup history
	BackupHistoryDays int `yaml:"backup_history_days,omitempty" json:"backup_history_days"`

	// Performance monitoring
	CollectMemoryMetrics  bool `yaml:"collect_memory_metrics,omitempty" json:"collect_memory_metrics"`
	CollectWaitMetrics    bool `yaml:"collect_wait_metrics,omitempty" json:"collect_wait_metrics"`
	CollectTableIOMetrics bool `yaml:"collect_table_io_metrics,omitempty" json:"collect_table_io_metrics"`

	// Selectors for filtering
	CollectDatabasesMatching   string `yaml:"collect_databases_matching,omitempty" json:"collect_databases_matching"`
	CollectBufferpoolsMatching string `yaml:"collect_bufferpools_matching,omitempty" json:"collect_bufferpools_matching"`
	CollectTablespacesMatching string `yaml:"collect_tablespaces_matching,omitempty" json:"collect_tablespaces_matching"`
	CollectConnectionsMatching string `yaml:"collect_connections_matching,omitempty" json:"collect_connections_matching"`
	CollectTablesMatching      string `yaml:"collect_tables_matching,omitempty" json:"collect_tables_matching"`
	CollectIndexesMatching     string `yaml:"collect_indexes_matching,omitempty" json:"collect_indexes_matching"`
}

type DB2 struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	db   *sql.DB
	once *sync.Once
	mx   *metricsData

	// Instance tracking
	databases   map[string]*databaseMetrics
	bufferpools map[string]*bufferpoolMetrics
	tablespaces map[string]*tablespaceMetrics
	connections map[string]*connectionMetrics
	tables      map[string]*tableMetrics
	indexes     map[string]*indexMetrics
	memoryPools map[string]*memoryPoolMetrics

	// Selectors
	databaseSelector   matcher.Matcher
	bufferpoolSelector matcher.Matcher
	tablespaceSelector matcher.Matcher
	connectionSelector matcher.Matcher
	tableSelector      matcher.Matcher
	indexSelector      matcher.Matcher

	// DB2 version info
	version      string
	edition      string // LUW, z/OS, i, Cloud
	versionMajor int    // Parsed major version (e.g., 11 from "11.5.7")
	versionMinor int    // Parsed minor version (e.g., 5 from "11.5.7")
	serverInfo   serverInfo

	// Filtering mode (persistent state)
	databaseFilterMode   bool
	bufferpoolFilterMode bool
	tablespaceFilterMode bool
	connectionFilterMode bool
	tableFilterMode      bool
	indexFilterMode      bool

	// Resilience tracking (following AS/400 pattern)
	disabledMetrics  map[string]bool // Track disabled metrics due to version incompatibility
	disabledFeatures map[string]bool // Track disabled features (tables, views, functions)

	// Edition flags (for labels only)
	isDB2ForAS400 bool // DB2 for i (AS/400)
	isDB2ForZOS   bool // DB2 for z/OS
	isDB2Cloud    bool // Db2 on Cloud

	// Memory set instance tracking (Screen 26)
	memorySets               map[string]*memorySetInstanceMetrics
	currentMemorySetHostName string
	currentMemorySetDBName   string
	currentMemorySetType     string
	currentMemorySetMember   int64

	// Prefetcher instance tracking (Screen 15)
	prefetchers           map[string]*prefetcherInstanceMetrics
	currentBufferPoolName string
}

type serverInfo struct {
	instanceName string
	hostName     string
	version      string
	platform     string
}

func (d *DB2) Configuration() any {
	return d.Config
}

func (d *DB2) Init(ctx context.Context) error {
	if d.DSN == "" {
		return errors.New("dsn required but not set")
	}

	// Initialize selectors
	if d.CollectDatabasesMatching != "" {
		m, err := matcher.NewSimplePatternsMatcher(d.CollectDatabasesMatching)
		if err != nil {
			return fmt.Errorf("invalid database selector pattern '%s': %v", d.CollectDatabasesMatching, err)
		}
		d.databaseSelector = m
		d.Infof("database selector configured: pattern='%s'", d.CollectDatabasesMatching)
	}

	if d.CollectBufferpoolsMatching != "" {
		m, err := matcher.NewSimplePatternsMatcher(d.CollectBufferpoolsMatching)
		if err != nil {
			return fmt.Errorf("invalid bufferpool selector pattern '%s': %v", d.CollectBufferpoolsMatching, err)
		}
		d.bufferpoolSelector = m
		d.Infof("bufferpool selector configured: pattern='%s'", d.CollectBufferpoolsMatching)
	}

	if d.CollectTablespacesMatching != "" {
		m, err := matcher.NewSimplePatternsMatcher(d.CollectTablespacesMatching)
		if err != nil {
			return fmt.Errorf("invalid tablespace selector pattern '%s': %v", d.CollectTablespacesMatching, err)
		}
		d.tablespaceSelector = m
		d.Infof("tablespace selector configured: pattern='%s'", d.CollectTablespacesMatching)
	}

	if d.CollectConnectionsMatching != "" {
		m, err := matcher.NewSimplePatternsMatcher(d.CollectConnectionsMatching)
		if err != nil {
			return fmt.Errorf("invalid connection selector pattern '%s': %v", d.CollectConnectionsMatching, err)
		}
		d.connectionSelector = m
	}

	if d.CollectTablesMatching != "" {
		m, err := matcher.NewSimplePatternsMatcher(d.CollectTablesMatching)
		if err != nil {
			return fmt.Errorf("invalid table selector pattern '%s': %v", d.CollectTablesMatching, err)
		}
		d.tableSelector = m
	}

	if d.CollectIndexesMatching != "" {
		m, err := matcher.NewSimplePatternsMatcher(d.CollectIndexesMatching)
		if err != nil {
			return fmt.Errorf("invalid index selector pattern '%s': %v", d.CollectIndexesMatching, err)
		}
		d.indexSelector = m
	}

	return nil
}

func (d *DB2) Check(ctx context.Context) error {
	mx, err := d.collect(ctx)
	if err != nil {
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (d *DB2) Charts() *module.Charts {
	return d.charts
}

func (d *DB2) Collect(ctx context.Context) map[string]int64 {
	mx, err := d.collect(ctx)
	if err != nil {
		d.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (d *DB2) Cleanup(context.Context) {
	if d.db == nil {
		return
	}
	if err := d.db.Close(); err != nil {
		d.Errorf("cleanup: error closing database: %v", err)
	}
	d.db = nil
}

func (d *DB2) verifyConfig() error {
	if d.DSN == "" {
		return errors.New("DSN is required but not set")
	}
	return nil
}

func (d *DB2) initDatabase(ctx context.Context) (*sql.DB, error) {
	d.Infof("connecting to DB2 with DSN: %s", safeDSN(d.DSN))

	var db *sql.DB
	var err error

	// Parse the existing DSN or use it as-is if it's already ODBC format
	if strings.Contains(d.DSN, "Driver=") {
		// Already ODBC format - use optimized ODBC bridge
		db, err = sql.Open("odbcbridge", d.DSN)
		if err != nil {
			return nil, fmt.Errorf("error opening ODBC database connection: %v", err)
		}
		d.Infof("connected to DB2 using optimized ODBC bridge driver")
	} else {
		// Convert existing DB2 DSN to ODBC format
		// For now, just try to use ODBC with the DSN as-is
		// In practice, the DSN would need to be converted from DB2 format to ODBC format
		odbcDSN := convertDB2DSNToODBC(d.DSN)

		db, err = sql.Open("odbcbridge", odbcDSN)
		if err != nil {
			return nil, fmt.Errorf("error opening ODBC bridge connection with converted DSN: %v", err)
		}

		d.Infof("connected to DB2 using optimized ODBC bridge with converted DSN")
	}

	db.SetMaxOpenConns(d.MaxDbConns)
	db.SetConnMaxLifetime(time.Duration(d.MaxDbLifeTime))

	pingCtx, cancel := context.WithTimeout(ctx, time.Duration(d.Timeout))
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("error pinging database: %v", err)
	}

	d.Infof("successfully connected to DB2")
	return db, nil
}

func (d *DB2) ping(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, time.Duration(d.Timeout))
	defer cancel()

	return d.db.PingContext(pingCtx)
}

func cleanName(name string) string {
	r := strings.NewReplacer(
		" ", "_",
		".", "_",
		"-", "_",
		"/", "_",
		":", "_",
		"=", "_",
	)
	return strings.ToLower(r.Replace(name))
}

// safeDSN masks sensitive information in DSN for logging
func safeDSN(dsn string) string {
	if dsn == "" {
		return "<empty>"
	}

	// Parse DSN and mask sensitive parts
	masked := dsn

	// Mask password
	if strings.Contains(masked, "PWD=") {
		// Find PWD= and mask until next ; or end of string
		start := strings.Index(masked, "PWD=")
		if start != -1 {
			end := strings.Index(masked[start:], ";")
			if end == -1 {
				// PWD is at the end
				masked = masked[:start] + "PWD=***"
			} else {
				// PWD is in the middle
				masked = masked[:start] + "PWD=***" + masked[start+end:]
			}
		}
	}

	// Mask password in lowercase
	if strings.Contains(masked, "pwd=") {
		start := strings.Index(masked, "pwd=")
		if start != -1 {
			end := strings.Index(masked[start:], ";")
			if end == -1 {
				masked = masked[:start] + "pwd=***"
			} else {
				masked = masked[:start] + "pwd=***" + masked[start+end:]
			}
		}
	}

	// Mask AUTHENTICATION fields if present
	if strings.Contains(masked, "AUTHENTICATION=") {
		start := strings.Index(masked, "AUTHENTICATION=")
		if start != -1 {
			end := strings.Index(masked[start:], ";")
			if end == -1 {
				masked = masked[:start] + "AUTHENTICATION=***"
			} else {
				masked = masked[:start] + "AUTHENTICATION=***" + masked[start+end:]
			}
		}
	}

	return masked
}

// convertDB2DSNToODBC converts a DB2 DSN format to ODBC format
func convertDB2DSNToODBC(db2DSN string) string {
	// For this test, assume it's already close to ODBC format
	// Add IBM DB2 ODBC driver if not present
	if !strings.Contains(db2DSN, "Driver=") {
		return "Driver={IBM DB2 ODBC DRIVER};" + db2DSN
	}
	return db2DSN
}

// Resilience functions following AS/400 pattern

// logOnce logs a warning message only once per key to avoid spam
func (d *DB2) logOnce(key string, format string, args ...interface{}) {
	// Check if already logged using either map
	if d.disabledMetrics[key] || d.disabledFeatures[key] {
		return // Already logged
	}

	// Log the message
	d.Warningf(format, args...)

	// Mark as logged to prevent future logs
	d.disabledMetrics[key] = true
}

// isDisabled checks if a metric or feature is disabled due to version incompatibility
func (d *DB2) isDisabled(key string) bool {
	return d.disabledMetrics[key] || d.disabledFeatures[key]
}

// isSQLFeatureError detects SQL errors that indicate missing features (tables, columns, functions)
func isSQLFeatureError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToUpper(err.Error())
	// Common DB2 error codes for missing objects
	return strings.Contains(errStr, "SQL0204N") || // object not found (table, view, function)
		strings.Contains(errStr, "SQL0206N") || // column not found
		strings.Contains(errStr, "SQL0443N") || // function not found
		strings.Contains(errStr, "SQL0551N") || // authorization error (often means table doesn't exist)
		strings.Contains(errStr, "SQL0707N") || // name too long (version compatibility)
		strings.Contains(errStr, "SQLCODE=-204") || // Alternative format
		strings.Contains(errStr, "SQLCODE=-206") ||
		strings.Contains(errStr, "SQLCODE=-443") ||
		strings.Contains(errStr, "SQLCODE=-551") ||
		strings.Contains(errStr, "UNSUPPORTED COLUMN TYPE") // ODBC driver limitation
}

// collectSingleMetric executes a single-value query and handles version-specific errors gracefully
func (d *DB2) collectSingleMetric(ctx context.Context, metricKey string, query string, handler func(value string)) error {
	if d.isDisabled(metricKey) {
		return nil
	}

	var nullValue sql.NullString
	queryCtx, cancel := context.WithTimeout(ctx, time.Duration(d.Timeout))
	defer cancel()

	err := d.db.QueryRowContext(queryCtx, query).Scan(&nullValue)
	if err != nil {
		if isSQLFeatureError(err) {
			d.logOnce(metricKey, "metric %s not available on this DB2 edition/version: %v", metricKey, err)
			d.disabledMetrics[metricKey] = true
			return nil // Not a fatal error
		}
		return err
	}

	if nullValue.Valid && nullValue.String != "" {
		handler(nullValue.String)
	}

	return nil
}

// detectDB2Edition tries to detect the DB2 edition and version for compatibility
func (d *DB2) detectDB2Edition(ctx context.Context) error {
	// Try to detect LUW first (most common)
	err := d.collectSingleMetric(ctx, "version_detection_luw", queryDetectVersionLUW, func(value string) {
		d.edition = "LUW"
		d.version = value
		d.parseDB2Version()
		d.Debugf("detected DB2 LUW edition, version: %s", value)
	})

	if err == nil && d.edition != "" {
		d.logVersionInformation()
		d.addVersionLabelsToCharts()
		return nil
	}

	// Try to detect DB2 for i (AS/400)
	err = d.collectSingleMetric(ctx, "version_detection_i", queryDetectVersionI, func(value string) {
		d.edition = "i"
		d.version = "DB2 for i"
		d.Debugf("detected DB2 for i (AS/400) edition")
	})

	if err == nil && d.edition != "" {
		d.logVersionInformation()
		d.addVersionLabelsToCharts()
		return nil
	}

	// Try to detect z/OS by checking for z/OS specific tables
	err = d.collectSingleMetric(ctx, "version_detection_zos", queryDetectVersionZOS, func(value string) {
		d.edition = "z/OS"
		d.version = value
		d.parseDB2Version()
		d.Debugf("detected DB2 for z/OS edition")
	})

	if err == nil && d.edition != "" {
		d.logVersionInformation()
		d.addVersionLabelsToCharts()
		return nil
	}

	// Cloud detection removed - Cloud queries had static values

	// Default to LUW if all detection fails
	d.edition = "LUW"
	d.version = "Unknown"
	d.Warningf("could not detect DB2 edition, defaulting to LUW")
	d.parseDB2Version()
	d.logVersionInformation()
	d.addVersionLabelsToCharts()

	return nil
}

// parseDB2Version extracts major/minor version numbers from version string
func (d *DB2) parseDB2Version() {
	if d.version == "" || d.version == "Unknown" {
		return
	}

	// Parse version strings like "DB2 v11.5.7.0", "11.5", "V11R5M0", etc.
	versionStr := d.version

	// Handle different version formats
	if strings.Contains(versionStr, "v") {
		// Format: "DB2 v11.5.7.0"
		parts := strings.Split(versionStr, "v")
		if len(parts) > 1 {
			versionStr = parts[1]
		}
	} else if strings.Contains(versionStr, "V") && strings.Contains(versionStr, "R") {
		// Format: "V11R5M0" (z/OS style)
		versionStr = strings.Replace(versionStr, "V", "", 1)
		versionStr = strings.Replace(versionStr, "R", ".", 1)
		versionStr = strings.Replace(versionStr, "M", ".", 1)
	}

	// Split on dots and parse major.minor
	parts := strings.Split(versionStr, ".")
	if len(parts) >= 2 {
		if major, err := strconv.Atoi(parts[0]); err == nil {
			d.versionMajor = major
		}
		if minor, err := strconv.Atoi(parts[1]); err == nil {
			d.versionMinor = minor
		}
	} else if len(parts) == 1 {
		// Just major version
		if major, err := strconv.Atoi(parts[0]); err == nil {
			d.versionMajor = major
		}
	}

	d.Debugf("parsed DB2 version: major=%d, minor=%d", d.versionMajor, d.versionMinor)
}

// setConfigurationDefaults sets default values for configuration options based on detected version
// Only sets defaults if the admin hasn't explicitly configured the option
func (d *DB2) setConfigurationDefaults() {
	// Helper to create bool pointer
	boolPtr := func(v bool) *bool { return &v }

	// CollectDatabaseMetrics - available on all editions with SYSIBMADM views
	if d.CollectDatabaseMetrics == nil {
		defaultValue := d.edition == "LUW" || d.edition == "Cloud"
		d.CollectDatabaseMetrics = boolPtr(defaultValue)
		d.Debugf("CollectDatabaseMetrics not configured, defaulting to %v (based on DB2 edition: %s)",
			defaultValue, d.edition)
	}

	// CollectBufferpoolMetrics - available on most editions
	if d.CollectBufferpoolMetrics == nil {
		defaultValue := d.edition != "i" // Not available on AS/400
		d.CollectBufferpoolMetrics = boolPtr(defaultValue)
		d.Debugf("CollectBufferpoolMetrics not configured, defaulting to %v (based on DB2 edition: %s)",
			defaultValue, d.edition)
	}

	// CollectTablespaceMetrics - available on all editions
	if d.CollectTablespaceMetrics == nil {
		defaultValue := true
		d.CollectTablespaceMetrics = boolPtr(defaultValue)
		d.Debugf("CollectTablespaceMetrics not configured, defaulting to %v", defaultValue)
	}

	// CollectConnectionMetrics - available with SYSIBMADM views
	if d.CollectConnectionMetrics == nil {
		defaultValue := d.edition == "LUW" || (d.edition == "Cloud" && !d.isDisabled("connection_instances"))
		d.CollectConnectionMetrics = boolPtr(defaultValue)
		d.Debugf("CollectConnectionMetrics not configured, defaulting to %v (based on DB2 edition: %s)",
			defaultValue, d.edition)
	}

	// CollectLockMetrics - available on all editions
	if d.CollectLockMetrics == nil {
		defaultValue := true
		d.CollectLockMetrics = boolPtr(defaultValue)
		d.Debugf("CollectLockMetrics not configured, defaulting to %v", defaultValue)
	}

	// CollectTableMetrics - expensive operation, default to false
	if d.CollectTableMetrics == nil {
		defaultValue := false
		d.CollectTableMetrics = boolPtr(defaultValue)
		d.Debugf("CollectTableMetrics not configured, defaulting to %v (expensive operation)", defaultValue)
	}

	// CollectIndexMetrics - expensive operation, default to false
	if d.CollectIndexMetrics == nil {
		defaultValue := false
		d.CollectIndexMetrics = boolPtr(defaultValue)
		d.Debugf("CollectIndexMetrics not configured, defaulting to %v (expensive operation)", defaultValue)
	}

	// Set defaults for new performance metrics

	// Log final configuration
	d.Infof("Configuration after defaults:")
	d.Infof("  Collection settings: DatabaseMetrics=%v, BufferpoolMetrics=%v, TablespaceMetrics=%v",
		*d.CollectDatabaseMetrics, *d.CollectBufferpoolMetrics, *d.CollectTablespaceMetrics)
	d.Infof("  Collection settings: ConnectionMetrics=%v, LockMetrics=%v, TableMetrics=%v, IndexMetrics=%v",
		*d.CollectConnectionMetrics, *d.CollectLockMetrics, *d.CollectTableMetrics, *d.CollectIndexMetrics)
	d.Infof("  Performance settings: MemoryMetrics=%v, WaitMetrics=%v, TableIOMetrics=%v",
		d.CollectMemoryMetrics, d.CollectWaitMetrics, d.CollectTableIOMetrics)
	d.Infof("  Cardinality limits: MaxDatabases=%d, MaxBufferpools=%d, MaxTablespaces=%d",
		d.MaxDatabases, d.MaxBufferpools, d.MaxTablespaces)
	d.Infof("  Cardinality limits: MaxConnections=%d, MaxTables=%d, MaxIndexes=%d",
		d.MaxConnections, d.MaxTables, d.MaxIndexes)

}

// logVersionInformation logs detected DB2 edition and version for informational purposes only
func (d *DB2) logVersionInformation() {
	d.parseDB2Version()

	// Log edition and version information for user awareness
	d.Infof("DB2 %s edition detected - collector will attempt all configured features with graceful error handling", d.edition)

	// Provide informational context about typical edition/version capabilities
	switch d.edition {
	case "i": // DB2 for i (AS/400)
		d.Infof("DB2 for i (AS/400) typically has limited SYSIBMADM view support")

	case "z/OS": // DB2 for z/OS
		d.Infof("DB2 for z/OS typically has different monitoring capabilities than LUW")

	case "Cloud": // Db2 on Cloud
		d.Infof("Db2 on Cloud typically restricts some system-level metrics compared to standard DB2 LUW")

	case "LUW": // DB2 LUW (most feature-complete)
		if d.versionMajor > 0 {
			d.Infof("DB2 LUW %d.%d detected", d.versionMajor, d.versionMinor)
			if d.versionMajor >= 11 {
				d.Infof("DB2 11+ typically supports all collector features including column store metrics")
			} else if d.versionMajor >= 10 {
				d.Infof("DB2 10+ typically supports most features, some advanced features may not be available")
			} else if d.versionMajor >= 9 && d.versionMinor >= 7 {
				d.Infof("DB2 9.7+ typically supports SYSIBMADM views, some advanced features may not be available")
			} else {
				d.Infof("DB2 %d.%d has limited monitoring capabilities - many features may not be available", d.versionMajor, d.versionMinor)
			}
		} else {
			d.Infof("DB2 LUW version unknown - typical features include comprehensive monitoring")
		}

	default:
		d.Infof("Unknown DB2 edition '%s' - assuming LUW-like capabilities", d.edition)
	}

	d.Infof("Note: Admin configuration takes precedence - all enabled features will be attempted regardless of edition/version")
}

// addVersionLabelsToCharts adds DB2 version and edition labels to all charts
func (d *DB2) addVersionLabelsToCharts() {
	versionLabels := []module.Label{
		{Key: "db2_edition", Value: d.edition},
		{Key: "db2_version", Value: d.version},
	}

	// Add labels to all existing charts
	for _, chart := range *d.charts {
		chart.Labels = append(chart.Labels, versionLabels...)
	}
}

// detectMonGetSupport is no longer needed - we always use MON_GET functions
// Legacy SNAP views have been removed due to static value issues

// detectColumnOrganizedSupport checks if column-organized table metrics are available
// detectColumnOrganizedSupport checks if column-organized metrics are available
// This now simply adds charts based on MON_GET_BUFFERPOOL availability
func (d *DB2) detectColumnOrganizedSupport(ctx context.Context) {
	// Column-organized tables (BLU Acceleration) were introduced in DB2 10.5
	// MON_GET_BUFFERPOOL always includes column metrics, even if zero

	if d.edition == "i" {
		d.Infof("Column-organized tables not available on DB2 for i")
		return
	}

	if d.versionMajor > 0 && d.versionMajor < 10 {
		d.Infof("Column-organized tables require DB2 10.5+, current version %d.%d", d.versionMajor, d.versionMinor)
		return
	}

	if d.versionMajor == 10 && d.versionMinor < 5 {
		d.Infof("Column-organized tables require DB2 10.5+, current version %d.%d", d.versionMajor, d.versionMinor)
		return
	}

	// For DB2 10.5+, add column-organized charts
	// MON_GET_BUFFERPOOL will provide the metrics (may be zero in Community Edition)
	d.addColumnOrganizedCharts()
}

// addColumnOrganizedCharts adds charts for column-organized buffer pool metrics
func (d *DB2) addColumnOrganizedCharts() {
	columnCharts := module.Charts{
		bufferpoolColumnHitRatioChart.Copy(),
		bufferpoolColumnReadsChart.Copy(),
	}

	for _, chart := range columnCharts {
		if err := d.charts.Add(chart); err != nil {
			d.Warningf("failed to add column-organized chart %s: %v", chart.ID, err)
		}
	}

	d.Infof("Added %d column-organized buffer pool charts", len(columnCharts))
}

// Extended metrics and federation detection removed
// These features will be attempted during collection with graceful error handling

// Memory set chart management methods for Screen 26
func (d *DB2) addMemorySetCharts(setKey string) {
	ms := d.memorySets[setKey]
	if ms == nil {
		return
	}

	charts := d.newMemorySetCharts(ms)
	if err := d.charts.Add(*charts...); err != nil {
		d.Warningf("failed to add memory set charts for %s: %v", setKey, err)
	} else {
		d.Debugf("added memory set charts for %s (%s.%s.%s)", setKey, ms.hostName, ms.dbName, ms.setType)
	}
}

func (d *DB2) removeMemorySetCharts(setKey string) {
	setIdentifier := strings.ReplaceAll(setKey, ".", "_")

	// Mark all charts for this memory set as obsolete
	for _, chart := range *d.charts {
		if strings.Contains(chart.ID, fmt.Sprintf("memory_set_%s", setIdentifier)) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}

	d.Debugf("removed memory set charts for %s", setKey)
}

func (d *DB2) addPrefetcherCharts(p *prefetcherInstanceMetrics, bufferPoolName string) {
	charts := d.newPrefetcherCharts(p, bufferPoolName)
	if err := d.charts.Add(*charts...); err != nil {
		d.Warningf("failed to add prefetcher charts for %s: %v", bufferPoolName, err)
	} else {
		d.Debugf("added prefetcher charts for buffer pool %s", bufferPoolName)
	}
}

func (d *DB2) removePrefetcherCharts(bufferPoolName string) {
	cleanName := cleanName(bufferPoolName)

	// Mark all charts for this prefetcher as obsolete
	for _, chart := range *d.charts {
		if strings.Contains(chart.ID, fmt.Sprintf("prefetcher_%s", cleanName)) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}

	d.Debugf("removed prefetcher charts for buffer pool %s", bufferPoolName)
}
