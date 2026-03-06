// SPDX-License-Identifier: GPL-3.0-or-later

package scriptsd

import (
	"context"
	_ "embed"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed config_schema.json
var configSchema string

//go:embed charts.yaml
var scriptsdChartTemplateV2 string

func init() {
	collectorapi.Register("scriptsd", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 10,
		},
		CreateV2: func() collectorapi.CollectorV2 { return New() },
		Config:   func() any { return &Config{} },
	})
}

// Config is the initial v2 skeleton config surface.
type Config struct {
	Vnode       string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`
}

// Collector is a v2 skeleton collector used as migration scaffold.
type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:",inline"`

	store metrix.CollectorStore
}

func New() *Collector {
	return &Collector{
		Config: Config{
			UpdateEvery: 10,
		},
		store: metrix.NewCollectorStore(),
	}
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) Init(context.Context) error { return nil }

func (c *Collector) Check(context.Context) error { return nil }

func (c *Collector) Collect(context.Context) error { return nil }

func (c *Collector) Cleanup(context.Context) {}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return scriptsdChartTemplateV2 }
