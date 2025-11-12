// SPDX-License-Identifier: GPL-3.0-or-later

package charts

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/nagios.d/pkg/ids"
)

const (
	chartState          = "state"
	chartRuntime        = "runtime"
	chartLatency        = "latency"
	chartCPU            = "cpu"
	chartMemory         = "mem"
	chartDisk           = "disk"
	chartSchedulerQueue = "scheduler_queue"
	chartSchedulerSkip  = "scheduler_skipped"
	chartSchedulerNext  = "scheduler_next"
)

func StateChart(shard, job string, priority int) *module.Chart {
	chart := jobChartBase(shard, job)
	chart.ID = ChartID(shard, job, chartState)
	chart.Title = job + " state"
	chart.Units = "state"
	chart.Priority = priority
	chart.Ctx = ctxPrefix + ".state"
	chart.Dims = module.Dims{
		{ID: "ok", Name: "OK", Algo: module.Absolute, Div: 1},
		{ID: "warning", Name: "WARNING", Algo: module.Absolute, Div: 1},
		{ID: "critical", Name: "CRITICAL", Algo: module.Absolute, Div: 1},
		{ID: "unknown", Name: "UNKNOWN", Algo: module.Absolute, Div: 1},
	}
	chart.Opts.Detail = true
	return &chart
}

func RuntimeChart(shard, job string, priority int) *module.Chart {
	chart := jobChartBase(shard, job)
	chart.ID = ChartID(shard, job, chartRuntime)
	chart.Title = job + " runtime state"
	chart.Units = "boolean"
	chart.Priority = priority
	chart.Ctx = ctxPrefix + ".runtime"
	chart.Dims = module.Dims{
		{ID: "running", Name: "running", Algo: module.Absolute},
		{ID: "retrying", Name: "retrying", Algo: module.Absolute},
		{ID: "skipped", Name: "skipped", Algo: module.Absolute},
	}
	chart.Opts.Detail = true
	return &chart
}

func LatencyChart(shard, job string, priority int) *module.Chart {
	chart := jobChartBase(shard, job)
	chart.ID = ChartID(shard, job, chartLatency)
	chart.Title = job + " latency"
	chart.Units = "milliseconds"
	chart.Priority = priority
	chart.Ctx = ctxPrefix + ".latency"
	chart.Dims = module.Dims{{ID: "duration", Name: "duration", Algo: module.Absolute}}
	return &chart
}

func CPUChart(shard, job string, priority int) *module.Chart {
	chart := jobChartBase(shard, job)
	chart.ID = ChartID(shard, job, chartCPU)
	chart.Title = job + " CPU"
	chart.Units = "milliseconds"
	chart.Priority = priority
	chart.Ctx = ctxPrefix + ".cpu"
	chart.Dims = module.Dims{{ID: "cpu_time", Name: "cpu", Algo: module.Absolute}}
	return &chart
}

func MemoryChart(shard, job string, priority int) *module.Chart {
	chart := jobChartBase(shard, job)
	chart.ID = ChartID(shard, job, chartMemory)
	chart.Title = job + " memory"
	chart.Units = "bytes"
	chart.Priority = priority
	chart.Ctx = ctxPrefix + ".mem"
	chart.Dims = module.Dims{{ID: "rss", Name: "rss", Algo: module.Absolute}}
	return &chart
}

func DiskChart(shard, job string, priority int) *module.Chart {
	chart := jobChartBase(shard, job)
	chart.ID = ChartID(shard, job, chartDisk)
	chart.Title = job + " disk IO"
	chart.Units = "bytes"
	chart.Priority = priority
	chart.Ctx = ctxPrefix + ".disk"
	chart.Dims = module.Dims{
		{ID: "read", Name: "read", Algo: module.Absolute},
		{ID: "write", Name: "write", Algo: module.Absolute},
	}
	return &chart
}

func SchedulerQueueChart(shard string, priority int) *module.Chart {
	chart := jobChartBase(shard, "scheduler")
	chart.ID = ChartID(shard, "scheduler", chartSchedulerQueue)
	chart.Title = "nagios scheduler queue"
	chart.Units = "jobs"
	chart.Priority = priority
	chart.Ctx = ctxPrefix + ".scheduler.queue"
	chart.Dims = module.Dims{
		{ID: "queue_depth", Name: "queue", Algo: module.Absolute},
		{ID: "waiting", Name: "waiting", Algo: module.Absolute},
		{ID: "executing", Name: "executing", Algo: module.Absolute},
	}
	return &chart
}

func SchedulerSkippedChart(shard string, priority int) *module.Chart {
	chart := jobChartBase(shard, "scheduler")
	chart.ID = ChartID(shard, "scheduler", chartSchedulerSkip)
	chart.Title = "nagios scheduler skipped"
	chart.Units = "count"
	chart.Priority = priority
	chart.Ctx = ctxPrefix + ".scheduler.skipped"
	chart.Dims = module.Dims{{ID: "skipped", Name: "skipped", Algo: module.Absolute}}
	chart.Opts.Detail = true
	return &chart
}

func SchedulerNextRunChart(shard string, priority int) *module.Chart {
	chart := jobChartBase(shard, "scheduler")
	chart.ID = ChartID(shard, "scheduler", chartSchedulerNext)
	chart.Title = "nagios scheduler next run"
	chart.Units = "milliseconds"
	chart.Priority = priority
	chart.Ctx = ctxPrefix + ".scheduler.next"
	chart.Dims = module.Dims{{ID: "next_ms", Name: "next", Algo: module.Absolute}}
	chart.Opts.Detail = true
	return &chart
}

func PerfdataChart(shard, job, label string, priority int) *module.Chart {
	chart := jobChartBase(shard, job)
	suffix := fmt.Sprintf("perf.%s", ids.Sanitize(label))
	chart.ID = ChartID(shard, job, suffix)
	chart.Title = fmt.Sprintf("%s %s", job, label)
	chart.Units = "value"
	chart.Priority = priority
	chart.Ctx = ctxPrefix + ".perfdata"
	chart.Dims = module.Dims{
		{ID: "value", Name: "value", Algo: module.Absolute},
		{ID: "min", Name: "min", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "max", Name: "max", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "warn_low", Name: "warn_low", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "warn_high", Name: "warn_high", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "warn_low_defined", Name: "warn_low_defined", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "warn_high_defined", Name: "warn_high_defined", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "warn_defined", Name: "warn_defined", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "warn_inclusive", Name: "warn_inclusive", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "crit_low", Name: "crit_low", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "crit_high", Name: "crit_high", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "crit_low_defined", Name: "crit_low_defined", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "crit_high_defined", Name: "crit_high_defined", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "crit_defined", Name: "crit_defined", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "crit_inclusive", Name: "crit_inclusive", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
	}
	chart.Labels = append(chart.Labels,
		module.Label{Key: "perf_label", Value: label, Source: module.LabelSourceConf},
		module.Label{Key: "nagios_job", Value: job, Source: module.LabelSourceConf},
		module.Label{Key: "nagios_shard", Value: shard, Source: module.LabelSourceConf},
	)
	chart.Opts.Detail = true
	return &chart
}
