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
		Create:       func() module.Module { return New() },
		Config:       func() any { return &Config{} },
		Methods:      mssqlMethods,
		HandleMethod: mssqlHandleMethod,
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			DSN:     "sqlserver://localhost:1433",
			Timeout: confopt.Duration(time.Second * 5),
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

	// QueryStoreTimeWindowDays controls how far back to look in Query Store
	// Uses pointer to distinguish "unset" from explicit "0":
	//   - nil (unset): Apply default of 7 days
	//   - 0: Query ALL available data (not recommended for busy servers)
	//   - N > 0: Query last N days
	QueryStoreTimeWindowDays *int `yaml:"query_store_time_window_days,omitempty" json:"query_store_time_window_days"`

	// QueryStoreFunctionEnabled controls whether the top-queries function is available
	// Uses pointer to distinguish "unset" from explicit "false":
	//   - nil (unset): Apply default of false (disabled)
	//   - false: Explicitly disabled
	//   - true: Explicitly enabled (user accepts PII risk)
	// Default: false (opt-in) - MSSQL Query Store may contain unmasked PII in query text
	QueryStoreFunctionEnabled *bool `yaml:"query_store_function_enabled,omitempty" json:"query_store_function_enabled"`
}

// GetQueryStoreTimeWindowDays returns the time window for Query Store queries (default: 7)
func (c *Config) GetQueryStoreTimeWindowDays() int {
	if c.QueryStoreTimeWindowDays == nil {
		return 7
	}
	return *c.QueryStoreTimeWindowDays
}

// GetQueryStoreFunctionEnabled returns whether the Query Store function is enabled (default: false)
func (c *Config) GetQueryStoreFunctionEnabled() bool {
	if c.QueryStoreFunctionEnabled == nil {
		return false
	}
	return *c.QueryStoreFunctionEnabled
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
