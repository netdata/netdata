// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	_ "github.com/microsoft/go-mssqldb"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("mssql", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			DSN:     "sqlserver://localhost:1433",
			Timeout: confopt.Duration(time.Second * 5),

			CollectTransactions:     true,
			CollectWaits:            true,
			CollectLocks:            true,
			CollectJobs:             true,
			CollectBufferStats:      true,
			CollectDatabaseSize:     true,
			CollectUserConnections:  true,
			CollectBlockedProcesses: true,
			CollectSQLErrors:        true,
			CollectDatabaseStatus:   true,
			CollectReplication:      true,
		},

		charts: instanceCharts.Copy(),

		seenDatabases:      make(map[string]bool),
		seenWaitTypes:      make(map[string]bool),
		seenLockTypes:      make(map[string]bool),
		seenLockStatsTypes: make(map[string]bool),
		seenJobs:           make(map[string]bool),
		seenReplications:   make(map[string]bool),
	}
}

type Config struct {
	Vnode       string           `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	DSN         string           `yaml:"dsn" json:"dsn"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`

	CollectTransactions     bool `yaml:"collect_transactions,omitempty" json:"collect_transactions"`
	CollectWaits            bool `yaml:"collect_waits,omitempty" json:"collect_waits"`
	CollectLocks            bool `yaml:"collect_locks,omitempty" json:"collect_locks"`
	CollectJobs             bool `yaml:"collect_jobs,omitempty" json:"collect_jobs"`
	CollectBufferStats      bool `yaml:"collect_buffer_stats,omitempty" json:"collect_buffer_stats"`
	CollectDatabaseSize     bool `yaml:"collect_database_size,omitempty" json:"collect_database_size"`
	CollectUserConnections  bool `yaml:"collect_user_connections,omitempty" json:"collect_user_connections"`
	CollectBlockedProcesses bool `yaml:"collect_blocked_processes,omitempty" json:"collect_blocked_processes"`
	CollectSQLErrors        bool `yaml:"collect_sql_errors,omitempty" json:"collect_sql_errors"`
	CollectDatabaseStatus   bool `yaml:"collect_database_status,omitempty" json:"collect_database_status"`
	CollectReplication      bool `yaml:"collect_replication,omitempty" json:"collect_replication"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	db *sql.DB

	version string

	seenDatabases      map[string]bool
	seenWaitTypes      map[string]bool
	seenLockTypes      map[string]bool
	seenLockStatsTypes map[string]bool
	seenJobs           map[string]bool
	seenReplications   map[string]bool
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if c.DSN == "" {
		return errors.New("config: dsn not set")
	}
	c.Debugf("using DSN [%s]", c.DSN)
	return nil
}

func (c *Collector) Check(context.Context) error {
	mx, err := c.collect()
	if err != nil {
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (c *Collector) Charts() *module.Charts {
	return c.charts
}

func (c *Collector) Collect(context.Context) map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
		return nil
	}
	return mx
}

func (c *Collector) Cleanup(context.Context) {
	if c.db == nil {
		return
	}
	if err := c.db.Close(); err != nil {
		c.Errorf("cleanup: error closing database connection: %v", err)
	}
	c.db = nil
}
