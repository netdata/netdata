// SPDX-License-Identifier: GPL-3.0-or-later

package charts

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

func BuildJobCharts(meta JobIdentity, basePriority int) []*module.Chart {
	return []*module.Chart{
		StateChart(meta, basePriority),
		RuntimeChart(meta, basePriority+1),
		LatencyChart(meta, basePriority+2),
		CPUChart(meta, basePriority+3),
		MemoryChart(meta, basePriority+4),
		DiskChart(meta, basePriority+5),
	}
}

func BuildSchedulerCharts(shard string, basePriority int) []*module.Chart {
	return []*module.Chart{
		SchedulerJobsChart(shard, basePriority),
		SchedulerRateChart(shard, basePriority+1),
		SchedulerNextRunChart(shard, basePriority+2),
	}
}
