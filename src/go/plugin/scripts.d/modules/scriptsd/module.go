// SPDX-License-Identifier: GPL-3.0-or-later

package scriptsd

import (
	"context"
	_ "embed"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
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
	router   *perfdataRouter
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
		router:   newPerfdataRouter(defaultPerfdataMetricKeyBudget),
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
	snapshot, ok := c.registry.Snapshot(c.Scheduler)
	if !ok {
		return nil
	}

	sm := c.store.Write().SnapshotMeter("scriptsd")
	lbl := sm.LabelSet(metrix.Label{Key: "nagios_scheduler", Value: c.Scheduler})

	observe := func(name string, value float64) {
		sm.Gauge(name).Observe(value, lbl)
	}
	observe("scheduler.running", float64(snapshot.Running))
	observe("scheduler.queued", float64(snapshot.Queued))
	observe("scheduler.scheduled", float64(snapshot.Scheduled))
	observe("scheduler.started", float64(snapshot.Started))
	observe("scheduler.finished", float64(snapshot.Finished))
	observe("scheduler.skipped", float64(snapshot.Skipped))

	for _, job := range snapshot.Jobs {
		jobLbl := sm.LabelSet(
			metrix.Label{Key: "nagios_scheduler", Value: c.Scheduler},
			metrix.Label{Key: "nagios_job", Value: job.JobName},
		)
		observeJob := func(name string, value float64) {
			sm.Gauge(name).Observe(value, jobLbl)
		}

		observeJob("job.state.ok", boolToFloat(strings.EqualFold(job.State, "OK")))
		observeJob("job.state.warning", boolToFloat(strings.EqualFold(job.State, "WARNING")))
		observeJob("job.state.critical", boolToFloat(strings.EqualFold(job.State, "CRITICAL")))
		observeJob("job.state.unknown", boolToFloat(strings.EqualFold(job.State, "UNKNOWN")))
		observeJob("job.attempt", float64(job.Attempt))
		observeJob("job.max_attempts", float64(job.MaxAttempt))

		perf := c.router.route(c.Scheduler, job.JobName, job.PerfSamples)
		for _, sample := range perf {
			observeJob(sample.name, sample.value)
		}
	}

	counters := c.router.dropCounters()
	sm.Counter("perfdata_dropped_invalid_total").ObserveTotal(float64(counters.Invalid), lbl)
	sm.Counter("perfdata_dropped_collision_total").ObserveTotal(float64(counters.Collision), lbl)
	sm.Counter("perfdata_dropped_unit_drift_total").ObserveTotal(float64(counters.UnitDrift), lbl)
	sm.Counter("perfdata_dropped_budget_total").ObserveTotal(float64(counters.Budget), lbl)

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

func boolToFloat(v bool) float64 {
	if v {
		return 1
	}
	return 0
}
