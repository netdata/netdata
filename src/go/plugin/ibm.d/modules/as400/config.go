package as400

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
)

// Config defines the user-facing configuration for the AS400 collector.
type Config struct {
	framework.Config `yaml:",inline" json:",inline"`

	// Vnode allows binding the collector to a virtual node.
	Vnode string `yaml:"vnode,omitempty" json:"vnode"`

	// DSN provides a full IBM i ODBC connection string if manual override is needed.
	DSN string `yaml:"dsn" json:"dsn"`

	// Timeout controls how long to wait for SQL statements and RPCs.
	Timeout confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`

	// MaxDbConns restricts the maximum number of open ODBC connections.
	MaxDbConns int `yaml:"max_db_conns,omitempty" json:"max_db_conns"`

	// MaxDbLifeTime limits how long a pooled connection may live before being recycled.
	MaxDbLifeTime confopt.Duration `yaml:"max_db_life_time,omitempty" json:"max_db_life_time"`

	// Hostname is the remote IBM i host to monitor.
	Hostname string `yaml:"hostname,omitempty" json:"hostname"`

	// Port is the TCP port for the IBM i Access ODBC server.
	Port int `yaml:"port,omitempty" json:"port"`

	// Username supplies the credentials used for authentication.
	Username string `yaml:"username,omitempty" json:"username"`

	// Password supplies the password used for authentication.
	Password string `yaml:"password,omitempty" json:"password"`

	// Database selects the IBM i database (library) to use when building the DSN.
	Database string `yaml:"database,omitempty" json:"database"`

	// ConnectionType selects how the collector connects (currently only "odbc").
	ConnectionType string `yaml:"connection_type,omitempty" json:"connection_type"`

	// ODBCDriver specifies the driver name registered on the host.
	ODBCDriver string `yaml:"odbc_driver,omitempty" json:"odbc_driver"`

	// UseSSL enables TLS for the ODBC connection when supported by the driver.
	UseSSL bool `yaml:"use_ssl,omitempty" json:"use_ssl"`

	// ResetStatistics toggles destructive SQL services that reset system statistics on each query.
	ResetStatistics bool `yaml:"reset_statistics,omitempty" json:"reset_statistics"`

	// CollectDiskMetrics toggles collection of disk unit statistics.
	CollectDiskMetrics *bool `yaml:"collect_disk_metrics,omitempty" json:"collect_disk_metrics"`

	// CollectSubsystemMetrics toggles collection of subsystem activity metrics.
	CollectSubsystemMetrics *bool `yaml:"collect_subsystem_metrics,omitempty" json:"collect_subsystem_metrics"`

	// CollectJobQueueMetrics toggles collection of job queue backlog metrics.
	CollectJobQueueMetrics *bool `yaml:"collect_job_queue_metrics,omitempty" json:"collect_job_queue_metrics"`

	// CollectActiveJobs toggles collection of detailed per-job metrics.
	CollectActiveJobs *bool `yaml:"collect_active_jobs,omitempty" json:"collect_active_jobs"`

	// CollectHTTPServerMetrics toggles collection of IBM HTTP Server statistics.
	CollectHTTPServerMetrics *bool `yaml:"collect_http_server_metrics,omitempty" json:"collect_http_server_metrics"`

	// CollectPlanCacheMetrics toggles collection of plan cache analysis metrics.
	CollectPlanCacheMetrics *bool `yaml:"collect_plan_cache_metrics,omitempty" json:"collect_plan_cache_metrics"`

	// MaxDisks caps how many disk units may be charted.
	MaxDisks int `yaml:"max_disks,omitempty" json:"max_disks"`

	// MaxSubsystems caps how many subsystems may be charted.
	MaxSubsystems int `yaml:"max_subsystems,omitempty" json:"max_subsystems"`

	// MaxJobQueues caps how many job queues may be charted.
	MaxJobQueues int `yaml:"max_job_queues,omitempty" json:"max_job_queues"`

	// MaxActiveJobs caps how many active jobs may be charted.
	MaxActiveJobs int `yaml:"max_active_jobs,omitempty" json:"max_active_jobs"`

	// DiskSelector filters disk units by name using glob-style patterns.
	DiskSelector string `yaml:"collect_disks_matching,omitempty" json:"collect_disks_matching"`

	// SubsystemSelector filters subsystems by name using glob-style patterns.
	SubsystemSelector string `yaml:"collect_subsystems_matching,omitempty" json:"collect_subsystems_matching"`

	// JobQueueSelector filters job queues by name using glob-style patterns.
	JobQueueSelector string `yaml:"collect_job_queues_matching,omitempty" json:"collect_job_queues_matching"`
}
