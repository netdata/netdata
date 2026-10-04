// SPDX-License-Identifier: GPL-3.0-or-later

package rum

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/agg"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/otlp"
	"github.com/netdata/netdata/go/plugins/plugin/dem/runtimehub"
	"github.com/netdata/netdata/go/plugins/plugin/dem/secrets"
	"github.com/netdata/netdata/go/plugins/plugin/dem/store"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed charts.yaml
var charts string

//go:embed config_schema.json
var schema string

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Window      confopt.Duration `yaml:"window"                 json:"window"`
	config.Site `yaml:",inline" json:""`
	OTLP        otlp.Config `yaml:"otlp"                   json:"otlp"`
}
type Dependencies struct {
	Hub     *runtimehub.Hub
	History *store.Store
}
type Collector struct {
	collectorapi.Base
	Config     `yaml:",inline" json:""`
	deps       Dependencies
	store      metrix.CollectorStore
	aggregator *agg.Aggregator
	redactor   *secrets.Redactor
	metrics    collectorMetrics
}

func Creator(deps Dependencies) collectorapi.Creator {
	return collectorapi.Creator{
		Defaults: collectorapi.Defaults{
			UpdateEvery: 10,
		},
		CreateV2:        func() collectorapi.CollectorV2 { return New(deps) },
		Config:          func() any { return &Config{} },
		JobConfigSchema: schema,
		StoreFirst:      true,
	}
}
func New(deps Dependencies) *Collector {
	return &Collector{
		Config: Config{
			Window: confopt.Duration(5 * time.Minute),
			Site: config.Site{
				PageGroups:        20,
				Countries:         20,
				MeasureSampleRate: 1,
				Bots:              config.BotsExclude,
			},
			OTLP: otlp.Config{
				Enabled:  "auto",
				Endpoint: "127.0.0.1:4317",
			},
		},
		deps:  deps,
		store: metrix.NewCollectorStore(),
	}
}
func (c *Collector) Configuration() any                 { return c.Config }
func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }
func (c *Collector) ChartTemplateYAML() string          { return charts }

func (c *Collector) Check(context.Context) error {
	if c.aggregator == nil {
		return errors.New("site is not initialized")
	}
	return nil
}

func (c *Collector) Cleanup(context.Context) {}
