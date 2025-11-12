// SPDX-License-Identifier: GPL-3.0-or-later

package charts

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

func BuildJobCharts(shard, jobName string, basePriority int) []*module.Chart {
	return []*module.Chart{
		StateChart(shard, jobName, basePriority),
		RuntimeChart(shard, jobName, basePriority+1),
		LatencyChart(shard, jobName, basePriority+2),
		CPUChart(shard, jobName, basePriority+3),
		MemoryChart(shard, jobName, basePriority+4),
		DiskChart(shard, jobName, basePriority+5),
	}
}

func BuildSchedulerCharts(shard string, basePriority int) []*module.Chart {
	return []*module.Chart{
		SchedulerQueueChart(shard, basePriority),
		SchedulerSkippedChart(shard, basePriority+1),
		SchedulerNextRunChart(shard, basePriority+2),
	}
}
