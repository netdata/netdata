// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"context"
	_ "embed"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/nagios/internal/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/nagios/internal/schedulers"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/nagios/internal/spec"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/timeperiod"
)

//go:embed config_schema.json
var configSchema string

//go:embed charts.yaml
var nagiosChartTemplateV2 string

func init() {
	sharedRegistry := schedulers.NewRegistry()
	collectorapi.Register("nagios", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 10,
		},
		CreateV2: func() collectorapi.CollectorV2 { return NewWithRegistry(sharedRegistry) },
		Config:   func() any { return &Config{} },
	})
}

// Config is the initial v2 skeleton config surface.
type Config struct {
	spec.JobConfig `yaml:",inline" json:",inline"`
	TimePeriods    []timeperiod.Config `yaml:"time_periods,omitempty" json:"time_periods,omitempty"`
	Workers        int                 `yaml:"workers,omitempty" json:"workers"`
	QueueSize      int                 `yaml:"queue_size,omitempty" json:"queue_size"`
}

// Collector is a v2 skeleton collector used as migration scaffold.
type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:",inline"`

	store    metrix.CollectorStore
	registry schedulers.SchedulerRegistry
	router   *perfdataRouter

	jobSpec   spec.JobSpec
	periods   *timeperiod.Set
	jobHandle *schedulers.SchedulerJobHandle
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
			JobConfig: spec.JobConfig{
				Scheduler: "default",
			},
			Workers:   50,
			QueueSize: 128,
		},
		store:    metrix.NewCollectorStore(),
		registry: reg,
		router:   newPerfdataRouter(defaultPerfdataMetricKeyBudget),
	}
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) Init(context.Context) error {
	if err := c.compileTimePeriods(); err != nil {
		return err
	}
	sp, err := c.JobConfig.ToSpec()
	if err != nil {
		return err
	}
	c.jobSpec = sp

	def := schedulers.Definition{
		Name:           c.jobSpec.Scheduler,
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
	if err := c.registry.Ensure(def, c.Logger); err != nil {
		return err
	}

	handle, err := c.registry.Attach(c.jobSpec.Scheduler, runtime.JobRegistration{
		Spec:    c.jobSpec,
		Emitter: runtime.NewLogEmitter(c.Logger),
		Periods: c.periods,
	}, c.Logger)
	if err != nil {
		return err
	}
	c.jobHandle = handle
	return nil
}

func (c *Collector) Check(context.Context) error {
	_, err := c.JobConfig.ToSpec()
	return err
}

func (c *Collector) Collect(context.Context) error {
	snapshot, ok := c.registry.Snapshot(c.jobSpec.Scheduler)
	if !ok {
		return nil
	}

	sm := c.store.Write().SnapshotMeter("nagios")
	lbl := sm.LabelSet(metrix.Label{Key: "nagios_scheduler", Value: c.jobSpec.Scheduler})

	observe := func(name string, value float64) {
		sm.Gauge(name).Observe(value, lbl)
	}
	observe("scheduler.running", float64(snapshot.Running))
	observe("scheduler.queued", float64(snapshot.Queued))
	observe("scheduler.scheduled", float64(snapshot.Scheduled))
	sm.Counter("scheduler.started").ObserveTotal(float64(snapshot.Started), lbl)
	sm.Counter("scheduler.finished").ObserveTotal(float64(snapshot.Finished), lbl)
	sm.Counter("scheduler.skipped").ObserveTotal(float64(snapshot.Skipped), lbl)

	for _, job := range snapshot.Jobs {
		if c.jobHandle != nil && job.JobID != c.jobHandle.JobID() {
			continue
		}

		jobLbl := sm.LabelSet(
			metrix.Label{Key: "nagios_scheduler", Value: c.jobSpec.Scheduler},
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

		perf := c.router.route(c.jobSpec.Scheduler, job.JobName, job.PerfSamples)
		for _, sample := range perf {
			sm.Gauge(sample.name, metrix.WithUnit(sample.unit), metrix.WithFloat(sample.float)).Observe(sample.value, jobLbl)
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
	if c.registry == nil {
		return
	}
	if c.jobHandle != nil {
		c.registry.Detach(c.jobHandle)
		c.jobHandle = nil
	}
	scheduler := c.jobSpec.Scheduler
	if scheduler == "" {
		scheduler = "default"
	}
	if err := c.registry.Remove(scheduler); err != nil {
		c.Warningf("scheduler cleanup remove %q: %v", scheduler, err)
	}
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return nagiosChartTemplateV2 }

func boolToFloat(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

func (c *Collector) compileTimePeriods() error {
	cfgs := timeperiod.EnsureDefault(append([]timeperiod.Config(nil), c.TimePeriods...))
	set, err := timeperiod.Compile(cfgs)
	if err != nil {
		return err
	}
	c.periods = set
	return nil
}
