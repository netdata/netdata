// SPDX-License-Identifier: GPL-3.0-or-later

// Package rum implements the rum module, one job per monitored site. The job owns the site's aggregator, history
// writer and optional OTLP exports, admits the site's route to the RUM registry while it runs, and publishes the
// site's metrics.
package rum

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	redact "github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	rumhistory "github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

//go:embed charts.yaml
var charts string

//go:embed config_schema.json
var schema string

type Config struct {
	Vnode       string               `yaml:"vnode,omitempty"        json:"vnode,omitempty"`
	UpdateEvery int                  `yaml:"update_every,omitempty" json:"update_every"`
	Window      confopt.LongDuration `yaml:"window"                 json:"window"`
	config.Site `yaml:",inline" json:""`
}

type Dependencies struct {
	Registry *rumregistry.Registry
	History  *rumhistory.Store
}

type Collector struct {
	collectorapi.Base
	Config     `yaml:",inline" json:""`
	deps       Dependencies
	store      metrix.CollectorStore
	aggregator *aggregate.Aggregator
	transport  *transportCounters
	redactor   *redact.Redactor
	metrics    collectorMetrics
	vnodeGUID  string
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
			Window: confopt.LongDuration(5 * time.Minute),
			Site: config.Site{
				PageGroups: 20,
				Countries:  20,
				Bots:       config.BotsExclude,
			}.Effective(),
		},
		deps:  deps,
		store: metrix.NewCollectorStore(),
	}
}

func (c *Collector) Configuration() any {
	cfg := c.Config
	cfg.Site = cfg.Site.Effective()
	return cfg
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }
func (c *Collector) ChartTemplateYAML() string          { return charts }

func (c *Collector) Check(context.Context) error {
	if c.aggregator == nil {
		return errors.New("site is not initialized")
	}
	return nil
}

func (c *Collector) Cleanup(context.Context) {}

// SetConfiguredVnode adopts framework-resolved identity without changing the authored reference.
func (c *Collector) SetConfiguredVnode(v vnodes.VirtualNode) {
	c.vnodeGUID, _ = vnodes.ConfiguredGUIDKey(v.GUID)
	if c.aggregator != nil {
		c.aggregator.SetVnodeGUID(c.vnodeGUID)
	}
}
