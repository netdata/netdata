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

		charts:        baseCharts.Copy(),
		once:          &sync.Once{},
		mx:            &metricsData{},
		disks:         make(map[string]*diskMetrics),
		subsystems:    make(map[string]*subsystemMetrics),
		jobQueues:     make(map[string]*jobQueueMetrics),
		messageQueues: make(map[string]*messageQueueMetrics),
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

// Instance management methods
func (a *AS400) getDiskMetrics(unit string) *diskMetrics {
	if a.disks[unit] == nil {
		a.disks[unit] = &diskMetrics{unit: unit}
	}
	return a.disks[unit]
}

func (a *AS400) getSubsystemMetrics(name string) *subsystemMetrics {
	if a.subsystems[name] == nil {
		a.subsystems[name] = &subsystemMetrics{name: name}
	}
	return a.subsystems[name]
}

func (a *AS400) getJobQueueMetrics(key string) *jobQueueMetrics {
	if a.jobQueues[key] == nil {
		a.jobQueues[key] = &jobQueueMetrics{}
	}
	return a.jobQueues[key]
}

func (a *AS400) getMessageQueueMetrics(key string) *messageQueueMetrics {
	if a.messageQueues[key] == nil {
		a.messageQueues[key] = &messageQueueMetrics{}
	}
	return a.messageQueues[key]
}

// Chart management methods
func (a *AS400) addDiskCharts(disk *diskMetrics) {
	charts := module.Charts{
		{
			ID:       fmt.Sprintf("disk_%s_busy", cleanName(disk.unit)),
			Title:    fmt.Sprintf("Disk %s Busy", disk.unit),
			Units:    "percentage",
			Fam:      "disk",
			Ctx:      "as400.disk_busy",
			Priority: module.Priority + 100,
			Labels: []module.Label{
				{Key: "disk", Value: disk.unit},
			},
			Dims: module.Dims{
				{ID: fmt.Sprintf("disk_%s_busy_percent", cleanName(disk.unit)), Name: "busy", Div: precision},
			},
		},
		{
			ID:       fmt.Sprintf("disk_%s_io_requests", cleanName(disk.unit)),
			Title:    fmt.Sprintf("Disk %s I/O Requests", disk.unit),
			Units:    "requests/s",
			Fam:      "disk",
			Ctx:      "as400.disk_io_requests",
			Priority: module.Priority + 101,
			Type:     module.Stacked,
			Labels: []module.Label{
				{Key: "disk", Value: disk.unit},
			},
			Dims: module.Dims{
				{ID: fmt.Sprintf("disk_%s_read_requests", cleanName(disk.unit)), Name: "read", Algo: module.Incremental},
				{ID: fmt.Sprintf("disk_%s_write_requests", cleanName(disk.unit)), Name: "write", Algo: module.Incremental},
			},
		},
		{
			ID:       fmt.Sprintf("disk_%s_io_bandwidth", cleanName(disk.unit)),
			Title:    fmt.Sprintf("Disk %s I/O Bandwidth", disk.unit),
			Units:    "bytes/s",
			Fam:      "disk",
			Ctx:      "as400.disk_io_bandwidth",
			Priority: module.Priority + 102,
			Type:     module.Area,
			Labels: []module.Label{
				{Key: "disk", Value: disk.unit},
			},
			Dims: module.Dims{
				{ID: fmt.Sprintf("disk_%s_read_bytes", cleanName(disk.unit)), Name: "read", Algo: module.Incremental},
				{ID: fmt.Sprintf("disk_%s_write_bytes", cleanName(disk.unit)), Name: "write", Algo: module.Incremental, Mul: -1},
			},
		},
		{
			ID:       fmt.Sprintf("disk_%s_average_time", cleanName(disk.unit)),
			Title:    fmt.Sprintf("Disk %s Average Request Time", disk.unit),
			Units:    "milliseconds",
			Fam:      "disk",
			Ctx:      "as400.disk_average_time",
			Priority: module.Priority + 103,
			Labels: []module.Label{
				{Key: "disk", Value: disk.unit},
			},
			Dims: module.Dims{
				{ID: fmt.Sprintf("disk_%s_average_time", cleanName(disk.unit)), Name: "time"},
			},
		},
	}

	for _, chart := range charts {
		if err := a.charts.Add(chart); err != nil {
			a.Warningf("failed to add disk chart for %s: %v", disk.unit, err)
		}
	}
}

func (a *AS400) removeDiskCharts(unit string) {
	cleanUnit := cleanName(unit)
	_ = a.charts.Remove(fmt.Sprintf("disk_%s_busy", cleanUnit))
	_ = a.charts.Remove(fmt.Sprintf("disk_%s_io_requests", cleanUnit))
	_ = a.charts.Remove(fmt.Sprintf("disk_%s_io_bandwidth", cleanUnit))
	_ = a.charts.Remove(fmt.Sprintf("disk_%s_average_time", cleanUnit))
}

