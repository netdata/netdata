// SPDX-License-Identifier: GPL-3.0-or-later

package charts

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
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

func StateChart(meta JobIdentity, priority int) *collectorapi.Chart {
	chart := telemetryChartBase(meta, TelemetryStateMetric)
	chart.ID = meta.TelemetryChartID(TelemetryStateMetric)
	chart.Title = "Nagios Plugin State"
	chart.Units = "state"
	chart.Priority = priority
	chart.Dims = collectorapi.Dims{
		{ID: meta.TelemetryMetricID(TelemetryStateMetric, "ok"), Name: "OK", Algo: collectorapi.Absolute, Div: 1},
		{ID: meta.TelemetryMetricID(TelemetryStateMetric, "warning"), Name: "WARNING", Algo: collectorapi.Absolute, Div: 1},
		{ID: meta.TelemetryMetricID(TelemetryStateMetric, "critical"), Name: "CRITICAL", Algo: collectorapi.Absolute, Div: 1},
		{ID: meta.TelemetryMetricID(TelemetryStateMetric, "unknown"), Name: "UNKNOWN", Algo: collectorapi.Absolute, Div: 1},
		{ID: meta.TelemetryMetricID(TelemetryStateMetric, "attempt"), Name: "attempt", Algo: collectorapi.Absolute, DimOpts: collectorapi.DimOpts{Hidden: true}},
		{ID: meta.TelemetryMetricID(TelemetryStateMetric, "max_attempts"), Name: "max_attempts", Algo: collectorapi.Absolute, DimOpts: collectorapi.DimOpts{Hidden: true}},
	}
	chart.Opts.Detail = true
	return &chart
}

func RuntimeChart(meta JobIdentity, priority int) *collectorapi.Chart {
	chart := telemetryChartBase(meta, TelemetryRuntimeMetric)
	chart.ID = meta.TelemetryChartID(TelemetryRuntimeMetric)
	chart.Title = "Nagios Plugin Runtime State"
	chart.Units = "boolean"
	chart.Priority = priority
	chart.Dims = collectorapi.Dims{
		{ID: meta.TelemetryMetricID(TelemetryRuntimeMetric, "running"), Name: "running", Algo: collectorapi.Absolute},
		{ID: meta.TelemetryMetricID(TelemetryRuntimeMetric, "retrying"), Name: "retrying", Algo: collectorapi.Absolute},
		{ID: meta.TelemetryMetricID(TelemetryRuntimeMetric, "skipped"), Name: "skipped", Algo: collectorapi.Absolute},
		{ID: meta.TelemetryMetricID(TelemetryRuntimeMetric, "cpu_missing"), Name: "cpu_missing", Algo: collectorapi.Absolute, DimOpts: collectorapi.DimOpts{Hidden: true}},
	}
	chart.Opts.Detail = true
	return &chart
}

func LatencyChart(meta JobIdentity, priority int) *collectorapi.Chart {
	chart := telemetryChartBase(meta, TelemetryLatencyMetric)
	chart.ID = meta.TelemetryChartID(TelemetryLatencyMetric)
	chart.Title = "Nagios Plugin Execution Time"
	chart.Units = "seconds"
	chart.Priority = priority
	chart.Dims = collectorapi.Dims{{ID: meta.TelemetryMetricID(TelemetryLatencyMetric, "duration"), Name: "duration", Algo: collectorapi.Absolute, Div: 1_000_000_000}}
	return &chart
}

func CPUChart(meta JobIdentity, priority int) *collectorapi.Chart {
	chart := telemetryChartBase(meta, TelemetryCPUMetric)
	chart.ID = meta.TelemetryChartID(TelemetryCPUMetric)
	chart.Title = "Nagios Plugin CPU Usage"
	chart.Units = "seconds"
	chart.Priority = priority
	chart.Dims = collectorapi.Dims{{ID: meta.TelemetryMetricID(TelemetryCPUMetric, "cpu_time"), Name: "cpu", Algo: collectorapi.Absolute, Div: 1_000_000_000}}
	return &chart
}

func MemoryChart(meta JobIdentity, priority int) *collectorapi.Chart {
	chart := telemetryChartBase(meta, TelemetryMemoryMetric)
	chart.ID = meta.TelemetryChartID(TelemetryMemoryMetric)
	chart.Title = "Nagios Plugin Memory Usage"
	chart.Units = "bytes"
	chart.Priority = priority
	chart.Dims = collectorapi.Dims{{ID: meta.TelemetryMetricID(TelemetryMemoryMetric, "rss"), Name: "rss", Algo: collectorapi.Absolute}}
	return &chart
}

func DiskChart(meta JobIdentity, priority int) *collectorapi.Chart {
	chart := telemetryChartBase(meta, TelemetryDiskMetric)
	chart.ID = meta.TelemetryChartID(TelemetryDiskMetric)
	chart.Title = "Nagios Plugin Disk I/O"
	chart.Units = "bytes"
	chart.Priority = priority
	chart.Dims = collectorapi.Dims{
		{ID: meta.TelemetryMetricID(TelemetryDiskMetric, "read"), Name: "read", Algo: collectorapi.Absolute},
		{ID: meta.TelemetryMetricID(TelemetryDiskMetric, "write"), Name: "write", Algo: collectorapi.Absolute},
	}
	return &chart
}

