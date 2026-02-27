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
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/mysql/mysqlfunc"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var mysqlChartTemplateV2 string

func init() {
	collectorapi.Register("mysql", collectorapi.Creator{
		JobConfigSchema: configSchema,
		CreateV2:        func() collectorapi.CollectorV2 { return New() },
		Config:          func() any { return &Config{} },
		Methods:         mysqlfunc.Methods,
		MethodHandler: func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
			c, ok := job.Collector().(*Collector)
			if !ok {
				return nil
			}
			return c.funcRouter
		},
	})
}

func New() *Collector {
	store := metrix.NewCollectorStore()
	mx := newCollectorMetrics(store)

	return &Collector{
		Config: Config{
			DSN:     "root@tcp(localhost:3306)/",
			Timeout: confopt.Duration(time.Second),
			Functions: mysqlfunc.FunctionsConfig{
				TopQueries: mysqlfunc.TopQueriesConfig{
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
	Vnode              string                    `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int                       `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int                       `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	DSN                string                    `yaml:"dsn" json:"dsn"`
	MyCNF              string                    `yaml:"my.cnf,omitempty" json:"my.cnf"`
	Timeout            confopt.Duration          `yaml:"timeout,omitempty" json:"timeout"`
	Functions          mysqlfunc.FunctionsConfig `yaml:"functions,omitempty" json:"functions"`
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	dbMu sync.RWMutex // protects db pointer lifecycle across collect/functions/cleanup
	db   *sql.DB

	safeDSN string
	version *semver.Version

	doDisableSessionQueryLog bool
	doSlaveStatus            bool
	doUserStatistics         bool

	isMariaDB bool
	isPercona bool

	galeraDetected bool
	qcacheDetected bool

	recheckGlobalVarsTime  time.Time
	recheckGlobalVarsEvery time.Duration

	varDisabledStorageEngine string
	varLogBin                string
	varInnoDBLogFileSize     int64
	varInnoDBLogFilesInGroup int64
	varMaxConns              int64
	varTableOpenCache        int64
	varPerformanceSchema     string

	funcRouter funcapi.MethodHandler

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

	c.Debugf("using DSN [%s]", c.safeDSN)

	funcCfg := c.Functions
	funcCfg.Timeout = c.Timeout
	c.funcRouter = mysqlfunc.NewRouter(funcDepsAdapter{collector: c}, c.Logger, funcCfg)

	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	return c.check(ctx)
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
	if err := c.closeDB(); err != nil {
		c.Errorf("cleanup: error on closing the mysql database [%s]: %v", c.safeDSN, err)
	}
}
