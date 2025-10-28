// SPDX-License-Identifier: GPL-3.0-or-later

package coredns

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/blang/semver/v4"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("coredns", module.Creator{
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
					URL: "http://127.0.0.1:9153/metrics",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
				},
			},
		},
		charts:           summaryCharts.Copy(),
		collectedServers: make(map[string]bool),
		collectedZones:   make(map[string]bool),
	}
}

type Config struct {
	Vnode              string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	web.HTTPConfig     `yaml:",inline" json:""`
	PerServerStats     matcher.SimpleExpr `yaml:"per_server_stats,omitempty" json:"per_server_stats"`
	PerZoneStats       matcher.SimpleExpr `yaml:"per_zone_stats,omitempty" json:"per_zone_stats"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	prom prometheus.Prometheus

	charts *Charts

	perServerMatcher matcher.Matcher
	perZoneMatcher   matcher.Matcher
	collectedServers map[string]bool
	collectedZones   map[string]bool
	skipVersionCheck bool
	version          *semver.Version
	metricNames      requestMetricsNames
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	sm, err := c.initPerServerMatcher()
	if err != nil {
		return fmt.Errorf("init per_server_stats: %v", err)
	}
	if sm != nil {
		c.perServerMatcher = sm
	}

	zm, err := c.initPerZoneMatcher()
	if err != nil {
		return fmt.Errorf("init per_zone_stats: %v", err)
	}
	if zm != nil {
		c.perZoneMatcher = zm
	}

	prom, err := c.initPrometheusClient()
	if err != nil {
		return fmt.Errorf("init prometheus client: %v", err)
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

func (c *Collector) Charts() *Charts {
	return c.charts
}

func (c *Collector) Collect(context.Context) map[string]int64 {
	mx, err := c.collect()

	if err != nil {
		c.Error(err)
		return nil
	}

	return mx
}

func (c *Collector) Cleanup(context.Context) {
	if c.prom != nil && c.prom.HTTPClient() != nil {
		c.prom.HTTPClient().CloseIdleConnections()
	}
}
