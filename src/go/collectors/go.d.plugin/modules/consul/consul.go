// SPDX-License-Identifier: GPL-3.0-or-later

package consul

import (
	_ "embed"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/prometheus"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"

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
	})
}

func New() *Consul {
	return &Consul{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:8500",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second),
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
	web.HTTP    `yaml:",inline" json:""`
	UpdateEvery int    `yaml:"update_every" json:"update_every"`
	ACLToken    string `yaml:"acl_token" json:"acl_token"`
}

type Consul struct {
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

func (c *Consul) Configuration() any {
	return c.Config
}

func (c *Consul) Init() error {
	if err := c.validateConfig(); err != nil {
		c.Errorf("config validation: %v", err)
		return err
	}

	httpClient, err := c.initHTTPClient()
	if err != nil {
		c.Errorf("init HTTP client: %v", err)
		return err
	}
	c.httpClient = httpClient

	prom, err := c.initPrometheusClient(httpClient)
	if err != nil {
		c.Errorf("init Prometheus client: %v", err)
		return err
	}
	c.prom = prom

	return nil
}

func (c *Consul) Check() error {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")

	}
	return nil
}

func (c *Consul) Charts() *module.Charts {
	return c.charts
}

func (c *Consul) Collect() map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (c *Consul) Cleanup() {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
}
