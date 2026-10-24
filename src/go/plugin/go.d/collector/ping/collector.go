// SPDX-License-Identifier: GPL-3.0-or-later

package ping

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/pinger"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var pingChartTemplateV2 string

func init() {
	collectorapi.Register("ping", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 5,
		},
		CreateV2: func() collectorapi.CollectorV2 { return New() },
		Config:   func() any { return &Config{} },
	})
}

func New() *Collector {
	store := metrix.NewCollectorStore()

	return &Collector{
		Config: Config{
			ProbeConfig: pinger.ProbeConfig{
				Network:    "ip",
				Privileged: true,
				Packets:    5,
				Interval:   confopt.Duration(time.Millisecond * 100),
			},
			AnalysisConfig: pinger.AnalysisConfig{
				JitterEWMASamples: 16,
				JitterSMAWindow:   10,
			},
		},

		newPinger: pinger.New,
		store:     store,
	}
}

type Config struct {
	Vnode                 string   `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery           int      `yaml:"update_every,omitempty" json:"update_every"`
	Hosts                 []string `yaml:"hosts" json:"hosts"`
	pinger.ProbeConfig    `yaml:",inline" json:",inline"`
	pinger.AnalysisConfig `yaml:",inline" json:",inline"`
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	client    pinger.Client
	newPinger func(pinger.Config, *logger.Logger) (pinger.Client, error)

	store metrix.CollectorStore
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	err := c.validateConfig()
	if err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	pr, err := c.initPinger()
	if err != nil {
		return fmt.Errorf("init ping client: %v", err)
	}
	c.client = pr

	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	samples := c.collectSamples(ctx, false)
	if len(samples) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (c *Collector) Collect(ctx context.Context) error { return c.collect(ctx) }

func (c *Collector) Cleanup(context.Context) {}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return pingChartTemplateV2 }
