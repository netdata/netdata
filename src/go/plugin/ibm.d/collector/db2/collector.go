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

	_ "github.com/ibmdb/go_ibm_db"
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

			// Instance collection defaults
			CollectDatabaseMetrics:   true,
			CollectBufferpoolMetrics: true,
			CollectTablespaceMetrics: true,
			CollectConnectionMetrics: true,
			CollectLockMetrics:       true,
			CollectTableMetrics:      false,
			CollectIndexMetrics:      false,

			// Cardinality limits
			MaxDatabases:   10,
			MaxBufferpools: 20,
			MaxTablespaces: 100,
			MaxConnections: 200,
			MaxTables:      50,
			MaxIndexes:     100,

			// Backup history
			BackupHistoryDays: 30,
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
	CollectDatabaseMetrics   bool `yaml:"collect_database_metrics,omitempty" json:"collect_database_metrics"`
	CollectBufferpoolMetrics bool `yaml:"collect_bufferpool_metrics,omitempty" json:"collect_bufferpool_metrics"`
	CollectTablespaceMetrics bool `yaml:"collect_tablespace_metrics,omitempty" json:"collect_tablespace_metrics"`
	CollectConnectionMetrics bool `yaml:"collect_connection_metrics,omitempty" json:"collect_connection_metrics"`
	CollectLockMetrics       bool `yaml:"collect_lock_metrics,omitempty" json:"collect_lock_metrics"`
	CollectTableMetrics      bool `yaml:"collect_table_metrics,omitempty" json:"collect_table_metrics"`
	CollectIndexMetrics      bool `yaml:"collect_index_metrics,omitempty" json:"collect_index_metrics"`

	// Cardinality limits
	MaxDatabases   int `yaml:"max_databases,omitempty" json:"max_databases"`
	MaxBufferpools int `yaml:"max_bufferpools,omitempty" json:"max_bufferpools"`
	MaxTablespaces int `yaml:"max_tablespaces,omitempty" json:"max_tablespaces"`
	MaxConnections int `yaml:"max_connections,omitempty" json:"max_connections"`
	MaxTables      int `yaml:"max_tables,omitempty" json:"max_tables"`
	MaxIndexes     int `yaml:"max_indexes,omitempty" json:"max_indexes"`

	// Backup history
	BackupHistoryDays int `yaml:"backup_history_days,omitempty" json:"backup_history_days"`

	// Selectors for filtering
	DatabaseSelector   string `yaml:"collect_databases_matching,omitempty" json:"collect_databases_matching"`
	BufferpoolSelector string `yaml:"collect_bufferpools_matching,omitempty" json:"collect_bufferpools_matching"`
	TablespaceSelector string `yaml:"collect_tablespaces_matching,omitempty" json:"collect_tablespaces_matching"`
	ConnectionSelector string `yaml:"collect_connections_matching,omitempty" json:"collect_connections_matching"`
	TableSelector      string `yaml:"collect_tables_matching,omitempty" json:"collect_tables_matching"`
	IndexSelector      string `yaml:"collect_indexes_matching,omitempty" json:"collect_indexes_matching"`
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

	// Resilience tracking (following AS/400 pattern)
	disabledMetrics  map[string]bool // Track disabled metrics due to version incompatibility
	disabledFeatures map[string]bool // Track disabled features (tables, views, functions)

	// Modern monitoring support
	useMonGetFunctions bool // Use MON_GET_* functions instead of SNAP* views when available
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
	if d.DatabaseSelector != "" {
		m, err := matcher.NewSimplePatternsMatcher(d.DatabaseSelector)
		if err != nil {
			return fmt.Errorf("invalid database selector pattern '%s': %v", d.DatabaseSelector, err)
		}
		d.databaseSelector = m
	}

	if d.BufferpoolSelector != "" {
		m, err := matcher.NewSimplePatternsMatcher(d.BufferpoolSelector)
		if err != nil {
			return fmt.Errorf("invalid bufferpool selector pattern '%s': %v", d.BufferpoolSelector, err)
		}
		d.bufferpoolSelector = m
	}

	if d.TablespaceSelector != "" {
		m, err := matcher.NewSimplePatternsMatcher(d.TablespaceSelector)
		if err != nil {
			return fmt.Errorf("invalid tablespace selector pattern '%s': %v", d.TablespaceSelector, err)
		}
		d.tablespaceSelector = m
	}

	if d.ConnectionSelector != "" {
		m, err := matcher.NewSimplePatternsMatcher(d.ConnectionSelector)
		if err != nil {
			return fmt.Errorf("invalid connection selector pattern '%s': %v", d.ConnectionSelector, err)
		}
		d.connectionSelector = m
	}

	if d.TableSelector != "" {
		m, err := matcher.NewSimplePatternsMatcher(d.TableSelector)
		if err != nil {
			return fmt.Errorf("invalid table selector pattern '%s': %v", d.TableSelector, err)
		}
		d.tableSelector = m
	}

	if d.IndexSelector != "" {
		m, err := matcher.NewSimplePatternsMatcher(d.IndexSelector)
		if err != nil {
			return fmt.Errorf("invalid index selector pattern '%s': %v", d.IndexSelector, err)
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
	d.Debugf("connecting to DB2 with DSN: %s", safeDSN(d.DSN))

	db, err := sql.Open("go_ibm_db", d.DSN)
	if err != nil {
		return nil, fmt.Errorf("error opening database: %v", err)
	}

	db.SetMaxOpenConns(d.MaxDbConns)
	db.SetConnMaxLifetime(time.Duration(d.MaxDbLifeTime))

	pingCtx, cancel := context.WithTimeout(ctx, time.Duration(d.Timeout))
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("error pinging database: %v", err)
	}

	d.Debugf("successfully connected to DB2")
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

// Resilience functions following AS/400 pattern

// logOnce logs a warning message only once per key to avoid spam
func (d *DB2) logOnce(key string, format string, args ...interface{}) {
	if d.disabledMetrics[key] || d.disabledFeatures[key] {
		return // Already logged
	}
	d.Warningf(format, args...)
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
		strings.Contains(errStr, "SQLCODE=-551")
}

// collectSingleMetric executes a single-value query and handles version-specific errors gracefully
func (d *DB2) collectSingleMetric(ctx context.Context, metricKey string, query string, handler func(value string)) error {
	if d.isDisabled(metricKey) {
		return nil
	}

	var value string
	queryCtx, cancel := context.WithTimeout(ctx, time.Duration(d.Timeout))
	defer cancel()

	err := d.db.QueryRowContext(queryCtx, query).Scan(&value)
	if err != nil {
		if isSQLFeatureError(err) {
			d.logOnce(metricKey, "metric %s not available on this DB2 edition/version: %v", metricKey, err)
			d.disabledMetrics[metricKey] = true
			return nil // Not a fatal error
		}
		return err
	}

	if value != "" {
		handler(value)
	}

	return nil
}

// detectDB2Edition tries to detect the DB2 edition and version for compatibility
func (d *DB2) detectDB2Edition(ctx context.Context) error {
	// Try to detect LUW first (most common)
	err := d.collectSingleMetric(ctx, "version_detection_luw", queryDetectVersionLUW, func(value string) {
		d.edition = "LUW"
		d.version = value
		d.Debugf("detected DB2 LUW edition, version: %s", value)
	})

	if err == nil && d.edition != "" {
		d.applyVersionBasedFeatureGating()
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
		d.applyVersionBasedFeatureGating()
		d.addVersionLabelsToCharts()
		return nil
	}

	// Try to detect z/OS by checking for z/OS specific tables
	err = d.collectSingleMetric(ctx, "version_detection_zos", queryDetectVersionZOS, func(value string) {
		d.edition = "z/OS"
		d.version = value
		d.Debugf("detected DB2 for z/OS edition")
	})

	if err == nil && d.edition != "" {
		d.applyVersionBasedFeatureGating()
		d.addVersionLabelsToCharts()
		return nil
	}

	// Try to detect Db2 on Cloud - first method
	err = d.collectSingleMetric(ctx, "version_detection_cloud", queryDetectVersionCloud, func(value string) {
		d.edition = "Cloud"
		d.version = d.version // Keep LUW version if already detected
		if d.version == "" {
			d.version = "Db2 on Cloud"
		}
		d.Debugf("detected Db2 on Cloud edition via BLUADMIN schema")
	})

	if err == nil && d.edition != "" {
		d.applyVersionBasedFeatureGating()
		d.addVersionLabelsToCharts()
		return nil
	}

	// Try alternative Cloud detection
	err = d.collectSingleMetric(ctx, "version_detection_cloud_alt", queryDetectVersionCloudAlt, func(value string) {
		if value == "Db2 on Cloud" {
			d.edition = "Cloud"
			d.version = d.version // Keep LUW version if already detected
			if d.version == "" {
				d.version = "Db2 on Cloud"
			}
			d.Debugf("detected Db2 on Cloud edition via PROD_RELEASE")
		}
	})

	if err == nil && d.edition != "" {
		d.applyVersionBasedFeatureGating()
		d.addVersionLabelsToCharts()
		return nil
	}

	// Default to LUW if all detection fails
	d.edition = "LUW"
	d.version = "Unknown"
	d.Warningf("could not detect DB2 edition, defaulting to LUW")
	d.applyVersionBasedFeatureGating()
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

// applyVersionBasedFeatureGating proactively disables features based on DB2 edition and version
func (d *DB2) applyVersionBasedFeatureGating() {
	d.parseDB2Version()

	// Edition-based feature gating
	switch d.edition {
	case "i": // DB2 for i (AS/400)
		d.disableFeatureWithLog("sysibmadm_views", "SYSIBMADM views not available on DB2 for i (AS/400)")
		d.disableFeatureWithLog("advanced_monitoring", "Advanced monitoring views not available on DB2 for i")

	case "z/OS": // DB2 for z/OS
		d.disableFeatureWithLog("bufferpool_detailed_metrics", "Detailed buffer pool metrics limited on DB2 for z/OS")
		d.logFeatureAvailability("z/OS specific features enabled for DB2 for z/OS")

	case "Cloud": // Db2 on Cloud
		// Db2 on Cloud has a restricted schema compared to standard DB2 LUW
		d.disableFeatureWithLog("column_organized_metrics", "Column-organized table metrics (POOL_COL_*) not available on Db2 on Cloud")
		d.disableFeatureWithLog("lbp_pages_found_metrics", "Detailed buffer pool hit metrics (*_LBP_PAGES_FOUND) not available on Db2 on Cloud")
		d.disableFeatureWithLog("uow_comp_status", "UOW_COMP_STATUS column not available in APPLICATIONS view on Db2 on Cloud")
		d.disableFeatureWithLog("application_name", "APPLICATION_NAME column not available in APPLICATIONS view on Db2 on Cloud")
		d.disableFeatureWithLog("rows_modified", "ROWS_MODIFIED column not available in SNAPDB view on Db2 on Cloud")
		d.disableFeatureWithLog("pagesize", "PAGESIZE column not available in SNAPBP view on Db2 on Cloud")
		d.disableFeatureWithLog("free_pages", "FREE_PAGES column not available in SNAPBP view on Db2 on Cloud")
		d.disableFeatureWithLog("total_log_metrics", "TOTAL_LOG_USED/AVAILABLE not available in LOG_UTILIZATION on Db2 on Cloud")
		d.disableFeatureWithLog("bufferpool_instances", "Per-bufferpool instance metrics limited on Db2 on Cloud")
		d.disableFeatureWithLog("connection_instances", "Per-connection instance metrics limited on Db2 on Cloud")
		d.logFeatureAvailability("Cloud-specific monitoring enabled for Db2 on Cloud - using simplified metrics")

	case "LUW": // DB2 LUW (most feature-complete)
		d.logFeatureAvailability("Full feature set available for DB2 LUW")

	default:
		d.Warningf("Unknown DB2 edition '%s', assuming LUW feature set", d.edition)
	}

	// Version-based feature gating (primarily for LUW)
	if d.edition == "LUW" && d.versionMajor > 0 {
		if d.versionMajor < 9 || (d.versionMajor == 9 && d.versionMinor < 7) {
			// DB2 < 9.7 doesn't have SYSIBMADM views
			d.disableFeatureWithLog("sysibmadm_views", "SYSIBMADM views require DB2 9.7 or later (detected %d.%d)", d.versionMajor, d.versionMinor)
			d.disableFeatureWithLog("advanced_monitoring", "Advanced monitoring requires DB2 9.7 or later")
		}

		if d.versionMajor < 10 {
			d.disableFeatureWithLog("extended_monitoring", "Extended monitoring features require DB2 10.0 or later")
		}

		if d.versionMajor < 11 {
			d.disableFeatureWithLog("columnstore_metrics", "Column store metrics require DB2 11.0 or later")
		}

		// Log enabled features for newer versions
		if d.versionMajor >= 11 {
			d.logFeatureAvailability("Full DB2 11+ feature set enabled including column store metrics")
		} else if d.versionMajor >= 10 {
			d.logFeatureAvailability("DB2 10+ features enabled, column store metrics disabled")
		} else if d.versionMajor >= 9 && d.versionMinor >= 7 {
			d.logFeatureAvailability("DB2 9.7+ features enabled, advanced features disabled")
		}
	}
}

// disableFeatureWithLog disables a feature and logs the reason
func (d *DB2) disableFeatureWithLog(feature string, reason string, args ...interface{}) {
	d.disabledFeatures[feature] = true
	d.Infof("Feature disabled: %s - "+reason, append([]interface{}{feature}, args...)...)
}

// logFeatureAvailability logs what features are available
func (d *DB2) logFeatureAvailability(message string) {
	d.Infof("Feature availability: %s", message)
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

// detectMonGetSupport checks if MON_GET_* functions are available and usable
func (d *DB2) detectMonGetSupport(ctx context.Context) {
	// MON_GET_* functions are available in DB2 9.7+ for LUW
	// They're not available on DB2 for i (AS/400) or older versions

	// First check if we should even try (edition and version check)
	if d.edition == "i" {
		d.Infof("MON_GET_* functions not available on DB2 for i, using SNAP* views")
		d.useMonGetFunctions = false
		return
	}

	if d.edition == "LUW" && d.versionMajor > 0 {
		if d.versionMajor < 9 || (d.versionMajor == 9 && d.versionMinor < 7) {
			d.Infof("MON_GET_* functions require DB2 9.7+, using SNAP* views for version %d.%d", d.versionMajor, d.versionMinor)
			d.useMonGetFunctions = false
			return
		}
	}

	// Try a simple MON_GET_DATABASE query to verify it works
	testQuery := `SELECT COUNT(*) FROM TABLE(MON_GET_DATABASE(-1)) AS T`
	var count int

	queryCtx, cancel := context.WithTimeout(ctx, time.Duration(d.Timeout))
	defer cancel()

	err := d.db.QueryRowContext(queryCtx, testQuery).Scan(&count)
	if err != nil {
		if isSQLFeatureError(err) {
			d.Infof("MON_GET_* functions not available or not authorized: %v, using SNAP* views", err)
			d.useMonGetFunctions = false
		} else {
			d.Warningf("Error testing MON_GET_* support: %v, falling back to SNAP* views", err)
			d.useMonGetFunctions = false
		}
		return
	}

	// MON_GET functions are available and working
	d.useMonGetFunctions = true
	d.Infof("MON_GET_* functions detected and available - using modern monitoring approach for better performance")
}
