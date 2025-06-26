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
	version    string
	edition    string // LUW, z/OS, i, Cloud
	serverInfo serverInfo
	
	// Resilience tracking (following AS/400 pattern)
	disabledMetrics  map[string]bool // Track disabled metrics due to version incompatibility
	disabledFeatures map[string]bool // Track disabled features (tables, views, functions)
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
	return strings.Contains(errStr, "SQL0204N") ||  // object not found (table, view, function)
		strings.Contains(errStr, "SQL0206N") ||     // column not found  
		strings.Contains(errStr, "SQL0443N") ||     // function not found
		strings.Contains(errStr, "SQL0551N") ||     // authorization error (often means table doesn't exist)
		strings.Contains(errStr, "SQL0707N") ||     // name too long (version compatibility)
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
		return nil
	}

	// Try to detect DB2 for i (AS/400)
	err = d.collectSingleMetric(ctx, "version_detection_i", queryDetectVersionI, func(value string) {
		d.edition = "i"
		d.version = "DB2 for i"
		d.Debugf("detected DB2 for i (AS/400) edition")
	})
	
	if err == nil && d.edition != "" {
		return nil
	}

	// Try to detect z/OS by checking for z/OS specific tables
	err = d.collectSingleMetric(ctx, "version_detection_zos", queryDetectVersionZOS, func(value string) {
		d.edition = "z/OS"
		d.version = value
		d.Debugf("detected DB2 for z/OS edition")
	})
	
	if err == nil && d.edition != "" {
		return nil
	}

	// Try to detect Db2 on Cloud
	err = d.collectSingleMetric(ctx, "version_detection_cloud", queryDetectVersionCloud, func(value string) {
		d.edition = "Cloud"
		d.version = value
		d.Debugf("detected Db2 on Cloud edition")
	})
	
	if err == nil && d.edition != "" {
		return nil
	}

	// Default to LUW if all detection fails
	d.edition = "LUW"
	d.version = "Unknown"
	d.Warningf("could not detect DB2 edition, defaulting to LUW")
	
	return nil
}
