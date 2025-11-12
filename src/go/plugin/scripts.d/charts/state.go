// SPDX-License-Identifier: GPL-3.0-or-later

package charts

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/ids"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/units"
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

func StateChart(meta JobIdentity, priority int) *module.Chart {
	chart := jobChartBase(meta)
	chart.ID = meta.ChartID(chartState)
	chart.Title = fmt.Sprintf("Nagios %s state", meta.ScriptTitle)
	chart.Units = "state"
	chart.Priority = priority
	chart.Ctx = fmt.Sprintf("%s.%s.%s", ctxPrefix, meta.ScriptKey, chartState)
	chart.Dims = module.Dims{
		{ID: "ok", Name: "OK", Algo: module.Absolute, Div: 1},
		{ID: "warning", Name: "WARNING", Algo: module.Absolute, Div: 1},
		{ID: "critical", Name: "CRITICAL", Algo: module.Absolute, Div: 1},
		{ID: "unknown", Name: "UNKNOWN", Algo: module.Absolute, Div: 1},
		{ID: "attempt", Name: "attempt", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "max_attempts", Name: "max_attempts", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
	}
	chart.Opts.Detail = true
	return &chart
}

func RuntimeChart(meta JobIdentity, priority int) *module.Chart {
	chart := jobChartBase(meta)
	chart.ID = meta.ChartID(chartRuntime)
	chart.Title = fmt.Sprintf("Nagios %s runtime state", meta.ScriptTitle)
	chart.Units = "boolean"
	chart.Priority = priority
	chart.Ctx = fmt.Sprintf("%s.%s.%s", ctxPrefix, meta.ScriptKey, chartRuntime)
	chart.Dims = module.Dims{
		{ID: "running", Name: "running", Algo: module.Absolute},
		{ID: "retrying", Name: "retrying", Algo: module.Absolute},
		{ID: "skipped", Name: "skipped", Algo: module.Absolute},
	}
	chart.Opts.Detail = true
	return &chart
}

func LatencyChart(meta JobIdentity, priority int) *module.Chart {
	chart := jobChartBase(meta)
	chart.ID = meta.ChartID(chartLatency)
	chart.Title = fmt.Sprintf("Nagios %s latency", meta.ScriptTitle)
	chart.Units = "seconds"
	chart.Priority = priority
	chart.Ctx = fmt.Sprintf("%s.%s.%s", ctxPrefix, meta.ScriptKey, chartLatency)
	chart.Dims = module.Dims{{ID: "duration", Name: "duration", Algo: module.Absolute, Div: 1_000_000_000}}
	return &chart
}

func CPUChart(meta JobIdentity, priority int) *module.Chart {
	chart := jobChartBase(meta)
	chart.ID = meta.ChartID(chartCPU)
	chart.Title = fmt.Sprintf("Nagios %s CPU", meta.ScriptTitle)
	chart.Units = "seconds"
	chart.Priority = priority
	chart.Ctx = fmt.Sprintf("%s.%s.%s", ctxPrefix, meta.ScriptKey, chartCPU)
	chart.Dims = module.Dims{{ID: "cpu_time", Name: "cpu", Algo: module.Absolute, Div: 1_000_000_000}}
	return &chart
}

func MemoryChart(meta JobIdentity, priority int) *module.Chart {
	chart := jobChartBase(meta)
	chart.ID = meta.ChartID(chartMemory)
	chart.Title = fmt.Sprintf("Nagios %s memory", meta.ScriptTitle)
	chart.Units = "bytes"
	chart.Priority = priority
	chart.Ctx = fmt.Sprintf("%s.%s.%s", ctxPrefix, meta.ScriptKey, chartMemory)
	chart.Dims = module.Dims{{ID: "rss", Name: "rss", Algo: module.Absolute}}
	return &chart
}

func DiskChart(meta JobIdentity, priority int) *module.Chart {
	chart := jobChartBase(meta)
	chart.ID = meta.ChartID(chartDisk)
	chart.Title = fmt.Sprintf("Nagios %s disk IO", meta.ScriptTitle)
	chart.Units = "bytes"
	chart.Priority = priority
	chart.Ctx = fmt.Sprintf("%s.%s.%s", ctxPrefix, meta.ScriptKey, chartDisk)
	chart.Dims = module.Dims{
		{ID: "read", Name: "read", Algo: module.Absolute},
		{ID: "write", Name: "write", Algo: module.Absolute},
	}
	return &chart
}

func SchedulerQueueChart(shard string, priority int) *module.Chart {
	chart := schedulerChartBase(shard)
	chart.ID = ChartIDFromParts(shard, "scheduler", chartSchedulerQueue)
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
	chart := schedulerChartBase(shard)
	chart.ID = ChartIDFromParts(shard, "scheduler", chartSchedulerSkip)
	chart.Title = "nagios scheduler skipped"
	chart.Units = "count"
	chart.Priority = priority
	chart.Ctx = ctxPrefix + ".scheduler.skipped"
	chart.Dims = module.Dims{{ID: "skipped", Name: "skipped", Algo: module.Absolute}}
	chart.Opts.Detail = true
	return &chart
}

func SchedulerNextRunChart(shard string, priority int) *module.Chart {
	chart := schedulerChartBase(shard)
	chart.ID = ChartIDFromParts(shard, "scheduler", chartSchedulerNext)
	chart.Title = "nagios scheduler next run"
	chart.Units = "seconds"
	chart.Priority = priority
	chart.Ctx = ctxPrefix + ".scheduler.next"
	chart.Dims = module.Dims{{ID: "next", Name: "next", Algo: module.Absolute, Div: 1_000_000_000}}
	chart.Opts.Detail = true
	return &chart
}

func PerfdataChart(meta JobIdentity, label string, scale units.Scale, priority int) *module.Chart {
	chart := jobChartBase(meta)
	labelID := ids.Sanitize(label)
	if labelID == "" {
		labelID = "metric"
	}
	suffix := fmt.Sprintf("perf.%s", labelID)
	chart.ID = meta.ChartID(suffix)
	chart.Title = fmt.Sprintf("Nagios %s %s", meta.ScriptTitle, label)
	chart.Units = canonicalUnit(scale, label)
	chart.Priority = priority
	chart.Ctx = fmt.Sprintf("%s.%s.%s", ctxPrefix, meta.ScriptKey, labelID)
	div := scale.Divisor
	if div <= 0 {
		div = 1
	}
	chart.Dims = module.Dims{
		{ID: "value", Name: "value", Algo: module.Absolute, Div: div},
		{ID: "min", Name: "min", Algo: module.Absolute, Div: div, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "max", Name: "max", Algo: module.Absolute, Div: div, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "warn_low", Name: "warn_low", Algo: module.Absolute, Div: div, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "warn_high", Name: "warn_high", Algo: module.Absolute, Div: div, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "warn_low_defined", Name: "warn_low_defined", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "warn_high_defined", Name: "warn_high_defined", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "warn_defined", Name: "warn_defined", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "warn_inclusive", Name: "warn_inclusive", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "crit_low", Name: "crit_low", Algo: module.Absolute, Div: div, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "crit_high", Name: "crit_high", Algo: module.Absolute, Div: div, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "crit_low_defined", Name: "crit_low_defined", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "crit_high_defined", Name: "crit_high_defined", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "crit_defined", Name: "crit_defined", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
		{ID: "crit_inclusive", Name: "crit_inclusive", Algo: module.Absolute, DimOpts: module.DimOpts{Hidden: true}},
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

func schedulerChartBase(shard string) module.Chart {
	return module.Chart{
		Fam:  "nagios_scheduler",
		Ctx:  fmt.Sprintf("%s.scheduler", ctxPrefix),
		Type: module.Line,
		Labels: []module.Label{
			{Key: "nagios_job", Value: "scheduler", Source: module.LabelSourceConf},
			{Key: "nagios_shard", Value: shard, Source: module.LabelSourceConf},
			{Key: "nagios_cmdline", Value: "scheduler", Source: module.LabelSourceConf},
		},
	}
}
