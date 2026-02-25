// SPDX-License-Identifier: GPL-3.0-or-later

package charts

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

func BuildJobCharts(meta JobIdentity, basePriority int) []*collectorapi.Chart {
	return []*collectorapi.Chart{
		StateChart(meta, basePriority),
		RuntimeChart(meta, basePriority+1),
		LatencyChart(meta, basePriority+2),
		CPUChart(meta, basePriority+3),
		MemoryChart(meta, basePriority+4),
		DiskChart(meta, basePriority+5),
	}
}

func BuildSchedulerCharts(scheduler string, basePriority int) []*collectorapi.Chart {
	return []*collectorapi.Chart{
		SchedulerJobsChart(scheduler, basePriority),
		SchedulerRateChart(scheduler, basePriority+1),
		SchedulerNextRunChart(scheduler, basePriority+2),
	}
}
