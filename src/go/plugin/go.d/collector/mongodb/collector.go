// SPDX-License-Identifier: GPL-3.0-or-later

package mongo

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	collectorapi.Register("mongodb", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Create:          func() collectorapi.CollectorV1 { return New() },
		Config:          func() any { return &Config{} },
		Methods:         mongoMethods,
		MethodHandler:   mongoFunctionHandler,
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			URI:     "mongodb://localhost:27017",
			Timeout: confopt.Duration(time.Second),
			Databases: matcher.SimpleExpr{
				Includes: []string{},
				Excludes: []string{},
			},
			Functions: FunctionsConfig{
				TopQueries: TopQueriesConfig{
					Limit: 500,
				},
			},
		},

		conn: &mongoClient{},

		charts:                chartsServerStatus.Copy(),
		addShardingChartsOnce: &sync.Once{},

		optionalCharts: make(map[string]bool),
		replSetMembers: make(map[string]bool),
		databases:      make(map[string]bool),
		shards:         make(map[string]bool),
	}
}

type Config struct {
	Vnode              string             `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int                `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int                `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	URI                string             `yaml:"uri" json:"uri"`
	Timeout            confopt.Duration   `yaml:"timeout,omitempty" json:"timeout"`
	Databases          matcher.SimpleExpr `yaml:"databases,omitempty" json:"databases"`
	Functions          FunctionsConfig    `yaml:"functions,omitempty" json:"functions"`
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

	charts                *collectorapi.Charts
	addShardingChartsOnce *sync.Once

	conn mongoConn

	dbSelector     matcher.Matcher
	optionalCharts map[string]bool
	databases      map[string]bool
	replSetMembers map[string]bool
	shards         map[string]bool

	// Top queries column cache with double-checked locking
	topQueriesColsMu sync.RWMutex
	topQueriesCols   map[string]bool

	funcRouter *funcRouter
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.verifyConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	if err := c.initDatabaseSelector(); err != nil {
		return fmt.Errorf("init database selector: %v", err)
	}

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
	}

	if len(mx) == 0 {
		c.Warning("no values collected")
		return nil
	}

	return mx
}

func (c *Collector) Cleanup(ctx context.Context) {
	if c.funcRouter != nil {
		c.funcRouter.Cleanup(ctx)
	}
	if c.conn == nil {
		return
	}
	if err := c.conn.close(); err != nil {
		c.Warningf("cleanup: error on closing mongo conn: %v", err)
	}
}
