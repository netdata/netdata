// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redis/redisfunc"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/oldmetrix"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	// The collector reports client errors itself; go-redis must not write to stderr.
	redis.SetLogger(noopLogger{})

	collectorapi.Register("redis", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Create:          func() collectorapi.CollectorV1 { return New() },
		Config:          func() any { return &Config{} },
		SharedFunctions: redisfunc.Methods,
		MethodHandler: func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
			c, ok := job.Collector().(*Collector)
			if !ok {
				return nil
			}
			return c.funcRouter
		},
	})
}

type noopLogger struct{}

func (noopLogger) Printf(context.Context, string, ...any) {}

func New() *Collector {
	return &Collector{
		Config: Config{
			Address:     "redis://@localhost:6379",
			Timeout:     confopt.Duration(time.Second),
			PingSamples: 5,
			Functions: redisfunc.FunctionsConfig{
				TopQueries: redisfunc.TopQueriesConfig{
					Limit: redisfunc.DefaultTopQueriesLimit,
				},
			},
		},

		pingSummary:       oldmetrix.NewSummary(),
		collectedCommands: make(map[string]bool),
		collectedDBs:      make(map[string]bool),
		garnetKeyspace: garnetKeyspaceCache{
			refreshEvery: garnetKeyspaceRefreshEvery,
		},
	}
}

type Config struct {
	Vnode              string           `yaml:"vnode,omitempty"               json:"vnode"`
	UpdateEvery        int              `yaml:"update_every,omitempty"        json:"update_every"`
	AutoDetectionRetry int              `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	Address            string           `yaml:"address"                       json:"address"`
	Timeout            confopt.Duration `yaml:"timeout,omitempty"             json:"timeout"`
	Username           string           `yaml:"username,omitempty"            json:"username"`
	Password           string           `yaml:"password,omitempty"            json:"password"`
	tlscfg.TLSConfig   `yaml:",inline" json:""`
	PingSamples        int                       `yaml:"ping_samples"                  json:"ping_samples"`
	Functions          redisfunc.FunctionsConfig `yaml:"functions,omitempty"           json:"functions"`
}

type (
	Collector struct {
		collectorapi.Base
		Config `yaml:",inline" json:""`

		charts               *collectorapi.Charts
		addAOFChartsOnce     sync.Once
		addReplicaChartsOnce sync.Once

		// rdb is replaced by Init and Cleanup, which never overlap collection. Functions run
		// concurrently and read it through currentClient, so replacements hold rdbMu.
		rdbMu sync.RWMutex
		rdb   redisClient

		funcRouter funcapi.MethodHandler

		server            string // set from the first INFO response, see extractServerVersion
		pingSummary       oldmetrix.Summary
		collectedCommands map[string]bool
		collectedDBs      map[string]bool
		garnetKeyspace    garnetKeyspaceCache
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

func (c *Collector) Init(ctx context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	rdb, err := c.initRedisClient(ctx)
	if err != nil {
		return fmt.Errorf("init redis client: %v", err)
	}
	c.setClient(rdb)
	c.charts = redisCharts.Copy()

	funcCfg := c.Functions
	funcCfg.Timeout = c.Timeout
	deps := funcDepsAdapter{
		collector: c,
	}
	c.funcRouter = redisfunc.NewRouter(deps, funcCfg)

	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	mx, err := c.collect(ctx)
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

func (c *Collector) Collect(ctx context.Context) map[string]int64 {
	mx, err := c.collect(ctx)
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
	// The address is not logged: it may embed credentials.
	if err := c.closeClient(); err != nil {
		c.Warningf("cleanup: error on closing redis client: %v", err)
	}
}
