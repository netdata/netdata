// SPDX-License-Identifier: GPL-3.0-or-later

package scriptsd

import (
	"context"
	_ "embed"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/charts"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/schedulers"
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
	Scheduler   string `yaml:"scheduler,omitempty" json:"scheduler"`
	Workers     int    `yaml:"workers,omitempty" json:"workers"`
	QueueSize   int    `yaml:"queue_size,omitempty" json:"queue_size"`
}

// Collector is a v2 skeleton collector used as migration scaffold.
type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:",inline"`

	store    metrix.CollectorStore
	registry schedulers.SchedulerRegistry
}

func New() *Collector {
	return NewWithRegistry(nil)
}

func NewWithRegistry(reg schedulers.SchedulerRegistry) *Collector {
	if reg == nil {
		reg = schedulers.NewRegistry()
	}
	return &Collector{
		Config: Config{
			UpdateEvery: 10,
			Scheduler:   "default",
			Workers:     50,
			QueueSize:   128,
		},
		store:    metrix.NewCollectorStore(),
		registry: reg,
	}
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) Init(context.Context) error {
	if c.Scheduler == "" {
		c.Scheduler = "default"
	}

	def := schedulers.Definition{
		Name:           c.Scheduler,
		Workers:        c.Workers,
		QueueSize:      c.QueueSize,
		LoggingEnabled: true,
		Logging: runtime.OTLPEmitterConfig{
			Endpoint: runtime.DefaultOTLPEndpoint,
			Timeout:  runtime.DefaultOTLPTimeout,
			UseTLS:   false,
			Headers:  map[string]string{},
		},
	}
	return c.registry.Ensure(def, c.Logger)
}

func (c *Collector) Check(context.Context) error { return nil }

func (c *Collector) Collect(context.Context) error {
	metrics := c.registry.Collect(c.Scheduler)

	sm := c.store.Write().SnapshotMeter("scriptsd")
	lbl := sm.LabelSet(metrix.Label{Key: "nagios_scheduler", Value: c.Scheduler})

	observe := func(name string, key string) {
		v := metrics[key]
		sm.Gauge(name).Observe(float64(v), lbl)
	}

	observe("scheduler.running", charts.SchedulerMetricKey(c.Scheduler, charts.ChartSchedulerJobs, "running"))
	observe("scheduler.queued", charts.SchedulerMetricKey(c.Scheduler, charts.ChartSchedulerJobs, "queued"))
	observe("scheduler.scheduled", charts.SchedulerMetricKey(c.Scheduler, charts.ChartSchedulerJobs, "scheduled"))
	observe("scheduler.started", charts.SchedulerMetricKey(c.Scheduler, charts.ChartSchedulerRate, "started"))
	observe("scheduler.finished", charts.SchedulerMetricKey(c.Scheduler, charts.ChartSchedulerRate, "finished"))
	observe("scheduler.skipped", charts.SchedulerMetricKey(c.Scheduler, charts.ChartSchedulerRate, "skipped"))

	return nil
}

func (c *Collector) Cleanup(context.Context) {
	if c.registry == nil || c.Scheduler == "" {
		return
	}
	if err := c.registry.Remove(c.Scheduler); err != nil {
		c.Warningf("scheduler cleanup remove %q: %v", c.Scheduler, err)
	}
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return scriptsdChartTemplateV2 }
