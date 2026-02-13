// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/oldmetrix"

	"github.com/jackc/pgx/v5/stdlib"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("postgres", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
		Methods:         pgMethods,
		MethodHandler:   pgFunctionHandler,
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			Timeout:            confopt.Duration(time.Second * 2),
			DSN:                "postgres://postgres:postgres@127.0.0.1:5432/postgres",
			XactTimeHistogram:  []float64{.1, .5, 1, 2.5, 5, 10},
			QueryTimeHistogram: []float64{.1, .5, 1, 2.5, 5, 10},
			// charts: 20 x table, 4 x index.
			// https://discord.com/channels/847502280503590932/1022693928874549368
			MaxDBTables:  50,
			MaxDBIndexes: 250,
			Functions: FunctionsConfig{
				TopQueries: TopQueriesConfig{
					Limit: 500,
				},
			},
		},
		charts:  baseCharts.Copy(),
		dbConns: make(map[string]*dbConn),
		mx: &pgMetrics{
			dbs:       make(map[string]*dbMetrics),
			indexes:   make(map[string]*indexMetrics),
			tables:    make(map[string]*tableMetrics),
			replApps:  make(map[string]*replStandbyAppMetrics),
			replSlots: make(map[string]*replSlotMetrics),
		},
		recheckSettingsEvery:              time.Minute * 30,
		doSlowEvery:                       time.Minute * 5,
		addXactQueryRunningTimeChartsOnce: &sync.Once{},
		addWALFilesChartsOnce:             &sync.Once{},
	}
}

type Config struct {
	Vnode              string           `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int              `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int              `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	DSN                string           `yaml:"dsn" json:"dsn"`
	Timeout            confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	DBSelector         string           `yaml:"collect_databases_matching,omitempty" json:"collect_databases_matching"`
	XactTimeHistogram  []float64        `yaml:"transaction_time_histogram,omitempty" json:"transaction_time_histogram"`
	QueryTimeHistogram []float64        `yaml:"query_time_histogram,omitempty" json:"query_time_histogram"`
	MaxDBTables        int64            `yaml:"max_db_tables" json:"max_db_tables"`
	MaxDBIndexes       int64            `yaml:"max_db_indexes" json:"max_db_indexes"`
	Functions          FunctionsConfig  `yaml:"functions,omitempty" json:"functions"`
}

type FunctionsConfig struct {
	TopQueries TopQueriesConfig `yaml:"top_queries,omitempty" json:"top_queries"`
}

type TopQueriesConfig struct {
	Disabled bool             `yaml:"disabled" json:"disabled"`
	Timeout  confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	Limit    int              `yaml:"limit,omitempty" json:"limit"`
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

type (
	Collector struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts                            *module.Charts
		addXactQueryRunningTimeChartsOnce *sync.Once
		addWALFilesChartsOnce             *sync.Once

		db      *sql.DB
		dbConns map[string]*dbConn

		superUser               *bool
		pgIsInRecovery          *bool
		pgVersion               int
		pgStatStatementsAvail   bool            // cached positive result only
		pgStatStatementsColumns map[string]bool // cached column names from pg_stat_statements
		pgStatMonitorAvail      bool            // cached positive result only
		pgStatMonitorColumns    map[string]bool // cached column names from pg_stat_monitor
		queryStatsSource        string          // "pg_stat_monitor" or "pg_stat_statements" (auto-detected)
		pgStatStatementsMu      sync.RWMutex    // protects pgStatStatements*/pgStatMonitor* fields for concurrent access
		dbSr                    matcher.Matcher
		recheckSettingsTime     time.Time
		recheckSettingsEvery    time.Duration
		doSlowTime              time.Time
		doSlowEvery             time.Duration

		mx *pgMetrics

		funcRouter *funcRouter
	}
	dbConn struct {
		db         *sql.DB
		connStr    string
		connErrors int
	}
)

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	err := c.validateConfig()
	if err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	sr, err := c.initDBSelector()
	if err != nil {
		return fmt.Errorf("config validation: %v", err)
	}
	c.dbSr = sr

	c.mx.xactTimeHist = oldmetrix.NewHistogramWithRangeBuckets(c.XactTimeHistogram)
	c.mx.queryTimeHist = oldmetrix.NewHistogramWithRangeBuckets(c.QueryTimeHistogram)

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

func (c *Collector) Charts() *module.Charts {
	return c.charts
}

func (c *Collector) Collect(context.Context) map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(mx) == 0 {
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
		c.Warningf("cleanup: error on closing the Postgres database [%s]: %v", c.DSN, err)
	}
	c.db = nil

	for dbname, conn := range c.dbConns {
		delete(c.dbConns, dbname)
		if conn.connStr != "" {
			stdlib.UnregisterConnConfig(conn.connStr)
		}
		if conn.db != nil {
			_ = conn.db.Close()
		}
	}
}
