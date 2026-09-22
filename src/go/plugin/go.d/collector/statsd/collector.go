// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"context"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const defaultChartExpiry = 5

// The package is deliberately unregistered until receiver/profile and operator
// delivery stages are complete. These resource values remain provisional.
func New() *Collector {
	return &Collector{
		Config: Config{
			MaxSeries:         1000,
			MetricIdleTimeout: confopt.Duration(5 * time.Minute),
		},
		store: metrix.NewCollectorStore(),
		now:   time.Now,
	}
}

type Config struct {
	UpdateEvery       int              `yaml:"update_every,omitempty" json:"update_every"`
	MaxSeries         int              `yaml:"max_series"             json:"max_series"`
	MetricIdleTimeout confopt.Duration `yaml:"metric_idle_timeout"    json:"metric_idle_timeout"`
}

type Collector struct {
	collectorapi.Base
	Config    `yaml:",inline" json:""`
	store     metrix.CollectorStore
	receiver  *receiver
	templates *chartengine.TemplateSet
	now       func() time.Time
	scratch   []percentileBin // Collect-owned, shared across all interval queries.
}

var _ collectorapi.CollectorV2 = (*Collector)(nil)
var _ collectorapi.ChartTemplateSetProvider = (*Collector)(nil)

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) Init(context.Context) error {
	if c.MaxSeries <= 0 {
		return errors.New("max_series must be positive")
	}
	if c.MetricIdleTimeout < 0 {
		return errors.New("metric_idle_timeout must be nonnegative")
	}
	set, err := chartengine.NewTemplateSet(chartengine.TemplateSetSpec{
		Policy: chartengine.EnginePolicy{
			Autogen: &chartengine.AutogenPolicy{
				Enabled:                  true,
				ExpireAfterSuccessCycles: defaultChartExpiry,
			},
		},
		FallbackContextNamespace: "statsd",
	})
	if err != nil {
		return err
	}
	c.templates = set
	c.receiver = newReceiver(
		c.MaxSeries,
		time.Duration(c.MetricIdleTimeout),
		c.store.(metrix.DescriptorRetention),
		defaultChartExpiry,
	)
	return nil
}

func (c *Collector) Check(context.Context) error {
	if c.receiver == nil {
		return errors.New("collector is not initialized")
	}
	return nil
}

func (c *Collector) Collect(ctx context.Context) error { return c.collect(ctx) }

func (c *Collector) Cleanup(context.Context) {
	if c.receiver != nil {
		c.receiver.stop()
	}
}

func (c *Collector) MetricStore() metrix.CollectorStore         { return c.store }
func (c *Collector) ChartTemplateSet() *chartengine.TemplateSet { return c.templates }
