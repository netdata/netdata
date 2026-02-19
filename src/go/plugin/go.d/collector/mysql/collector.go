// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/blang/semver/v4"
	"github.com/go-sql-driver/mysql"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var mysqlChartTemplateV2 string

func init() {
	module.Register("mysql", module.Creator{
		JobConfigSchema: configSchema,
		CreateV2:        func() module.ModuleV2 { return New() },
		Config:          func() any { return &Config{} },
		Methods:         mysqlMethods,
		MethodHandler:   mysqlFunctionHandler,
	})
}

func New() *Collector {
	store := metrix.NewCollectorStore()
	mx := newCollectorMetrics(store)

	return &Collector{
		Config: Config{
			DSN:     "root@tcp(localhost:3306)/",
			Timeout: confopt.Duration(time.Second),
			Functions: FunctionsConfig{
				TopQueries: TopQueriesConfig{
					Limit: 500,
				},
			},
		},

		doDisableSessionQueryLog: true,
		doSlaveStatus:            true,
		doUserStatistics:         true,

		recheckGlobalVarsEvery: time.Minute * 10,

		// innodb_log_files_in_group is available in mysql and <mariadb-10.6,
		// otherwise it defaults to 1.
		// see https://mariadb.com/kb/en/innodb-system-variables/#innodb_log_files_in_group
		varInnoDBLogFilesInGroup: 1,
		store:                    store,
		mx:                       mx,
	}
}

type Config struct {
	Vnode              string           `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int              `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int              `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	DSN                string           `yaml:"dsn" json:"dsn"`
	MyCNF              string           `yaml:"my.cnf,omitempty" json:"my.cnf"`
	Timeout            confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	Functions          FunctionsConfig  `yaml:"functions,omitempty" json:"functions"`
}

type FunctionsConfig struct {
	TopQueries   TopQueriesConfig   `yaml:"top_queries,omitempty" json:"top_queries"`
	DeadlockInfo DeadlockInfoConfig `yaml:"deadlock_info,omitempty" json:"deadlock_info"`
	ErrorInfo    ErrorInfoConfig    `yaml:"error_info,omitempty" json:"error_info"`
}

type TopQueriesConfig struct {
	Disabled bool             `yaml:"disabled" json:"disabled"`
	Timeout  confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	Limit    int              `yaml:"limit,omitempty" json:"limit"`
}

type DeadlockInfoConfig struct {
	Disabled bool             `yaml:"disabled" json:"disabled"`
	Timeout  confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type ErrorInfoConfig struct {
	Disabled bool             `yaml:"disabled" json:"disabled"`
	Timeout  confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
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

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	db *sql.DB

	safeDSN   string
	version   *semver.Version
	isMariaDB bool
	isPercona bool

	doDisableSessionQueryLog bool

	doSlaveStatus    bool
	doUserStatistics bool

	recheckGlobalVarsTime    time.Time
	recheckGlobalVarsEvery   time.Duration
	varInnoDBLogFileSize     int64
	varInnoDBLogFilesInGroup int64
	varMaxConns              int64
	varTableOpenCache        int64
	varDisabledStorageEngine string
	varLogBin                string
	varPerformanceSchema     string
	varPerfSchemaMu          sync.RWMutex // protects varPerformanceSchema for concurrent access

	stmtSummaryCols   map[string]bool // cached column names from events_statements_summary_by_digest
	stmtSummaryColsMu sync.RWMutex    // protects stmtSummaryCols for concurrent access

	funcRouter *funcRouter

	store metrix.CollectorStore
	mx    *collectorMetrics
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if c.MyCNF != "" {
		dsn, err := dsnFromFile(c.MyCNF)
		if err != nil {
			return err
		}
		c.DSN = dsn
	}

	if c.DSN == "" {
		return errors.New("config: dsn not set")
	}

	cfg, err := mysql.ParseDSN(c.DSN)
	if err != nil {
		return fmt.Errorf("error on parsing DSN: %v", err)
	}

	cfg.Passwd = strings.Repeat("x", len(cfg.Passwd))
	c.safeDSN = cfg.FormatDSN()

	c.Debugf("using DSN [%s]", c.DSN)

	c.funcRouter = newFuncRouter(c)

	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	return c.checkMandatory(ctx)
}

func (c *Collector) Collect(ctx context.Context) error {
	return c.collect(ctx)
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return mysqlChartTemplateV2 }

func (c *Collector) Cleanup(ctx context.Context) {
	if c.funcRouter != nil {
		c.funcRouter.Cleanup(ctx)
	}
	if c.db == nil {
		return
	}
	if err := c.db.Close(); err != nil {
		c.Errorf("cleanup: error on closing the mysql database [%s]: %v", c.safeDSN, err)
	}
	c.db = nil
}
