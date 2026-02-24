// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

	_ "github.com/microsoft/go-mssqldb"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	collectorapi.Register("mssql", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 10,
		},
		Create:        func() collectorapi.CollectorV1 { return New() },
		Config:        func() any { return &Config{} },
		Methods:       mssqlMethods,
		MethodHandler: mssqlFunctionHandler,
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			DSN:     "sqlserver://localhost:1433",
			Timeout: confopt.Duration(time.Second * 5),
			Functions: FunctionsConfig{
				TopQueries: TopQueriesConfig{
					Limit:          500,
					TimeWindowDays: 7,
				},
			},
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
	Functions   FunctionsConfig  `yaml:"functions,omitempty" json:"functions"`
}

type FunctionsConfig struct {
	TopQueries   TopQueriesConfig   `yaml:"top_queries,omitempty" json:"top_queries"`
	DeadlockInfo DeadlockInfoConfig `yaml:"deadlock_info,omitempty" json:"deadlock_info"`
	ErrorInfo    ErrorInfoConfig    `yaml:"error_info,omitempty" json:"error_info"`
}

type TopQueriesConfig struct {
	Disabled       bool             `yaml:"disabled" json:"disabled"`
	Timeout        confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	Limit          int              `yaml:"limit,omitempty" json:"limit"`
	TimeWindowDays int              `yaml:"time_window_days,omitempty" json:"time_window_days"`
}

type DeadlockInfoConfig struct {
	Disabled      bool             `yaml:"disabled" json:"disabled"`
	Timeout       confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	UseRingBuffer bool             `yaml:"use_ring_buffer" json:"use_ring_buffer"`
}

type ErrorInfoConfig struct {
	Disabled      bool             `yaml:"disabled" json:"disabled"`
	Timeout       confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	SessionName   string           `yaml:"session_name,omitempty" json:"session_name,omitempty"`
	UseRingBuffer bool             `yaml:"use_ring_buffer" json:"use_ring_buffer"`
}

func (c Config) topQueriesTimeout() time.Duration {
	if c.Functions.TopQueries.Timeout == 0 {
		return c.Timeout.Duration()
	}
	return c.Functions.TopQueries.Timeout.Duration()
}

func (c Config) topQueriesLimit() int {
	if c.Functions.TopQueries.Limit <= 0 {
		return 500
	}
	return c.Functions.TopQueries.Limit
}

func (c Config) topQueriesTimeWindowDays() int {
	if c.Functions.TopQueries.TimeWindowDays == -1 {
		return 0 // -1 means "query all history"
	}
	if c.Functions.TopQueries.TimeWindowDays <= 0 {
		return 7
	}
	return c.Functions.TopQueries.TimeWindowDays
}

func (c Config) deadlockInfoTimeout() time.Duration {
	if c.Functions.DeadlockInfo.Timeout == 0 {
		return c.Timeout.Duration()
	}
	return c.Functions.DeadlockInfo.Timeout.Duration()
}

func (c Config) errorInfoTimeout() time.Duration {
	if c.Functions.ErrorInfo.Timeout == 0 {
		return c.Timeout.Duration()
	}
	return c.Functions.ErrorInfo.Timeout.Duration()
}

func (c Config) errorInfoSessionName() string {
	if strings.TrimSpace(c.Functions.ErrorInfo.SessionName) == "" {
		return "netdata_errors"
	}
	return c.Functions.ErrorInfo.SessionName
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	charts *collectorapi.Charts

	db *sql.DB

	version string

	seenDatabases      map[string]bool
	seenWaitTypes      map[string]bool
	seenLockTypes      map[string]bool
	seenLockStatsTypes map[string]bool
	seenJobs           map[string]bool
	seenReplications   map[string]bool

	// Query Store column cache (per-instance to handle different SQL Server versions)
	queryStoreColsMu sync.RWMutex // protects queryStoreCols for concurrent access
	queryStoreCols   map[string]bool

	funcRouter *funcRouter
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if c.DSN == "" {
		return errors.New("config: dsn not set")
	}
	c.Debugf("using DSN [%s]", c.DSN)

	c.funcRouter = newFuncRouter(c)

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

func (c *Collector) Charts() *collectorapi.Charts {
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

func (c *Collector) Cleanup(ctx context.Context) {
	if c.funcRouter != nil {
		c.funcRouter.Cleanup(ctx)
	}
	if c.db == nil {
		return
	}
	if err := c.db.Close(); err != nil {
		c.Errorf("cleanup: error closing database connection: %v", err)
	}
	c.db = nil
}