func (a *AS400) addSubsystemCharts(subsystem *subsystemMetrics) {
	charts := module.Charts{
		{
			ID:       fmt.Sprintf("subsystem_%s_jobs", cleanName(subsystem.name)),
			Title:    fmt.Sprintf("Subsystem %s Jobs", subsystem.name),
			Units:    "jobs",
			Fam:      "subsystems",
			Ctx:      "as400.subsystem_jobs",
			Priority: module.Priority + 200,
			Type:     module.Stacked,
			Labels: []module.Label{
				{Key: "subsystem", Value: subsystem.name},
				{Key: "library", Value: subsystem.library},
			},
			Dims: module.Dims{
				{ID: fmt.Sprintf("subsystem_%s_jobs_active", cleanName(subsystem.name)), Name: "active"},
				{ID: fmt.Sprintf("subsystem_%s_jobs_held", cleanName(subsystem.name)), Name: "held"},
			},
		},
		{
			ID:       fmt.Sprintf("subsystem_%s_storage", cleanName(subsystem.name)),
			Title:    fmt.Sprintf("Subsystem %s Storage Used", subsystem.name),
			Units:    "kilobytes",
			Fam:      "subsystems",
			Ctx:      "as400.subsystem_storage",
			Priority: module.Priority + 201,
			Labels: []module.Label{
				{Key: "subsystem", Value: subsystem.name},
				{Key: "library", Value: subsystem.library},
			},
			Dims: module.Dims{
				{ID: fmt.Sprintf("subsystem_%s_storage_used_kb", cleanName(subsystem.name)), Name: "used"},
			},
		},
		{
			ID:       fmt.Sprintf("subsystem_%s_jobs_current", cleanName(subsystem.name)),
			Title:    fmt.Sprintf("Subsystem %s Current Jobs", subsystem.name),
			Units:    "jobs",
			Fam:      "subsystems",
			Ctx:      "as400.subsystem_current_jobs",
			Priority: module.Priority + 202,
			Labels: []module.Label{
				{Key: "subsystem", Value: subsystem.name},
				{Key: "library", Value: subsystem.library},
			},
			Dims: module.Dims{
				{ID: fmt.Sprintf("subsystem_%s_current_jobs", cleanName(subsystem.name)), Name: "current"},
			},
		},
	}

	for _, chart := range charts {
		if err := a.charts.Add(chart); err != nil {
			a.Warningf("failed to add subsystem chart for %s: %v", subsystem.name, err)
		}
	}
}

func (a *AS400) removeSubsystemCharts(name string) {
	cleanName := cleanName(name)
	_ = a.charts.Remove(fmt.Sprintf("subsystem_%s_jobs", cleanName))
	_ = a.charts.Remove(fmt.Sprintf("subsystem_%s_storage", cleanName))
	_ = a.charts.Remove(fmt.Sprintf("subsystem_%s_jobs_current", cleanName))
}

func (a *AS400) addJobQueueCharts(queue *jobQueueMetrics, key string) {
	cleanKey := cleanName(key)
	charts := module.Charts{
		{
			ID:       fmt.Sprintf("jobqueue_%s_jobs", cleanKey),
			Title:    fmt.Sprintf("Job Queue %s/%s Jobs", queue.library, queue.name),
			Units:    "jobs",
			Fam:      "job_queues",
			Ctx:      "as400.job_queue_jobs",
			Priority: module.Priority + 300,
			Type:     module.Stacked,
			Labels: []module.Label{
				{Key: "queue", Value: queue.name},
				{Key: "library", Value: queue.library},
			},
			Dims: module.Dims{
				{ID: fmt.Sprintf("jobqueue_%s_jobs_waiting", cleanKey), Name: "waiting"},
				{ID: fmt.Sprintf("jobqueue_%s_jobs_held", cleanKey), Name: "held"},
				{ID: fmt.Sprintf("jobqueue_%s_jobs_scheduled", cleanKey), Name: "scheduled"},
			},
		},
	}

	for _, chart := range charts {
		if err := a.charts.Add(chart); err != nil {
			a.Warningf("failed to add job queue chart for %s: %v", key, err)
		}
	}
}

func (a *AS400) removeJobQueueCharts(key string) {
	cleanKey := cleanName(key)
	_ = a.charts.Remove(fmt.Sprintf("jobqueue_%s_jobs", cleanKey))
}

func (a *AS400) cleanupStaleInstances() {
	// Clean up stale disk instances
	for unit, disk := range a.disks {
		if !disk.updated {
			delete(a.disks, unit)
			a.removeDiskCharts(unit)
		}
	}

	// Clean up stale subsystem instances
	for name, subsystem := range a.subsystems {
		if !subsystem.updated {
			delete(a.subsystems, name)
			a.removeSubsystemCharts(name)
		}
	}

	// Clean up stale job queue instances
	for key, queue := range a.jobQueues {
		if !queue.updated {
			delete(a.jobQueues, key)
			a.removeJobQueueCharts(key)
		}
	}
}
