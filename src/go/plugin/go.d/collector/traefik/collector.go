// SPDX-License-Identifier: GPL-3.0-or-later

package traefik

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("traefik", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://127.0.0.1:8082/metrics",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
				},
			},
		},

		charts:       &module.Charts{},
		checkMetrics: true,
		cache: &cache{
			entrypoints: make(map[string]*cacheEntrypoint),
		},
	}
}

type Config struct {
	Vnode              string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	web.HTTPConfig     `yaml:",inline" json:""`
}

type (
	Collector struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		prom prometheus.Prometheus

		checkMetrics bool
		cache        *cache
	}
	cache struct {
		entrypoints map[string]*cacheEntrypoint
	}
	cacheEntrypoint struct {
		name, proto     string
		requests        *module.Chart
		reqDur          *module.Chart
		reqDurData      map[string]cacheEntrypointReqDur
		openConn        *module.Chart
		openConnMethods map[string]bool
	}
	cacheEntrypointReqDur struct {
		prev, cur struct{ reqs, secs float64 }
		seen      bool
	}
)

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	prom, err := c.initPrometheusClient()
	if err != nil {
		return fmt.Errorf("prometheus client initialization: %v", err)
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
		return nil
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (c *Collector) Cleanup(context.Context) {}
