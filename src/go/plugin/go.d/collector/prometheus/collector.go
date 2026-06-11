// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	collectorapi.Register("prometheus", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 10,
		},
		CreateV2: func() collectorapi.CollectorV2 { return New() },
		Config:   func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 10),
				},
			},
			MaxTS:          2000,
			MaxTSPerMetric: 200,
		},
		store: metrix.NewCollectorStore(),
	}
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	prom          prometheus.Prometheus
	relabelBlocks []relabelBlock
	store         metrix.CollectorStore
	writer        *metricFamilyWriter
	chartTemplate string
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("validating config: %v", err)
	}

	prom, err := c.initPrometheusClient()
	if err != nil {
		return fmt.Errorf("init prometheus client: %v", err)
	}
	c.prom = prom

	// With no relabeling blocks the scrape keeps the direct, no-buffering Scrape fast
	// path; invalid rules or match patterns fail Init here.
	blocks, err := c.initRelabelBlocks()
	if err != nil {
		return fmt.Errorf("init relabeling: %v", err)
	}
	c.relabelBlocks = blocks

	gaugeFallback, err := c.initFallbackTypeMatcher(c.FallbackType.Gauge)
	if err != nil {
		return fmt.Errorf("init gauge fallback type matcher: %v", err)
	}
	counterFallback, err := c.initFallbackTypeMatcher(c.FallbackType.Counter)
	if err != nil {
		return fmt.Errorf("init counter fallback type matcher: %v", err)
	}

	c.writer = newMetricFamilyWriter(c.store, metricFamilyWriterPolicy{
		maxTSPerMetric:        c.MaxTSPerMetric,
		isFallbackTypeGauge:   gaugeFallback,
		isFallbackTypeCounter: counterFallback,
	}, c.Logger)

	tmpl, err := buildChartTemplate(c.application())
	if err != nil {
		return fmt.Errorf("build chart template: %v", err)
	}
	c.chartTemplate = tmpl

	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	return c.check(ctx)
}

func (c *Collector) Collect(ctx context.Context) error {
	return c.collect(ctx)
}

func (c *Collector) Cleanup(context.Context) {
	if c.prom != nil && c.prom.HTTPClient() != nil {
		c.prom.HTTPClient().CloseIdleConnections()
	}
}

func (c *Collector) MetricStore() metrix.CollectorStore {
	return c.store
}

func (c *Collector) ChartTemplateYAML() string {
	return c.chartTemplate
}