func SchedulerJobsChart(scheduler string, priority int) *collectorapi.Chart {
	chart := schedulerChartBase(scheduler)
	chart.ID = SchedulerChartID(scheduler, ChartSchedulerJobs)
	chart.Title = "Nagios Scheduler Jobs Status"
	chart.Units = "jobs"
	chart.Priority = priority
	chart.Ctx = ctxPrefix + ".scheduler.jobs"
	chart.Dims = collectorapi.Dims{
		{ID: SchedulerMetricKey(scheduler, ChartSchedulerJobs, "running"), Name: "running", Algo: collectorapi.Absolute},
		{ID: SchedulerMetricKey(scheduler, ChartSchedulerJobs, "queued"), Name: "queued", Algo: collectorapi.Absolute},
		{ID: SchedulerMetricKey(scheduler, ChartSchedulerJobs, "scheduled"), Name: "scheduled", Algo: collectorapi.Absolute},
	}
	return &chart
}

func SchedulerRateChart(scheduler string, priority int) *collectorapi.Chart {
	chart := schedulerChartBase(scheduler)
	chart.ID = SchedulerChartID(scheduler, ChartSchedulerRate)
	chart.Title = "Nagios Scheduler Workload"
	chart.Units = "jobs"
	chart.Priority = priority
	chart.Ctx = ctxPrefix + ".scheduler.rate"
	chart.Dims = collectorapi.Dims{
		{ID: SchedulerMetricKey(scheduler, ChartSchedulerRate, "started"), Name: "started", Algo: collectorapi.Incremental},
		{ID: SchedulerMetricKey(scheduler, ChartSchedulerRate, "finished"), Name: "finished", Algo: collectorapi.Incremental},
		{ID: SchedulerMetricKey(scheduler, ChartSchedulerRate, "skipped"), Name: "skipped", Algo: collectorapi.Incremental},
	}
	chart.Opts.Detail = true
	return &chart
}

func SchedulerNextRunChart(scheduler string, priority int) *collectorapi.Chart {
	chart := schedulerChartBase(scheduler)
	chart.ID = SchedulerChartID(scheduler, ChartSchedulerNext)
	chart.Title = "Nagios Scheduler Next Run Time"
	chart.Units = "seconds"
	chart.Priority = priority
	chart.Ctx = ctxPrefix + ".scheduler.next"
	chart.Dims = collectorapi.Dims{{ID: SchedulerMetricKey(scheduler, ChartSchedulerNext, "next"), Name: "next", Algo: collectorapi.Absolute, Div: 1_000_000_000}}
	chart.Opts.Detail = true
	return &chart
}

func PerfdataChart(meta JobIdentity, label string, scale units.Scale, priority int) *collectorapi.Chart {
	chart := perfdataChartBase(meta)
	labelID := ids.Sanitize(label)
	if labelID == "" {
		labelID = "metric"
	}
	chart.ID = meta.PerfdataChartID(labelID)
	chart.Title = "Nagios Plugin Performance Data"
	chart.Units = canonicalUnit(scale, label)
	chart.Priority = priority
	chart.Ctx = fmt.Sprintf("%s.%s.%s", ctxPrefix, meta.ScriptKey, labelID)
	div := scale.Divisor
	if div <= 0 {
		div = 1
	}
	chart.Dims = collectorapi.Dims{
		{ID: meta.PerfdataMetricID(labelID, "value"), Name: "value", Algo: collectorapi.Absolute, Div: div},
		{ID: meta.PerfdataMetricID(labelID, "min"), Name: "min", Algo: collectorapi.Absolute, Div: div, DimOpts: collectorapi.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "max"), Name: "max", Algo: collectorapi.Absolute, Div: div, DimOpts: collectorapi.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "warn_low"), Name: "warn_low", Algo: collectorapi.Absolute, Div: div, DimOpts: collectorapi.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "warn_high"), Name: "warn_high", Algo: collectorapi.Absolute, Div: div, DimOpts: collectorapi.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "warn_low_defined"), Name: "warn_low_defined", Algo: collectorapi.Absolute, DimOpts: collectorapi.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "warn_high_defined"), Name: "warn_high_defined", Algo: collectorapi.Absolute, DimOpts: collectorapi.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "warn_defined"), Name: "warn_defined", Algo: collectorapi.Absolute, DimOpts: collectorapi.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "warn_inclusive"), Name: "warn_inclusive", Algo: collectorapi.Absolute, DimOpts: collectorapi.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "crit_low"), Name: "crit_low", Algo: collectorapi.Absolute, Div: div, DimOpts: collectorapi.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "crit_high"), Name: "crit_high", Algo: collectorapi.Absolute, Div: div, DimOpts: collectorapi.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "crit_low_defined"), Name: "crit_low_defined", Algo: collectorapi.Absolute, DimOpts: collectorapi.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "crit_high_defined"), Name: "crit_high_defined", Algo: collectorapi.Absolute, DimOpts: collectorapi.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "crit_defined"), Name: "crit_defined", Algo: collectorapi.Absolute, DimOpts: collectorapi.DimOpts{Hidden: true}},
		{ID: meta.PerfdataMetricID(labelID, "crit_inclusive"), Name: "crit_inclusive", Algo: collectorapi.Absolute, DimOpts: collectorapi.DimOpts{Hidden: true}},
	}
	chart.Labels = append(chart.Labels,
		collectorapi.Label{Key: "perf_label", Value: label, Source: collectorapi.LabelSourceConf},
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

func schedulerChartBase(scheduler string) collectorapi.Chart {
	return collectorapi.Chart{
		Fam:  "scheduler",
		Ctx:  fmt.Sprintf("%s.scheduler", ctxPrefix),
		Type: collectorapi.Line,
		Labels: []collectorapi.Label{
			{Key: "nagios_scheduler", Value: scheduler, Source: collectorapi.LabelSourceConf},
		},
	}
}
