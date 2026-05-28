// SPDX-License-Identifier: GPL-3.0-or-later

package yugabytedb

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	collectorapi.Register("yugabytedb", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 5,
		},
		Methods:       yugabyteMethods,
		MethodHandler: yugabyteFunctionHandler,
		Create:        func() collectorapi.CollectorV1 { return New() },
		Config:        func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://127.0.0.1:7000/prometheus-metrics",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
				},
			},
			Functions: FunctionsConfig{
				TopQueries: TopQueriesConfig{
					Limit: 500,
				},
				RunningQueries: RunningQueriesConfig{
					Limit: 500,
				},
			},
		},
		charts: &collectorapi.Charts{},

		cache: make(map[string]map[string]bool),
	}
}

type Config struct {
	Vnode              string          `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int             `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int             `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	Functions          FunctionsConfig `yaml:"functions,omitempty" json:"functions"`
	web.HTTPConfig     `yaml:",inline" json:""`
}

type FunctionsConfig struct {
	DSN            string               `yaml:"dsn,omitempty" json:"dsn,omitempty"`
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

	charts *collectorapi.Charts

	httpClient *http.Client
	prom       prometheus.Prometheus

	srvType string

	cache map[string]map[string]bool

	funcRouter *funcRouter
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if c.URL == "" {
		return errors.New("yugabytedb URL required but not set")
	}

	httpClient, err := web.NewHTTPClient(c.ClientConfig)
	if err != nil {
		return fmt.Errorf("init HTTP client: %v", err)
	}
	c.httpClient = httpClient

	prom, err := c.initPrometheusClient(httpClient)
	if err != nil {
		return fmt.Errorf("init Prometheus client: %v", err)
	}
	c.prom = prom

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
		return nil
	}
	return mx
}

func (c *Collector) Cleanup(ctx context.Context) {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
	if c.funcRouter != nil {
		c.funcRouter.Cleanup(ctx)
	}
}
