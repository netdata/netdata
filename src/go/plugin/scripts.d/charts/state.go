// SPDX-License-Identifier: GPL-3.0-or-later

package charts

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/ids"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/units"
)

const (
	TelemetryStateMetric   = "state"
	TelemetryRuntimeMetric = "runtime"
	TelemetryLatencyMetric = "latency"
	TelemetryCPUMetric     = "cpu"
	TelemetryMemoryMetric  = "mem"
	TelemetryDiskMetric    = "disk"
	ChartSchedulerJobs     = "jobs"
	ChartSchedulerRate     = "rate"
	ChartSchedulerNext     = "next"
)

func StateChart(meta JobIdentity, priority int) *module.Chart {
	chart := telemetryChartBase(meta, TelemetryStateMetric)
	chart.ID = meta.TelemetryChartID(TelemetryStateMetric)
	chart.Title = fmt.Sprintf("Nagios %s state", meta.ScriptTitle)
	chart.Units = "state"
	chart.Priority = priority
	chart.Dims = module.Dims{
		{ID: meta.TelemetryMetricID(TelemetryStateMetric, "ok"), Name: "OK", Algo: module.Absolute, Div: 1},
		{ID: meta.TelemetryMetricID(TelemetryStateMetric, "warning"), Name: "WARNING", Algo: module.Absolute, Div: 1},
		{ID: meta.TelemetryMetricID(TelemetryStateMetric, "critical"), Name: "CRITICAL", Algo: module.Absolute, Div: 1},
		{ID: meta.TelemetryMetricID(TelemetryStateMetric, "unknown"), Name: "UNKNOWN", Algo: module.Absolute, Div: 1},
		{ID: meta.TelemetryMetricID(TelemetryStateMetric, "attempt"), Name: "attempt", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: meta.TelemetryMetricID(TelemetryStateMetric, "max_attempts"), Name: "max_attempts", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
	}
	chart.Opts.Detail = true
	return &chart
}

func RuntimeChart(meta JobIdentity, priority int) *module.Chart {
	chart := telemetryChartBase(meta, TelemetryRuntimeMetric)
	chart.ID = meta.TelemetryChartID(TelemetryRuntimeMetric)
	chart.Title = fmt.Sprintf("Nagios %s runtime state", meta.ScriptTitle)
	chart.Units = "boolean"
	chart.Priority = priority
	chart.Dims = module.Dims{
		{ID: meta.TelemetryMetricID(TelemetryRuntimeMetric, "running"), Name: "running", Algo: module.Absolute},
		{ID: meta.TelemetryMetricID(TelemetryRuntimeMetric, "retrying"), Name: "retrying", Algo: module.Absolute},
		{ID: meta.TelemetryMetricID(TelemetryRuntimeMetric, "skipped"), Name: "skipped", Algo: module.Absolute},
		{ID: meta.TelemetryMetricID(TelemetryRuntimeMetric, "cpu_missing"), Name: "cpu_missing", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
	}
	chart.Opts.Detail = true
	return &chart
}

func LatencyChart(meta JobIdentity, priority int) *module.Chart {
	chart := telemetryChartBase(meta, TelemetryLatencyMetric)
	chart.ID = meta.TelemetryChartID(TelemetryLatencyMetric)
	chart.Title = fmt.Sprintf("Nagios %s latency", meta.ScriptTitle)
	chart.Units = "seconds"
	chart.Priority = priority
	chart.Dims = module.Dims{{ID: meta.TelemetryMetricID(TelemetryLatencyMetric, "duration"), Name: "duration", Algo: module.Absolute, Div: 1_000_000_000}}
	return &chart
}

func CPUChart(meta JobIdentity, priority int) *module.Chart {
	chart := telemetryChartBase(meta, TelemetryCPUMetric)
	chart.ID = meta.TelemetryChartID(TelemetryCPUMetric)
	chart.Title = fmt.Sprintf("Nagios %s CPU", meta.ScriptTitle)
	chart.Units = "seconds"
	chart.Priority = priority
	chart.Dims = module.Dims{{ID: meta.TelemetryMetricID(TelemetryCPUMetric, "cpu_time"), Name: "cpu", Algo: module.Absolute, Div: 1_000_000_000}}
	return &chart
}

func MemoryChart(meta JobIdentity, priority int) *module.Chart {
	chart := telemetryChartBase(meta, TelemetryMemoryMetric)
	chart.ID = meta.TelemetryChartID(TelemetryMemoryMetric)
	chart.Title = fmt.Sprintf("Nagios %s memory", meta.ScriptTitle)
	chart.Units = "bytes"
	chart.Priority = priority
	chart.Dims = module.Dims{{ID: meta.TelemetryMetricID(TelemetryMemoryMetric, "rss"), Name: "rss", Algo: module.Absolute}}
	return &chart
}

func DiskChart(meta JobIdentity, priority int) *module.Chart {
	chart := telemetryChartBase(meta, TelemetryDiskMetric)
	chart.ID = meta.TelemetryChartID(TelemetryDiskMetric)
	chart.Title = fmt.Sprintf("Nagios %s disk IO", meta.ScriptTitle)
	chart.Units = "bytes"
	chart.Priority = priority
	chart.Dims = module.Dims{
		{ID: meta.TelemetryMetricID(TelemetryDiskMetric, "read"), Name: "read", Algo: module.Absolute},
		{ID: meta.TelemetryMetricID(TelemetryDiskMetric, "write"), Name: "write", Algo: module.Absolute},
	}
	return &chart
}

func SchedulerJobsChart(scheduler string, priority int) *module.Chart {
	chart := schedulerChartBase(scheduler)
	chart.ID = SchedulerChartID(scheduler, ChartSchedulerJobs)
	chart.Title = "Nagios Scheduler Jobs Status"
	chart.Units = "jobs"
	chart.Priority = priority
	chart.Ctx = ctxPrefix + ".scheduler.jobs"
	chart.Dims = module.Dims{
		{ID: SchedulerMetricKey(scheduler, ChartSchedulerJobs, "running"), Name: "running", Algo: module.Absolute},
		{ID: SchedulerMetricKey(scheduler, ChartSchedulerJobs, "queued"), Name: "queued", Algo: module.Absolute},
		{ID: SchedulerMetricKey(scheduler, ChartSchedulerJobs, "scheduled"), Name: "scheduled", Algo: module.Absolute},
	}
	return &chart
}

func SchedulerRateChart(scheduler string, priority int) *module.Chart {
	chart := schedulerChartBase(scheduler)
	chart.ID = SchedulerChartID(scheduler, ChartSchedulerRate)
	chart.Title = "Nagios Scheduler Workload"
	chart.Units = "jobs"
	chart.Priority = priority
	chart.Ctx = ctxPrefix + ".scheduler.rate"
	chart.Dims = module.Dims{
		{ID: SchedulerMetricKey(scheduler, ChartSchedulerRate, "started"), Name: "started", Algo: module.Incremental},
		{ID: SchedulerMetricKey(scheduler, ChartSchedulerRate, "finished"), Name: "finished", Algo: module.Incremental},
		{ID: SchedulerMetricKey(scheduler, ChartSchedulerRate, "skipped"), Name: "skipped", Algo: module.Incremental},
	}
	chart.Opts.Detail = true
	return &chart
}

func SchedulerNextRunChart(scheduler string, priority int) *module.Chart {
	chart := schedulerChartBase(scheduler)
	chart.ID = SchedulerChartID(scheduler, ChartSchedulerNext)
	chart.Title = "Nagios Scheduler Next Run Time"
	chart.Units = "seconds"
	chart.Priority = priority
	chart.Ctx = ctxPrefix + ".scheduler.next"
	chart.Dims = module.Dims{{ID: SchedulerMetricKey(scheduler, ChartSchedulerNext, "next"), Name: "next", Algo: module.Absolute, Div: 1_000_000_000}}
	chart.Opts.Detail = true
	return &chart
}

func PerfdataChart(meta JobIdentity, label string, scale units.Scale, priority int) *module.Chart {
	chart := perfdataChartBase(meta)
	labelID := ids.Sanitize(label)
	if labelID == "" {
		labelID = "metric"
	}
	chart.ID = meta.PerfdataChartID(labelID)
	chart.Title = fmt.Sprintf("Nagios %s %s", meta.ScriptTitle, label)
	chart.Units = canonicalUnit(scale, label)
	chart.Priority = priority
	chart.Ctx = fmt.Sprintf("%s.%s.%s", ctxPrefix, meta.ScriptKey, labelID)
	div := scale.Divisor
	if div <= 0 {
		div = 1
	}
	chart.Dims = module.Dims{
		{ID: meta.PerfdataMetricID(labelID, "value"), Name: "value", Algo: module.Absolute, Div: div},
		{ID: meta.PerfdataMetricID(labelID, "min"), Name: "min", Algo: module.Absolute, Div: div, DimOpts: module.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "max"), Name: "max", Algo: module.Absolute, Div: div, DimOpts: module.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "warn_low"), Name: "warn_low", Algo: module.Absolute, Div: div, DimOpts: module.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "warn_high"), Name: "warn_high", Algo: module.Absolute, Div: div, DimOpts: module.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "warn_low_defined"), Name: "warn_low_defined", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "warn_high_defined"), Name: "warn_high_defined", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "warn_defined"), Name: "warn_defined", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "warn_inclusive"), Name: "warn_inclusive", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "crit_low"), Name: "crit_low", Algo: module.Absolute, Div: div, DimOpts: module.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "crit_high"), Name: "crit_high", Algo: module.Absolute, Div: div, DimOpts: module.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "crit_low_defined"), Name: "crit_low_defined", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "crit_high_defined"), Name: "crit_high_defined", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "crit_defined"), Name: "crit_defined", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "crit_inclusive"), Name: "crit_inclusive", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
	}
	chart.Labels = append(chart.Labels,
		module.Label{Key: "perf_label", Value: label, Source: module.LabelSourceConf},
	)
	chart.Opts.Detail = true
	return &chart
}

func canonicalUnit(scale units.Scale, fallback string) string {
	if scale.CanonicalUnit != "" {
		return scale.CanonicalUnit
	}
	return fallback
}

func schedulerChartBase(scheduler string) module.Chart {
	return module.Chart{
		Fam:          "scheduler",
		Ctx:          fmt.Sprintf("%s.scheduler", ctxPrefix),
		Type:         module.Line,
		TypeOverride: ctxPrefix,
		Labels: []module.Label{
			{Key: "nagios_scheduler", Value: scheduler, Source: module.LabelSourceConf},
		},
	}
}
