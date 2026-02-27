// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/blang/semver/v4"
	"github.com/redis/go-redis/v9"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/oldmetrix"
)

//go:embed "config_schema.json"
var configSchema string

type noopLogger struct{}

func (noopLogger) Printf(context.Context, string, ...any) {}

func init() {
	redis.SetLogger(noopLogger{})

	collectorapi.Register("redis", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Create:          func() collectorapi.CollectorV1 { return New() },
		Config:          func() any { return &Config{} },
		Methods:         redisMethods,
		MethodHandler:   redisFunctionHandler,
	})
}

func New() *Collector {
	c := &Collector{
		Config: Config{
			Address:     "redis://@localhost:6379",
			Timeout:     confopt.Duration(time.Second),
			PingSamples: 5,
			Functions: FunctionsConfig{
				TopQueries: TopQueriesConfig{
					Limit: 500,
				},
			},
		},

		addAOFChartsOnce:       &sync.Once{},
		addReplSlaveChartsOnce: &sync.Once{},
		pingSummary:            oldmetrix.NewSummary(),
		collectedCommands:      make(map[string]bool),
		collectedDbs:           make(map[string]bool),
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
	tlscfg.TLSConfig   `yaml:",inline" json:""`
	PingSamples        int             `yaml:"ping_samples" json:"ping_samples"`
	Functions          FunctionsConfig `yaml:"functions,omitempty" json:"functions"`
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
		collectorapi.Base
		Config `yaml:",inline" json:""`

		charts                 *collectorapi.Charts
		addAOFChartsOnce       *sync.Once
		addReplSlaveChartsOnce *sync.Once

		rdb redisClient

		funcRouter *funcRouter

		server            string
		version           *semver.Version
		pingSummary       oldmetrix.Summary
		collectedCommands map[string]bool
		collectedDbs      map[string]bool
	}
	redisClient interface {
		Info(ctx context.Context, section ...string) *redis.StringCmd
		Ping(context.Context) *redis.StatusCmd
		SlowLogGet(ctx context.Context, num int64) *redis.SlowLogCmd
		Close() error
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

	rdb, err := c.initRedisClient()
	if err != nil {
		return fmt.Errorf("init redis client: %v", err)
	}
	c.rdb = rdb

	charts, err := c.initCharts()
	if err != nil {
		return fmt.Errorf("init charts: %v", err)
	}
	c.charts = charts

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
	if c.rdb == nil {
		return
	}
	err := c.rdb.Close()
	if err != nil {
		c.Warningf("cleanup: error on closing redis client [%s]: %v", c.Address, err)
	}
	c.rdb = nil
}
