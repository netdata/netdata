// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts_v2.yaml"
var chartTemplateYAML string

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
	store := metrix.NewCollectorStore()
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
		store:  store,
		charts: &collectorapi.Charts{},
		cache:  newCache(),
	}
}

type Config struct {
	Vnode              string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	web.HTTPConfig     `yaml:",inline" json:""`
	Name               string        `yaml:"name,omitempty" json:"name"`
	Application        string        `yaml:"app,omitempty" json:"app"`
	LabelPrefix        string        `yaml:"label_prefix,omitempty" json:"label_prefix"`
	Selector           selector.Expr `yaml:"selector,omitempty" json:"selector"`
	ExpectedPrefix     string        `yaml:"expected_prefix,omitempty" json:"expected_prefix"`
	MaxTS              int           `yaml:"max_time_series" json:"max_time_series"`
	MaxTSPerMetric     int           `yaml:"max_time_series_per_metric" json:"max_time_series_per_metric"`
	FallbackType       struct {
		Gauge   []string `yaml:"gauge,omitempty" json:"gauge"`
		Counter []string `yaml:"counter,omitempty" json:"counter"`
	} `yaml:"fallback_type,omitempty" json:"fallback_type"`
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	charts *collectorapi.Charts
	store  metrix.CollectorStore

	prom prometheus.Prometheus
	pipe *pipeline
	mw   *metricFamilyWriter

	cache        *cache
	fallbackType struct {
		counter matcher.Matcher
		gauge   matcher.Matcher
	}
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
	c.pipe = newPipeline(prom)
	c.mw = newMetricFamilyWriter(c.store, c)

	m, err := c.initFallbackTypeMatcher(c.FallbackType.Counter)
	if err != nil {
		return fmt.Errorf("init counter fallback type matcher: %v", err)
	}
	c.fallbackType.counter = m

	m, err = c.initFallbackTypeMatcher(c.FallbackType.Gauge)
	if err != nil {
		return fmt.Errorf("init counter fallback type matcher: %v", err)
	}
	c.fallbackType.gauge = m

	return nil
}

func (c *Collector) Check(context.Context) error {
	count, err := c.checkV2()
	if err != nil {
		return err
	}
	if count == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (c *Collector) Charts() *collectorapi.Charts {
	return c.charts
}

func (c *Collector) Collect(ctx context.Context) error { return c.collectV2(ctx) }

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }

func (c *Collector) Cleanup(context.Context) {
	if c.prom != nil && c.prom.HTTPClient() != nil {
		c.prom.HTTPClient().CloseIdleConnections()
	}
}
