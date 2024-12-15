// SPDX-License-Identifier: GPL-3.0-or-later

package consul

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"

	"github.com/blang/semver/v4"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("consul", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 1,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://127.0.0.1:8500",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
				},
			},
		},
		charts:                       &module.Charts{},
		addGlobalChartsOnce:          &sync.Once{},
		addServerAutopilotChartsOnce: &sync.Once{},
		checks:                       make(map[string]bool),
	}
}

type Config struct {
	Vnode          string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery    int    `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
	ACLToken       string `yaml:"acl_token,omitempty" json:"acl_token"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts                       *module.Charts
	addGlobalChartsOnce          *sync.Once
	addServerAutopilotChartsOnce *sync.Once

	httpClient *http.Client
	prom       prometheus.Prometheus

	cfg               *consulConfig
	version           *semver.Version
	hasLeaderCharts   bool
	hasFollowerCharts bool
	checks            map[string]bool
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	httpClient, err := c.initHTTPClient()
	if err != nil {
		return fmt.Errorf("init HTTP client: %v", err)
	}
	c.httpClient = httpClient

	prom, err := c.initPrometheusClient(httpClient)
	if err != nil {
		return fmt.Errorf("init Prometheus client: %v", err)
	}
	c.prom = prom

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

func (c *Collector) Charts() *module.Charts {
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

func (c *Collector) Cleanup(context.Context) {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
}
