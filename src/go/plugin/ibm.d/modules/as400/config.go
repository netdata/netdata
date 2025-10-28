package as400

import (
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
)

// Config defines the user-facing configuration for the AS400 collector.
type Config struct {
	framework.Config `yaml:",inline" json:",inline"`

	// Vnode allows binding the collector to a virtual node.
	Vnode string `yaml:"vnode,omitempty" json:"vnode" ui:"group:Connection"`

	// DSN provides a full IBM i ODBC connection string if manual override is needed.
	DSN string `yaml:"dsn" json:"dsn" ui:"group:Connection"`

	// Timeout controls how long to wait for SQL statements and RPCs.
	Timeout confopt.Duration `yaml:"timeout,omitempty" json:"timeout" ui:"group:Connection"`

	// Hostname is the remote IBM i host to monitor.
	Hostname string `yaml:"hostname,omitempty" json:"hostname" ui:"group:Connection"`

	// Port is the TCP port for the IBM i Access ODBC server.
	Port int `yaml:"port,omitempty" json:"port" ui:"group:Connection"`

	// Username supplies the credentials used for authentication.
	Username string `yaml:"username,omitempty" json:"username" ui:"group:Connection"`

	// Password supplies the password used for authentication.
	Password string `yaml:"password,omitempty" json:"password" ui:"group:Connection"`

	// Database selects the IBM i database (library) to use when building the DSN.
	Database string `yaml:"database,omitempty" json:"database" ui:"group:Connection"`

	// ConnectionType selects how the collector connects (currently only "odbc").
	ConnectionType string `yaml:"connection_type,omitempty" json:"connection_type" ui:"group:Connection"`

	// ODBCDriver specifies the driver name registered on the host.
	ODBCDriver string `yaml:"odbc_driver,omitempty" json:"odbc_driver" ui:"group:Connection"`

	// UseSSL enables TLS for the ODBC connection when supported by the driver.
	UseSSL bool `yaml:"use_ssl,omitempty" json:"use_ssl" ui:"group:Connection"`

	// ResetStatistics toggles destructive SQL services that reset system statistics on each query.
	ResetStatistics bool `yaml:"reset_statistics,omitempty" json:"reset_statistics" ui:"group:Advanced"`

	// CollectDiskMetrics toggles collection of disk unit statistics.
	CollectDiskMetrics confopt.AutoBool `yaml:"collect_disk_metrics,omitempty" json:"collect_disk_metrics" ui:"group:Disks"`

	// CollectSubsystemMetrics toggles collection of subsystem activity metrics.
	CollectSubsystemMetrics confopt.AutoBool `yaml:"collect_subsystem_metrics,omitempty" json:"collect_subsystem_metrics" ui:"group:Subsystems"`

	// CollectActiveJobs toggles collection of detailed per-job metrics.
	CollectActiveJobs confopt.AutoBool `yaml:"collect_active_jobs,omitempty" json:"collect_active_jobs" ui:"group:Active Jobs"`

	// CollectHTTPServerMetrics toggles collection of IBM HTTP Server statistics.
	CollectHTTPServerMetrics confopt.AutoBool `yaml:"collect_http_server_metrics,omitempty" json:"collect_http_server_metrics" ui:"group:Other Metrics"`

	// CollectPlanCacheMetrics toggles collection of plan cache analysis metrics.
	CollectPlanCacheMetrics confopt.AutoBool `yaml:"collect_plan_cache_metrics,omitempty" json:"collect_plan_cache_metrics" ui:"group:Other Metrics"`

	// CollectMessageQueueTotals enables expensive aggregate counting across all message queues.
	CollectMessageQueueTotals confopt.AutoBool `yaml:"collect_message_queue_totals,omitempty" json:"collect_message_queue_totals" ui:"group:Queues"`

	// CollectJobQueueTotals enables expensive aggregate counting across all job queues.
	CollectJobQueueTotals confopt.AutoBool `yaml:"collect_job_queue_totals,omitempty" json:"collect_job_queue_totals" ui:"group:Queues"`

	// CollectOutputQueueTotals enables expensive aggregate counting across all output queues.
	CollectOutputQueueTotals confopt.AutoBool `yaml:"collect_output_queue_totals,omitempty" json:"collect_output_queue_totals" ui:"group:Queues"`

	// SlowPath enables the asynchronous slow-path worker for heavy queries.
	SlowPath bool `yaml:"slow_path,omitempty" json:"slow_path" ui:"group:Advanced"`

	// SlowPathUpdateEvery controls the beat interval for the slow-path worker.
	SlowPathUpdateEvery confopt.Duration `yaml:"slow_path_update_every,omitempty" json:"slow_path_update_every" ui:"group:Advanced"`

	// SlowPathMaxConnections caps the number of concurrent queries the slow-path worker may run.
	SlowPathMaxConnections int `yaml:"slow_path_max_connections,omitempty" json:"slow_path_max_connections" ui:"group:Advanced"`

	// BatchPath enables the long-period batch worker for expensive queue aggregates.
	BatchPath bool `yaml:"batch_path,omitempty" json:"batch_path" ui:"group:Advanced"`

	// BatchPathUpdateEvery controls the beat interval for the batch worker.
	BatchPathUpdateEvery confopt.Duration `yaml:"batch_path_update_every,omitempty" json:"batch_path_update_every" ui:"group:Advanced"`

	// BatchPathMaxConnections caps concurrent queries for the batch worker.
	BatchPathMaxConnections int `yaml:"batch_path_max_connections,omitempty" json:"batch_path_max_connections" ui:"group:Advanced"`

	// MaxDisks caps how many disk units may be charted.
	MaxDisks int `yaml:"max_disks,omitempty" json:"max_disks" ui:"group:Disks"`

	// MaxSubsystems caps how many subsystems may be charted.
	MaxSubsystems int `yaml:"max_subsystems,omitempty" json:"max_subsystems" ui:"group:Subsystems"`

	// DiskSelector filters disk units by name using glob-style patterns.
	DiskSelector string `yaml:"collect_disks_matching,omitempty" json:"collect_disks_matching" ui:"group:Disks"`

	// SubsystemSelector filters subsystems by name using glob-style patterns.
	SubsystemSelector string `yaml:"collect_subsystems_matching,omitempty" json:"collect_subsystems_matching" ui:"group:Subsystems"`

	// ActiveJobs lists active jobs to monitor, using fully-qualified job identifiers (JOB_NUMBER/USER/JOB_NAME).
	// When empty, active job collection is disabled.
	ActiveJobs []string `yaml:"active_jobs,omitempty" json:"active_jobs" ui:"group:Active Jobs"`

	// MessageQueues lists message queues to collect, formatted as LIBRARY/QUEUE strings.
	// When empty, message queue collection is disabled. The default configuration monitors
	// QSYS/QSYSOPR, QSYS/QSYSMSG, and QSYS/QHST.
	MessageQueues []string `yaml:"message_queues,omitempty" json:"message_queues" ui:"group:Queues"`

	// JobQueues lists job queues to collect, formatted as LIBRARY/QUEUE strings.
	// When empty, job queue collection is disabled.
	JobQueues []string `yaml:"job_queues,omitempty" json:"job_queues" ui:"group:Queues"`

	// OutputQueues lists output queues to collect, formatted as LIBRARY/QUEUE strings.
	// When empty, output queue collection is disabled.
	OutputQueues []string `yaml:"output_queues,omitempty" json:"output_queues" ui:"group:Queues"`
}
