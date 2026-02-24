// SPDX-License-Identifier: GPL-3.0-or-later

package proxysql

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	collectorapi.Register("proxysql", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Create:          func() collectorapi.CollectorV1 { return New() },
		Config:          func() any { return &Config{} },
		Methods:         proxysqlMethods,
		MethodHandler:   proxysqlFunctionHandler,
	})
}

func New() *Collector {
	c := &Collector{
		Config: Config{
			DSN:     "stats:stats@tcp(127.0.0.1:6032)/",
			Timeout: confopt.Duration(time.Second),
			Functions: FunctionsConfig{
				TopQueries: TopQueriesConfig{
					Limit: 500,
				},
			},
		},

		charts: baseCharts.Copy(),
		once:   &sync.Once{},
		cache: &cache{
			commands:   make(map[string]*commandCache),
			users:      make(map[string]*userCache),
			backends:   make(map[string]*backendCache),
			hostgroups: make(map[string]*hostgroupCache),
		},
	}

	c.funcRouter = newFuncRouter(c)

	return c
}

type Config struct {
	Vnode              string           `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int              `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int              `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	DSN                string           `yaml:"dsn" json:"dsn"`
	Timeout            confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
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

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	charts *collectorapi.Charts

	db *sql.DB

	once  *sync.Once
	cache *cache

	funcRouter *funcRouter // function router for method handlers

	queryDigestCols   map[string]bool
	queryDigestColsMu sync.RWMutex
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if c.DSN == "" {
		return errors.New("dsn not set")
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

func (c *Collector) Charts() *collectorapi.Charts {
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
		c.Errorf("cleanup: error on closing the ProxySQL instance [%s]: %v", c.DSN, err)
	}
	c.db = nil
}
