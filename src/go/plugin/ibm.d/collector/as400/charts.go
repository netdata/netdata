// SPDX-License-Identifier: GPL-3.0-or-later

package as400

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var (
	baseCharts = module.Charts{
		cpuUtilizationChart.Copy(),
		activeJobsChart.Copy(),
		systemAspUsageChart.Copy(),
		memoryPoolUsageChart.Copy(),
		diskBusyChart.Copy(),
		jobQueueLengthChart.Copy(),
	}
)

var (
	cpuUtilizationChart = module.Chart{
		ID:       "cpu_utilization",
		Title:    "CPU Utilization",
		Units:    "percentage",
		Fam:      "cpu",
		Ctx:      "as400.cpu_utilization",
		Priority: module.Priority,
		Dims: module.Dims{
			{ID: "cpu_percentage", Name: "utilization"},
		},
	}

	activeJobsChart = module.Chart{
		ID:       "active_jobs",
		Title:    "Active Jobs",
		Units:    "jobs",
		Fam:      "jobs",
		Ctx:      "as400.active_jobs",
		Priority: module.Priority + 10,
		Dims: module.Dims{
			{ID: "active_jobs_count", Name: "active"},
		},
	}

	systemAspUsageChart = module.Chart{
		ID:       "system_asp_usage",
		Title:    "System ASP Usage",
		Units:    "percentage",
		Fam:      "storage",
		Ctx:      "as400.system_asp_usage",
		Priority: module.Priority + 20,
		Dims: module.Dims{
			{ID: "system_asp_used", Name: "used"},
		},
	}

	memoryPoolUsageChart = module.Chart{
		ID:       "memory_pool_usage",
		Title:    "Memory Pool Usage",
		Units:    "megabytes",
		Fam:      "memory",
		Ctx:      "as400.memory_pool_usage",
		Priority: module.Priority + 30,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "machine_pool_size", Name: "machine", Div: 1048576}, // Convert to MB
			{ID: "base_pool_size", Name: "base", Div: 1048576},
			{ID: "interactive_pool_size", Name: "interactive", Div: 1048576},
			{ID: "spool_pool_size", Name: "spool", Div: 1048576},
		},
	}

	diskBusyChart = module.Chart{
		ID:       "disk_busy",
		Title:    "Disk Busy Percentage",
		Units:    "percentage",
		Fam:      "disk",
		Ctx:      "as400.disk_busy",
		Priority: module.Priority + 40,
		Dims: module.Dims{
			{ID: "disk_busy_percentage", Name: "busy"},
		},
	}

	jobQueueLengthChart = module.Chart{
		ID:       "job_queue_length",
		Title:    "Job Queue Length",
		Units:    "jobs",
		Fam:      "jobs",
		Ctx:      "as400.job_queue_length",
		Priority: module.Priority + 50,
		Dims: module.Dims{
			{ID: "job_queue_length", Name: "queued"},
		},
	}
)
