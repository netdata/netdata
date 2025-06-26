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

			// Instance collection defaults
			CollectDiskMetrics:      true,
			CollectSubsystemMetrics: true,
			CollectJobQueueMetrics:  true,
			CollectActiveJobs:       false,

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

		charts:           baseCharts.Copy(),
		once:             &sync.Once{},
		mx:               &metricsData{},
		disks:            make(map[string]*diskMetrics),
		subsystems:       make(map[string]*subsystemMetrics),
		jobQueues:        make(map[string]*jobQueueMetrics),
		messageQueues:    make(map[string]*messageQueueMetrics),
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
	CollectDiskMetrics      bool `yaml:"collect_disk_metrics,omitempty" json:"collect_disk_metrics"`
	CollectSubsystemMetrics bool `yaml:"collect_subsystem_metrics,omitempty" json:"collect_subsystem_metrics"`
	CollectJobQueueMetrics  bool `yaml:"collect_job_queue_metrics,omitempty" json:"collect_job_queue_metrics"`
	CollectActiveJobs       bool `yaml:"collect_active_jobs,omitempty" json:"collect_active_jobs"`

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

	// Selectors
	diskSelector      matcher.Matcher
	subsystemSelector matcher.Matcher
	jobQueueSelector  matcher.Matcher

	// System info for labels
	systemName   string
	serialNumber string
	model        string

	// Resilience tracking
	disabledMetrics  map[string]bool // Track disabled metrics
	disabledFeatures map[string]bool // Track disabled features (table functions)
	osVersion        string          // IBM i version for compatibility checks
}

func (a *AS400) Configuration() any {
	return a.Config
}

func (a *AS400) Init(ctx context.Context) error {
	if a.DSN == "" {
		return errors.New("dsn required but not set")
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
	}

	// Detect available features
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
	db, err := sql.Open("go_ibm_db", a.DSN)
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
	if a.disabledMetrics[key] || a.disabledFeatures[key] {
		return // Already logged
	}
	a.Warningf(format, args...)
}

func (a *AS400) isDisabled(key string) bool {
	return a.disabledMetrics[key] || a.disabledFeatures[key]
}

func isSQLFeatureError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// SQL0204N = object not found (table, view, function)
	// SQL0206N = column not found
	// SQL0443N = function not found
	return strings.Contains(errStr, "SQL0204N") ||
		strings.Contains(errStr, "SQL0206N") ||
		strings.Contains(errStr, "SQL0443N")
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
		a.disabledMetrics[metricKey] = true
		return nil // Not a fatal error
	}

	return err
}

// detectIBMiVersion tries to detect the IBM i OS version
func (a *AS400) detectIBMiVersion(ctx context.Context) error {
	var version, release string

	// Try primary query first
	err := a.doQuery(ctx, queryIBMiVersion, func(column, value string, lineEnd bool) {
		switch column {
		case "OS_VERSION":
			version = value
		case "OS_RELEASE":
			release = value
		}
	})

	if err != nil {
		// Try fallback query
		if err := a.doQuery(ctx, queryIBMiVersionFallback, func(column, value string, lineEnd bool) {
			switch column {
			case "OS_VERSION":
				version = value
			case "OS_RELEASE":
				release = value
			}
		}); err != nil {
			return err
		}
	}

	if version != "" && release != "" {
		a.osVersion = fmt.Sprintf("%s %s", version, release)
		a.Debugf("detected IBM i version: %s", a.osVersion)
	}

	return nil
}

// detectAvailableFeatures checks which table functions are available
func (a *AS400) detectAvailableFeatures(ctx context.Context) {
	// Check if ACTIVE_JOB_INFO is available
	if err := a.doQuery(ctx, queryCheckActiveJobInfo, func(column, value string, lineEnd bool) {
		if column == "CNT" && value == "0" {
			a.disabledFeatures["active_job_info"] = true
			a.Warningf("ACTIVE_JOB_INFO function not available on this IBM i version")
		}
	}); err != nil {
		a.Debugf("failed to check ACTIVE_JOB_INFO availability: %v", err)
	}

	// Check if IFS_OBJECT_STATISTICS is available
	if err := a.doQuery(ctx, queryCheckIFSObjectStats, func(column, value string, lineEnd bool) {
		if column == "CNT" && value == "0" {
			a.disabledFeatures["ifs_object_statistics"] = true
			a.Warningf("IFS_OBJECT_STATISTICS function not available on this IBM i version")
		}
	}); err != nil {
		a.Debugf("failed to check IFS_OBJECT_STATISTICS availability: %v", err)
	}
}
