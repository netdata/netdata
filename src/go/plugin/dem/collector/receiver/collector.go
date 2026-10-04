// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"context"
	"crypto/tls"
	_ "embed"
	"errors"
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/geoip"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed charts.yaml
var charts string

//go:embed config_schema.json
var schema string

type Config struct {
	UpdateEvery     int `yaml:"update_every,omitempty" json:"update_every"`
	config.Receiver `    yaml:",inline"                json:""`
}
type Collector struct {
	collectorapi.Base
	Config    `yaml:",inline" json:""`
	registry  *rumregistry.Registry
	store     metrix.CollectorStore
	metrics   collectorMetrics
	tlsConfig *tls.Config
	geo       *geoip.Resolver
	requests  [3]atomic.Uint64
}

func Creator(registry *rumregistry.Registry) collectorapi.Creator {
	return collectorapi.Creator{
		Defaults: collectorapi.Defaults{
			UpdateEvery: 10,
		},
		InstancePolicy:  collectorapi.InstancePolicySingle,
		CreateV2:        func() collectorapi.CollectorV2 { return New(registry) },
		Config:          func() any { return &Config{} },
		JobConfigSchema: schema,
		StoreFirst:      true,
	}
}
func New(registry *rumregistry.Registry) *Collector {
	c := &Collector{
		Config: Config{
			Receiver: config.Receiver{
				Listen:       "127.0.0.1:19938",
				MaxBodyBytes: 262144,
				RateLimit: config.RateLimit{
					PerIPPerMin:   120,
					PerSitePerSec: 500,
				},
			},
		},
		registry: registry,
		store:    metrix.NewCollectorStore(),
	}
	c.metrics = newCollectorMetrics(c.store.Write().SnapshotMeter(""))
	return c
}
func (c *Collector) Configuration() any                 { return c.Config }
func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }
func (c *Collector) ChartTemplateYAML() string          { return charts }
func (c *Collector) Check(context.Context) error {
	if c.registry == nil {
		return errors.New("missing runtime routes")
	}
	return nil
}

func (c *Collector) Cleanup(context.Context) {
	if c.geo != nil {
		c.geo.Close()
		c.geo = nil
	}
}
