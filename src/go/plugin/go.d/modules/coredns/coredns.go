// SPDX-License-Identifier: GPL-3.0-or-later

package coredns

import (
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"

	"github.com/blang/semver/v4"
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

func New() *CoreDNS {
	return &CoreDNS{
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
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
	PerServerStats matcher.SimpleExpr `yaml:"per_server_stats,omitempty" json:"per_server_stats"`
	PerZoneStats   matcher.SimpleExpr `yaml:"per_zone_stats,omitempty" json:"per_zone_stats"`
}

type CoreDNS struct {
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

func (cd *CoreDNS) Configuration() any {
	return cd.Config
}

func (cd *CoreDNS) Init() error {
	if err := cd.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	sm, err := cd.initPerServerMatcher()
	if err != nil {
		return fmt.Errorf("init per_server_stats: %v", err)
	}
	if sm != nil {
		cd.perServerMatcher = sm
	}

	zm, err := cd.initPerZoneMatcher()
	if err != nil {
		return fmt.Errorf("init per_zone_stats: %v", err)
	}
	if zm != nil {
		cd.perZoneMatcher = zm
	}

	prom, err := cd.initPrometheusClient()
	if err != nil {
		return fmt.Errorf("init prometheus client: %v", err)
	}
	cd.prom = prom

	return nil
}

func (cd *CoreDNS) Check() error {
	mx, err := cd.collect()
	if err != nil {
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")

	}
	return nil
}

func (cd *CoreDNS) Charts() *Charts {
	return cd.charts
}

func (cd *CoreDNS) Collect() map[string]int64 {
	mx, err := cd.collect()

	if err != nil {
		cd.Error(err)
		return nil
	}

	return mx
}

func (cd *CoreDNS) Cleanup() {
	if cd.prom != nil && cd.prom.HTTPClient() != nil {
		cd.prom.HTTPClient().CloseIdleConnections()
	}
}
