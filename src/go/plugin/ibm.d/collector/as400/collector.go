// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package as400

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
	
	// Import database drivers and connection utilities
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/pkg/dbdriver"
	_ "github.com/netdata/netdata/go/plugins/plugin/ibm.d/pkg/dbdriver"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("as400", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *AS400 {
	return &AS400{
		Config: Config{
			DSN:           "",
			Timeout:       confopt.Duration(time.Second * 2),
			UpdateEvery:   5,
			MaxDbConns:    1,
			MaxDbLifeTime: confopt.Duration(time.Minute * 10),

			// Instance collection defaults (will be set in Init based on version)
			// CollectDiskMetrics:      nil,
			// CollectSubsystemMetrics: nil,
			// CollectJobQueueMetrics:  nil,
			// CollectActiveJobs:       nil,

			// Cardinality limits
			MaxDisks:      50,
			MaxSubsystems: 20,
			MaxJobQueues:  50,
			MaxActiveJobs: 10,

			// Selectors (empty = collect all)
			DiskSelector:      "",
			SubsystemSelector: "",
			JobQueueSelector:  "",

			// IFS monitoring
			IFSTopNDirectories: 20,
			IFSStartPath:       "/",
		},

		charts:        baseCharts.Copy(),
		once:          &sync.Once{},
		mx:            &metricsData{},
		disks:         make(map[string]*diskMetrics),
		subsystems:    make(map[string]*subsystemMetrics),
		jobQueues:     make(map[string]*jobQueueMetrics),
		messageQueues: make(map[string]*messageQueueMetrics),
		tempStorageNamed: make(map[string]*tempStorageMetrics),
		activeJobs:    make(map[string]*activeJobMetrics),
		networkInterfaces: make(map[string]*networkInterfaceMetrics),
		disabled:      make(map[string]bool),
	}
}

type Config struct {
	Vnode         string           `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery   int              `yaml:"update_every,omitempty" json:"update_every"`
	DSN           string           `yaml:"dsn" json:"dsn"`
	Timeout       confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	MaxDbConns    int              `yaml:"max_db_conns,omitempty" json:"max_db_conns"`
	MaxDbLifeTime confopt.Duration `yaml:"max_db_life_time,omitempty" json:"max_db_life_time"`

	// Individual connection parameters (alternative to DSN)
	Hostname       string `yaml:"hostname,omitempty" json:"hostname"`
	Port           int    `yaml:"port,omitempty" json:"port"`
	Username       string `yaml:"username,omitempty" json:"username"`
	Password       string `yaml:"password,omitempty" json:"password"`
	Database       string `yaml:"database,omitempty" json:"database"`
	ConnectionType string `yaml:"connection_type,omitempty" json:"connection_type"`
	ODBCDriver     string `yaml:"odbc_driver,omitempty" json:"odbc_driver"`
	UseSSL         bool   `yaml:"use_ssl,omitempty" json:"use_ssl"`

	// Instance collection settings
	CollectDiskMetrics      *bool `yaml:"collect_disk_metrics,omitempty" json:"collect_disk_metrics"`
	CollectSubsystemMetrics *bool `yaml:"collect_subsystem_metrics,omitempty" json:"collect_subsystem_metrics"`
	CollectJobQueueMetrics  *bool `yaml:"collect_job_queue_metrics,omitempty" json:"collect_job_queue_metrics"`
	CollectActiveJobs       *bool `yaml:"collect_active_jobs,omitempty" json:"collect_active_jobs"`

	// Cardinality limits
	MaxDisks      int `yaml:"max_disks,omitempty" json:"max_disks"`
	MaxSubsystems int `yaml:"max_subsystems,omitempty" json:"max_subsystems"`
	MaxJobQueues  int `yaml:"max_job_queues,omitempty" json:"max_job_queues"`
	MaxActiveJobs int `yaml:"max_active_jobs,omitempty" json:"max_active_jobs"`

	// Selectors for filtering
	DiskSelector      string `yaml:"collect_disks_matching,omitempty" json:"collect_disks_matching"`
	SubsystemSelector string `yaml:"collect_subsystems_matching,omitempty" json:"collect_subsystems_matching"`
	JobQueueSelector  string `yaml:"collect_job_queues_matching,omitempty" json:"collect_job_queues_matching"`

	// IFS monitoring
	IFSTopNDirectories int    `yaml:"ifs_top_n_directories,omitempty" json:"ifs_top_n_directories"`
	IFSStartPath       string `yaml:"ifs_start_path,omitempty" json:"ifs_start_path"`
}

type AS400 struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	db   *sql.DB
	once *sync.Once
	mx   *metricsData

	// Instance tracking
	disks         map[string]*diskMetrics
	subsystems    map[string]*subsystemMetrics
	jobQueues     map[string]*jobQueueMetrics
	messageQueues map[string]*messageQueueMetrics
	tempStorageNamed map[string]*tempStorageMetrics
	activeJobs    map[string]*activeJobMetrics
	networkInterfaces map[string]*networkInterfaceMetrics

	// Selectors
	diskSelector      matcher.Matcher
	subsystemSelector matcher.Matcher
	jobQueueSelector  matcher.Matcher

	// System info for labels
	systemName          string
	serialNumber        string
	model               string
	technologyRefresh   string // TR level (e.g., "TR1", "TR2", "TR3")

	// Resilience tracking
	disabled       map[string]bool // Track disabled metrics and features
	osVersion      string          // IBM i version for compatibility checks
	versionMajor   int             // Parsed major version (e.g., 7 from "V7R3")
	versionRelease int             // Parsed release number (e.g., 3 from "V7R3")
	versionMod     int             // Parsed modification level (e.g., 5 from "V7R3M5")
}

func (a *AS400) Configuration() any {
	return a.Config
}

func (a *AS400) Init(ctx context.Context) error {
	a.Debugf("Init called with DSN='%s'", a.DSN)
	
	// If no DSN provided, try to build one from individual parameters
	if a.DSN == "" {
		a.Debugf("no DSN provided, checking individual parameters: hostname='%s', username='%s', password='%s'", a.Hostname, a.Username, a.Password)
		if a.Hostname != "" && a.Username != "" && a.Password != "" {
			// Build DSN from individual parameters
			config := &dbdriver.ConnectionConfig{
				Hostname:   a.Hostname,
				Port:       a.Port,
				Username:   a.Username,
				Password:   a.Password,
				Database:   a.Database,
				SystemType: "AS400",
				ODBCDriver: a.ODBCDriver,
				UseSSL:     a.UseSSL,
			}
			
			// Use ODBC connection by default for AS/400
			a.DSN = dbdriver.BuildODBCDSN(config)
			a.Infof("built DSN from connection parameters for AS/400 system: %s", a.Hostname)
		} else {
			return fmt.Errorf("dsn required but not set, and insufficient connection parameters provided (need hostname, username, password). Got: hostname='%s', username='%s', password='%s'", a.Hostname, a.Username, a.Password)
		}
	}

	// Initialize selectors
	if a.DiskSelector != "" {
		m, err := matcher.NewSimplePatternsMatcher(a.DiskSelector)
		if err != nil {
			return fmt.Errorf("invalid disk selector pattern '%s': %v", a.DiskSelector, err)
		}
		a.diskSelector = m
	}

	if a.SubsystemSelector != "" {
		m, err := matcher.NewSimplePatternsMatcher(a.SubsystemSelector)
		if err != nil {
			return fmt.Errorf("invalid subsystem selector pattern '%s': %v", a.SubsystemSelector, err)
		}
		a.subsystemSelector = m
	}

	if a.JobQueueSelector != "" {
		m, err := matcher.NewSimplePatternsMatcher(a.JobQueueSelector)
		if err != nil {
			return fmt.Errorf("invalid job queue selector pattern '%s': %v", a.JobQueueSelector, err)
		}
		a.jobQueueSelector = m
	}

	// Initialize database connection
	db, err := a.initDatabase(ctx)
	if err != nil {
		return err
	}
	a.db = db

	// Detect IBM i version
	if err := a.detectIBMiVersion(ctx); err != nil {
		a.Warningf("failed to detect IBM i version: %v", err)
	} else {
		a.Infof("detected IBM i version: %s", a.osVersion)
	}

	// Log version information for user awareness (no feature gating)
	a.logVersionInformation()

	// Set configuration defaults based on version (only if admin hasn't configured)
	a.setConfigurationDefaults()

	// Detect available features (reactive detection for unknown features)
	a.detectAvailableFeatures(ctx)

	return nil
}

func (a *AS400) Check(ctx context.Context) error {
	mx, err := a.collect(ctx)
	if err != nil {
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (a *AS400) Charts() *module.Charts {
	return a.charts
}

func (a *AS400) Collect(ctx context.Context) map[string]int64 {
	mx, err := a.collect(ctx)
	if err != nil {
		a.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (a *AS400) Cleanup(context.Context) {
	if a.db == nil {
		return
	}
	if err := a.db.Close(); err != nil {
		a.Errorf("cleanup: error closing database: %v", err)
	}
	a.db = nil
}

func (a *AS400) verifyConfig() error {
	if a.DSN == "" {
		return errors.New("DSN is required but not set")
	}
	return nil
}

func (a *AS400) initDatabase(ctx context.Context) (*sql.DB, error) {
	db, err := sql.Open("odbc", a.DSN)
	if err != nil {
		return nil, fmt.Errorf("error opening database: %v", err)
	}

	db.SetMaxOpenConns(a.MaxDbConns)
	db.SetConnMaxLifetime(time.Duration(a.MaxDbLifeTime))

	pingCtx, cancel := context.WithTimeout(ctx, time.Duration(a.Timeout))
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("error pinging database: %v", err)
	}

	return db, nil
}

func (a *AS400) safeDSN() string {
	// Mask password in DSN for logging
	// IBM DB2 format: HOSTNAME=host;PORT=port;DATABASE=db;UID=user;PWD=pass
	if strings.Contains(a.DSN, "PWD=") {
		parts := strings.Split(a.DSN, ";")
		for i, part := range parts {
			if strings.HasPrefix(part, "PWD=") {
				parts[i] = "PWD=******"
			}
		}
		return strings.Join(parts, ";")
	}
	return a.DSN
}

func (a *AS400) ping(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, time.Duration(a.Timeout))
	defer cancel()

	return a.db.PingContext(pingCtx)
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

// Helper functions for resilience
func (a *AS400) logOnce(key string, format string, args ...interface{}) {
	if a.disabled[key] {
		return // Already logged
	}
	a.Warningf(format, args...)
	// Mark as logged to prevent future logs
	a.disabled[key] = true
}

func (a *AS400) isDisabled(key string) bool {
	return a.disabled[key]
}

func isSQLFeatureError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToUpper(err.Error())
	// Common IBM i SQL errors for missing objects
	return strings.Contains(errStr, "SQL0204") || // object not found (table, view, function)
		strings.Contains(errStr, "SQL0206") || // column not found
		strings.Contains(errStr, "SQL0443") || // function not found
		strings.Contains(errStr, "SQL0551") || // authorization error (often means object doesn't exist)
		strings.Contains(errStr, "SQL7024") || // index not found
		strings.Contains(errStr, "SQL0707") || // name too long (version compatibility)
		strings.Contains(errStr, "SQLCODE=-204") || // Alternative format
		strings.Contains(errStr, "SQLCODE=-206") ||
		strings.Contains(errStr, "SQLCODE=-443") ||
		strings.Contains(errStr, "SQLCODE=-551") ||
		strings.Contains(errStr, "SQLCODE=-707")
}

// collectSingleMetric executes a single-value query and handles errors gracefully
func (a *AS400) collectSingleMetric(ctx context.Context, metricKey string, query string, handler func(value string)) error {
	if a.isDisabled(metricKey) {
		return nil
	}

	err := a.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		if value != "" {
			handler(value)
		}
	})

	if err != nil && isSQLFeatureError(err) {
		a.logOnce(metricKey, "metric %s not available on this IBM i version: %v", metricKey, err)
		a.disabled[metricKey] = true
		return nil // Not a fatal error
	}

	return err
}

// detectIBMiVersion tries to detect the IBM i OS version
func (a *AS400) detectIBMiVersion(ctx context.Context) error {
	var version, release string
	versionDetected := false

	// Try primary method: SYSIBMADM.ENV_SYS_INFO
	err := a.doQuery(ctx, queryIBMiVersion, func(column, value string, lineEnd bool) {
		switch column {
		case "OS_NAME":
			// We could use this to verify it's IBM i
			_ = strings.TrimSpace(value)
		case "OS_VERSION":
			version = strings.TrimSpace(value)
		case "OS_RELEASE":
			release = strings.TrimSpace(value)
		}
	})

	if err == nil && version != "" && release != "" {
		versionDetected = true
		a.osVersion = fmt.Sprintf("%s.%s", version, release)
		a.Debugf("detected IBM i version from ENV_SYS_INFO: %s", a.osVersion)
	} else if err != nil {
		a.Debugf("ENV_SYS_INFO query failed: %v, trying fallback method", err)

		// Try fallback method: data area
		err = a.doQuery(ctx, queryIBMiVersionDataArea, func(column, value string, lineEnd bool) {
			if column == "VERSION" {
				dataAreaValue := strings.TrimSpace(value)
				// Parse version from data area value (e.g., "V7R4M0 L")
				if len(dataAreaValue) >= 6 {
					a.osVersion = dataAreaValue
					versionDetected = true
					a.Debugf("detected IBM i version from data area: %s", a.osVersion)
				}
			}
		})

		if err != nil {
			a.Warningf("failed to detect IBM i version from data area: %v", err)
		}
	}

	// If all methods fail, use a reasonable default
	if !versionDetected {
		a.osVersion = "7.4"
		a.Warningf("could not detect IBM i version, using default: %s", a.osVersion)
	}

	// Parse version components
	a.parseIBMiVersion()

	// Collect system information for labels
	a.collectSystemInfo(ctx)

	// Add labels to charts after collecting all system info
	a.addVersionLabelsToCharts()

	return nil
}

// detectAvailableFeatures checks which table functions are available
func (a *AS400) detectAvailableFeatures(ctx context.Context) {
	// Feature detection disabled - assume basic features only
	a.disabled["active_job_info"] = true

	// Additional feature detection disabled
	a.disabled["ifs_object_statistics"] = true
}

// collectSystemInfo gathers system information for chart labels
func (a *AS400) collectSystemInfo(ctx context.Context) {
	// System name collection disabled - no querySystemName available
	a.systemName = "Unknown"

	// Collect serial number
	_ = a.collectSingleMetric(ctx, "serial_number", querySerialNumber, func(value string) {
		a.serialNumber = strings.TrimSpace(value)
		a.Debugf("detected serial number: %s", a.serialNumber)
	})

	// Collect system model
	_ = a.collectSingleMetric(ctx, "system_model", querySystemModel, func(value string) {
		a.model = strings.TrimSpace(value)
		a.Debugf("detected system model: %s", a.model)
	})

	// Collect Technology Refresh level
	err := a.doQuery(ctx, queryTechnologyRefresh, func(column, value string, lineEnd bool) {
		if column == "TR_LEVEL" && value != "" {
			trLevel := strings.TrimSpace(value)
			if trLevel != "" {
				a.technologyRefresh = fmt.Sprintf("TR%s", trLevel)
				a.Debugf("detected Technology Refresh level: %s", a.technologyRefresh)
			}
		}
	})
	if err != nil {
		a.Debugf("failed to detect Technology Refresh level: %v", err)
	}

	// Note: PROCESSOR_FEATURE would require a separate query to SYSTEM_STATUS_INFO
	// but this can conflict with other concurrent queries to the same table.
	// For now, we'll skip processor feature to avoid prepared statement conflicts.

	// Default values if collection fails
	if a.systemName == "" {
		a.systemName = "Unknown"
	}
	if a.serialNumber == "" {
		a.serialNumber = "Unknown"
	}
	if a.model == "" {
		a.model = "Unknown"
	}
	if a.osVersion == "" {
		a.osVersion = "Unknown"
	}
	if a.technologyRefresh == "" {
		a.technologyRefresh = "Unknown"
	}
}

// addVersionLabelsToCharts adds IBM i version labels to all charts
func (a *AS400) addVersionLabelsToCharts() {
	versionLabels := []module.Label{
		{Key: "ibmi_version", Value: a.osVersion},
		{Key: "technology_refresh", Value: a.technologyRefresh},
		{Key: "system_name", Value: a.systemName},
		{Key: "serial_number", Value: a.serialNumber},
		{Key: "model", Value: a.model},
	}

	// Add labels to all existing charts
	for _, chart := range *a.charts {
		chart.Labels = append(chart.Labels, versionLabels...)
	}
}

// parseIBMiVersion extracts version components from the version string
func (a *AS400) parseIBMiVersion() {
	if a.osVersion == "" || a.osVersion == "Unknown" {
		return
	}

	// Parse version strings like "V7 R3", "V7R3M0", "V7R4M0 L", "7.3", "7.4.0", etc.
	versionStr := strings.ToUpper(strings.TrimSpace(a.osVersion))
	
	// Remove any trailing letters/spaces (e.g., "V7R4M0 L" -> "V7R4M0")
	if idx := strings.IndexAny(versionStr, " L"); idx > 0 {
		versionStr = versionStr[:idx]
	}
	
	// Remove all spaces for parsing
	versionStr = strings.ReplaceAll(versionStr, " ", "")

	// Try to parse VxRyMz format
	if strings.HasPrefix(versionStr, "V") {
		versionStr = versionStr[1:] // Remove "V" prefix

		// Extract major version (before R)
		if idx := strings.Index(versionStr, "R"); idx > 0 {
			if major, err := strconv.Atoi(versionStr[:idx]); err == nil {
				a.versionMajor = major
			}

			// Extract release (after R, before M if exists)
			remainder := versionStr[idx+1:]
			if mIdx := strings.Index(remainder, "M"); mIdx > 0 {
				if release, err := strconv.Atoi(remainder[:mIdx]); err == nil {
					a.versionRelease = release
				}
				// Extract modification level (numbers only, stop at first non-digit)
				modStr := remainder[mIdx+1:]
				modNum := ""
				for _, ch := range modStr {
					if ch >= '0' && ch <= '9' {
						modNum += string(ch)
					} else {
						break
					}
				}
				if modNum != "" {
					if mod, err := strconv.Atoi(modNum); err == nil {
						a.versionMod = mod
					}
				}
			} else {
				// No M, just release
				if release, err := strconv.Atoi(remainder); err == nil {
					a.versionRelease = release
				}
			}
		}
	} else {
		// Try numeric format like "7.3" or "7.4.0"
		parts := strings.Split(versionStr, ".")
		if len(parts) >= 2 {
			if major, err := strconv.Atoi(parts[0]); err == nil {
				a.versionMajor = major
			}
			if release, err := strconv.Atoi(parts[1]); err == nil {
				a.versionRelease = release
			}
			if len(parts) >= 3 {
				if mod, err := strconv.Atoi(parts[2]); err == nil {
					a.versionMod = mod
				}
			}
		} else if len(parts) == 1 {
			// Handle simple major version only
			if major, err := strconv.Atoi(parts[0]); err == nil {
				a.versionMajor = major
			}
		}
	}

	a.Debugf("parsed IBM i version: major=%d, release=%d, mod=%d", a.versionMajor, a.versionRelease, a.versionMod)
}

// logVersionInformation logs detected IBM i version for informational purposes only
func (a *AS400) logVersionInformation() {
	// Log base version information for user awareness
	if a.versionMajor > 0 {
		a.Infof("IBM i %d.%d detected - collector will attempt all configured features with graceful error handling", a.versionMajor, a.versionRelease)

		// Provide informational context about typical version capabilities
		if a.versionMajor >= 7 {
			if a.versionRelease >= 5 {
				a.Infof("IBM i 7.5+ typically supports all collector features")
			} else if a.versionRelease >= 3 {
				a.Infof("IBM i 7.3+ typically supports ACTIVE_JOB_INFO and IFS_OBJECT_STATISTICS")
			} else if a.versionRelease >= 2 {
				a.Infof("IBM i 7.2+ typically supports MESSAGE_QUEUE_INFO and JOB_QUEUE_ENTRIES")
			} else {
				a.Infof("IBM i 7.1+ has basic SQL services - some advanced features may not be available")
			}
		} else {
			a.Infof("IBM i %d.x has limited SQL services - many features may not be available", a.versionMajor)
		}

		a.Infof("Note: Admin configuration takes precedence - all enabled features will be attempted regardless of version")
	} else {
		a.Infof("IBM i version unknown - collector will attempt all configured features with graceful error handling")
	}
}

// setConfigurationDefaults sets default values for configuration options based on detected version
// Only sets defaults if the admin hasn't explicitly configured the option
func (a *AS400) setConfigurationDefaults() {
	// Helper to create bool pointer
	boolPtr := func(v bool) *bool { return &v }

	// CollectDiskMetrics - available on all versions
	if a.CollectDiskMetrics == nil {
		a.CollectDiskMetrics = boolPtr(true)
		a.Debugf("CollectDiskMetrics not configured, defaulting to true")
	}

	// CollectSubsystemMetrics - available on all versions
	if a.CollectSubsystemMetrics == nil {
		a.CollectSubsystemMetrics = boolPtr(true)
		a.Debugf("CollectSubsystemMetrics not configured, defaulting to true")
	}

	// CollectJobQueueMetrics - requires V7R2+
	if a.CollectJobQueueMetrics == nil {
		defaultValue := a.versionMajor >= 7 && a.versionRelease >= 2
		a.CollectJobQueueMetrics = boolPtr(defaultValue)
		a.Debugf("CollectJobQueueMetrics not configured, defaulting to %v (based on IBM i %d.%d)",
			defaultValue, a.versionMajor, a.versionRelease)
	}

	// CollectActiveJobs - requires V7R3+
	if a.CollectActiveJobs == nil {
		// Default to false even if version supports it (expensive operation)
		a.CollectActiveJobs = boolPtr(false)
		a.Debugf("CollectActiveJobs not configured, defaulting to false (expensive operation)")
	}

	// Log final configuration
	a.Infof("Configuration after defaults: DiskMetrics=%v, SubsystemMetrics=%v, JobQueueMetrics=%v, ActiveJobs=%v",
		*a.CollectDiskMetrics, *a.CollectSubsystemMetrics, *a.CollectJobQueueMetrics, *a.CollectActiveJobs)
}
