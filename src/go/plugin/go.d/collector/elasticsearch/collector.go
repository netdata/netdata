// SPDX-License-Identifier: GPL-3.0-or-later

package elasticsearch

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	collectorapi.Register("elasticsearch", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 5,
		},
		Create:        func() collectorapi.CollectorV1 { return New() },
		Config:        func() any { return &Config{} },
		Methods:       elasticsearchMethods,
		MethodHandler: elasticsearchFunctionHandler,
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://127.0.0.1:9200",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 2),
				},
			},
			ClusterMode: false,

			DoNodeStats:     true,
			DoClusterStats:  true,
			DoClusterHealth: true,
			DoIndicesStats:  false,
			Functions: FunctionsConfig{
				TopQueries: TopQueriesConfig{
					Limit: 500,
				},
			},
		},

		charts:                     &collectorapi.Charts{},
		addClusterHealthChartsOnce: &sync.Once{},
		addClusterStatsChartsOnce:  &sync.Once{},
		nodes:                      make(map[string]bool),
		indices:                    make(map[string]bool),
	}
}

type Config struct {
	Vnode              string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	web.HTTPConfig     `yaml:",inline" json:""`
	ClusterMode        bool            `yaml:"cluster_mode" json:"cluster_mode"`
	DoNodeStats        bool            `yaml:"collect_node_stats" json:"collect_node_stats"`
	DoClusterHealth    bool            `yaml:"collect_cluster_health" json:"collect_cluster_health"`
	DoClusterStats     bool            `yaml:"collect_cluster_stats" json:"collect_cluster_stats"`
	DoIndicesStats     bool            `yaml:"collect_indices_stats" json:"collect_indices_stats"`
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

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	charts                     *collectorapi.Charts
	addClusterHealthChartsOnce *sync.Once
	addClusterStatsChartsOnce  *sync.Once

	httpClient *http.Client

	clusterName string
	nodes       map[string]bool
	indices     map[string]bool

	funcRouter *funcRouter
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	err := c.validateConfig()
	if err != nil {
		return fmt.Errorf("check configuration: %v", err)
	}

	httpClient, err := c.initHTTPClient()
	if err != nil {
		return fmt.Errorf("init HTTP client: %v", err)
	}
	c.httpClient = httpClient

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
	if c.funcRouter != nil {
		c.funcRouter.Cleanup(ctx)
	}
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
}
