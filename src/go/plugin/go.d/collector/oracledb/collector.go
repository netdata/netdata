// SPDX-License-Identifier: GPL-3.0-or-later

package oracledb

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	collectorapi.Register("oracledb", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Create:          func() collectorapi.CollectorV1 { return New() },
		Config:          func() any { return &Config{} },
		Methods:         oracledbMethods,
		MethodHandler:   oracledbFunctionHandler,
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			Timeout: confopt.Duration(time.Second * 2),
			Functions: FunctionsConfig{
				TopQueries: TopQueriesConfig{
					Limit: 500,
				},
				RunningQueries: RunningQueriesConfig{
					Limit: 500,
				},
			},
			charts:          globalCharts.Copy(),
			seenTablespaces: make(map[string]bool),
			seenWaitClasses: make(map[string]bool),
		},
	}
}

type Config struct {
	Vnode              string           `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int              `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int              `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	DSN                string           `json:"dsn" yaml:"dsn"`
	Timeout            confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	Functions          FunctionsConfig  `yaml:"functions,omitempty" json:"functions"`

	charts *collectorapi.Charts

	publicDSN string // with hidden username/password

	seenTablespaces map[string]bool
	seenWaitClasses map[string]bool
}

type FunctionsConfig struct {
	TopQueries     TopQueriesConfig     `yaml:"top_queries,omitempty" json:"top_queries"`
	RunningQueries RunningQueriesConfig `yaml:"running_queries,omitempty" json:"running_queries"`
}

type TopQueriesConfig struct {
	Disabled bool             `yaml:"disabled" json:"disabled"`
	Timeout  confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	Limit    int              `yaml:"limit,omitempty" json:"limit"`
}

type RunningQueriesConfig struct {
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

func (c Config) runningQueriesTimeout() time.Duration {
	if c.Functions.RunningQueries.Timeout == 0 {
		return c.Timeout.Duration()
	}
	return c.Functions.RunningQueries.Timeout.Duration()
}

func (c Config) runningQueriesLimit() int {
	if c.Functions.RunningQueries.Limit <= 0 {
		return 500
	}
	return c.Functions.RunningQueries.Limit
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	db *sql.DB

	funcRouter *funcRouter
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	dsn, err := c.validateDSN()
	if err != nil {
		return fmt.Errorf("invalid oracle DSN: %w", err)
	}

	c.publicDSN = dsn

	c.funcRouter = newFuncRouter(c)

	return nil
}

func (c *Collector) Check(context.Context) error {
	mx, err := c.collect()
	if err != nil {
		return fmt.Errorf("failed to collect metrics [%s]: %w", c.publicDSN, err)
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
		c.Error(fmt.Sprintf("failed to collect metrics [%s]: %s", c.publicDSN, err))
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
	if c.db != nil {
		if err := c.db.Close(); err != nil {
			c.Errorf("cleanup: error on closing connection [%s]: %v", c.publicDSN, err)
		}
		c.db = nil
	}
}
