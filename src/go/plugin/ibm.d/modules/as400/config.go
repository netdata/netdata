package as400

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
)

// Config defines the user-facing configuration for the AS400 collector.
type Config struct {
	framework.Config `yaml:",inline" json:",inline"`

	Vnode         string           `yaml:"vnode,omitempty" json:"vnode"`
	DSN           string           `yaml:"dsn" json:"dsn"`
	Timeout       confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	MaxDbConns    int              `yaml:"max_db_conns,omitempty" json:"max_db_conns"`
	MaxDbLifeTime confopt.Duration `yaml:"max_db_life_time,omitempty" json:"max_db_life_time"`

	Hostname       string `yaml:"hostname,omitempty" json:"hostname"`
	Port           int    `yaml:"port,omitempty" json:"port"`
	Username       string `yaml:"username,omitempty" json:"username"`
	Password       string `yaml:"password,omitempty" json:"password"`
	Database       string `yaml:"database,omitempty" json:"database"`
	ConnectionType string `yaml:"connection_type,omitempty" json:"connection_type"`
	ODBCDriver     string `yaml:"odbc_driver,omitempty" json:"odbc_driver"`
	UseSSL         bool   `yaml:"use_ssl,omitempty" json:"use_ssl"`

	CollectDiskMetrics      *bool `yaml:"collect_disk_metrics,omitempty" json:"collect_disk_metrics"`
	CollectSubsystemMetrics *bool `yaml:"collect_subsystem_metrics,omitempty" json:"collect_subsystem_metrics"`
	CollectJobQueueMetrics  *bool `yaml:"collect_job_queue_metrics,omitempty" json:"collect_job_queue_metrics"`
	CollectActiveJobs       *bool `yaml:"collect_active_jobs,omitempty" json:"collect_active_jobs"`

	MaxDisks      int `yaml:"max_disks,omitempty" json:"max_disks"`
	MaxSubsystems int `yaml:"max_subsystems,omitempty" json:"max_subsystems"`
	MaxJobQueues  int `yaml:"max_job_queues,omitempty" json:"max_job_queues"`
	MaxActiveJobs int `yaml:"max_active_jobs,omitempty" json:"max_active_jobs"`

	DiskSelector      string `yaml:"collect_disks_matching,omitempty" json:"collect_disks_matching"`
	SubsystemSelector string `yaml:"collect_subsystems_matching,omitempty" json:"collect_subsystems_matching"`
	JobQueueSelector  string `yaml:"collect_job_queues_matching,omitempty" json:"collect_job_queues_matching"`
}
