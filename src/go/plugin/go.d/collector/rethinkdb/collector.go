// SPDX-License-Identifier: GPL-3.0-or-later

package rethinkdb

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	collectorapi.Register("rethinkdb", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Create:          func() collectorapi.CollectorV1 { return New() },
		Config:          func() any { return &Config{} },
		Methods:         rethinkdbMethods,
		MethodHandler:   rethinkdbFunctionHandler,
	})
}

func New() *Collector {
	c := &Collector{
		Config: Config{
			Address: "127.0.0.1:28015",
			Timeout: confopt.Duration(time.Second * 1),
			Functions: FunctionsConfig{
				RunningQueries: RunningQueriesConfig{
					Limit: 500,
				},
			},
		},

		charts:      clusterCharts.Copy(),
		newConn:     newRethinkdbConn,
		seenServers: make(map[string]bool),
	}

	c.funcRouter = newFuncRouter(c)

	return c
}

type Config struct {
	Vnode              string           `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int              `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int              `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	Address            string           `yaml:"address" json:"address"`
	Timeout            confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	Username           string           `yaml:"username,omitempty" json:"username"`
	Password           string           `yaml:"password,omitempty" json:"password"`
	Functions          FunctionsConfig  `yaml:"functions,omitempty" json:"functions"`
}

type FunctionsConfig struct {
	RunningQueries RunningQueriesConfig `yaml:"running_queries,omitempty" json:"running_queries"`
}

type RunningQueriesConfig struct {
	Disabled bool             `yaml:"disabled" json:"disabled"`
	Timeout  confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	Limit    int              `yaml:"limit,omitempty" json:"limit"`
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

	charts *collectorapi.Charts

	newConn func(cfg Config) (rdbConn, error)
	rdb     rdbConn

	funcRouter *funcRouter // function router for method handlers

	seenServers map[string]bool
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if c.Address == "" {
		return errors.New("config: address is not set")
	}
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
	ms, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(ms) == 0 {
		return nil
	}
	return ms
}

func (c *Collector) Cleanup(ctx context.Context) {
	if c.funcRouter != nil {
		c.funcRouter.Cleanup(ctx)
	}
	if c.rdb != nil {
		if err := c.rdb.close(); err != nil {
			c.Warningf("cleanup: error on closing client [%s]: %v", c.Address, err)
		}
		c.rdb = nil
	}
}
