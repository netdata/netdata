// SPDX-License-Identifier: GPL-3.0-or-later

package coredns

import (
	_ "embed"
	"time"

	"github.com/blang/semver/v4"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/matcher"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/prometheus"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
)

const (
	defaultURL         = "http://127.0.0.1:9153/metrics"
	defaultHTTPTimeout = time.Second * 2
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("coredns", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

// New creates CoreDNS with default values.
func New() *CoreDNS {
	config := Config{
		HTTP: web.HTTP{
			Request: web.Request{
				URL: defaultURL,
			},
			Client: web.Client{
				Timeout: web.Duration{Duration: defaultHTTPTimeout},
			},
		},
	}
	return &CoreDNS{
		Config:           config,
		charts:           summaryCharts.Copy(),
		collectedServers: make(map[string]bool),
		collectedZones:   make(map[string]bool),
	}
}

// Config is the CoreDNS module configuration.
type Config struct {
	web.HTTP       `yaml:",inline"`
	PerServerStats matcher.SimpleExpr `yaml:"per_server_stats"`
	PerZoneStats   matcher.SimpleExpr `yaml:"per_zone_stats"`
}

// CoreDNS CoreDNS module.
type CoreDNS struct {
	module.Base
	Config           `yaml:",inline"`
	charts           *Charts
	prom             prometheus.Prometheus
	perServerMatcher matcher.Matcher
	perZoneMatcher   matcher.Matcher
	collectedServers map[string]bool
	collectedZones   map[string]bool
	skipVersionCheck bool
	version          *semver.Version
	metricNames      requestMetricsNames
}

// Cleanup makes cleanup.
func (CoreDNS) Cleanup() {}

// Init makes initialization.
func (cd *CoreDNS) Init() bool {
	if cd.URL == "" {
		cd.Error("URL not set")
		return false
	}

	if !cd.PerServerStats.Empty() {
		m, err := cd.PerServerStats.Parse()
		if err != nil {
			cd.Errorf("error on creating 'per_server_stats' matcher : %v", err)
			return false
		}
		cd.perServerMatcher = matcher.WithCache(m)
	}

	if !cd.PerZoneStats.Empty() {
		m, err := cd.PerZoneStats.Parse()
		if err != nil {
			cd.Errorf("error on creating 'per_zone_stats' matcher : %v", err)
			return false
		}
		cd.perZoneMatcher = matcher.WithCache(m)
	}

	client, err := web.NewHTTPClient(cd.Client)
	if err != nil {
		cd.Errorf("error on creating http client : %v", err)
		return false
	}

	cd.prom = prometheus.New(client, cd.Request)

	return true
}

// Check makes check.
func (cd *CoreDNS) Check() bool {
	return len(cd.Collect()) > 0
}

// Charts creates Charts.
func (cd *CoreDNS) Charts() *Charts {
	return cd.charts
}

// Collect collects metrics.
func (cd *CoreDNS) Collect() map[string]int64 {
	mx, err := cd.collect()

	if err != nil {
		cd.Error(err)
		return nil
	}

	return mx
}
