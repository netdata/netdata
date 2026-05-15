// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"context"
	_ "embed"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/pathvalidate"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/timeperiod"
)

//go:embed config_schema.json
var configSchema string

//go:embed charts.yaml
var nagiosChartTemplateV2 string

func init() {
	collectorapi.Register("nagios", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: defaultCollectorUpdateEvery,
		},
		CreateV2: func() collectorapi.CollectorV2 { return New() },
		Config:   func() any { return &Config{} },
	})
}

// Config is the public v2 config surface.
type Config struct {
	UpdateEvery     int `yaml:"update_every,omitempty" json:"update_every,omitempty"`
	AutoDetectEvery int `yaml:"autodetection_retry,omitempty" json:"autodetection_retry,omitempty"`
	JobConfig       `yaml:",inline" json:",inline"`
	TimePeriods     []timeperiod.Config `yaml:"time_periods,omitempty" json:"time_periods,omitempty"`
	Notes           string              `yaml:"notes,omitempty" json:"notes,omitempty"`
	DirectorySource string              `yaml:"__directory_source__,omitempty" json:"-"`
}

// Collector is the v2 Nagios collector.
type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:",inline"`

	store          metrix.CollectorStore
	router         *perfdataRouter
	runner         checkRunner
	validatePlugin func(string) (string, error)
	now            func() time.Time
	vnode          vnodes.VirtualNode

	job   compiledJob
	state collectState

	cadenceWarning string
}

func New() *Collector {
	return &Collector{
		Config: Config{
			UpdateEvery: defaultCollectorUpdateEvery,
			JobConfig:   defaultedJobConfig(JobConfig{}),
		},
		store:          metrix.NewCollectorStore(),
		router:         newPerfdataRouter(defaultPerfdataMetricKeyBudget),
		runner:         systemCheckRunner{},
		validatePlugin: pathvalidate.ValidateBinaryPath,
		now:            time.Now,
	}
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) VirtualNode() *vnodes.VirtualNode { return &c.vnode }

func (c *Collector) Init(context.Context) error { return c.initCollector() }

func (c *Collector) Check(context.Context) error { return c.checkCollector() }

func (c *Collector) Collect(ctx context.Context) error { return c.collect(ctx) }

func (c *Collector) Cleanup(context.Context) {}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return nagiosChartTemplateV2 }
