// SPDX-License-Identifier: GPL-3.0-or-later

package as400

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var (
	baseCharts = module.Charts{
		cpuUtilizationChart.Copy(),
		activeJobsChart.Copy(),
		jobTypeBreakdownChart.Copy(),
		systemAspUsageChart.Copy(),
		ifsUsageChart.Copy(),
		ifsFilesChart.Copy(),
		memoryPoolUsageChart.Copy(),
		diskBusyChart.Copy(),
		jobQueueLengthChart.Copy(),
		messageQueueDepthChart.Copy(),
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
			{ID: "cpu_percentage", Name: "utilization", Div: precision},
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

	jobTypeBreakdownChart = module.Chart{
		ID:       "job_type_breakdown",
		Title:    "Jobs by Type",
		Units:    "jobs",
		Fam:      "jobs",
		Ctx:      "as400.job_type_breakdown",
		Priority: module.Priority + 15,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "batch_jobs", Name: "batch"},
			{ID: "interactive_jobs", Name: "interactive"},
			{ID: "system_jobs", Name: "system"},
			{ID: "spooled_jobs", Name: "spooled"},
			{ID: "other_jobs", Name: "other"},
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
			{ID: "system_asp_used", Name: "used", Div: precision},
		},
	}

	ifsUsageChart = module.Chart{
		ID:       "ifs_usage",
		Title:    "IFS Usage",
		Units:    "bytes",
		Fam:      "storage",
		Ctx:      "as400.ifs_usage",
		Priority: module.Priority + 25,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "ifs_used_size", Name: "used"},
			{ID: "ifs_total_size", Name: "total"},
		},
	}

	ifsFilesChart = module.Chart{
		ID:       "ifs_files",
		Title:    "IFS File Count",
		Units:    "files",
		Fam:      "storage",
		Ctx:      "as400.ifs_files",
		Priority: module.Priority + 26,
		Dims: module.Dims{
			{ID: "ifs_file_count", Name: "files"},
		},
	}

	memoryPoolUsageChart = module.Chart{
		ID:       "memory_pool_usage",
		Title:    "Memory Pool Usage",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "as400.memory_pool_usage",
		Priority: module.Priority + 30,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "machine_pool_size", Name: "machine"},
			{ID: "base_pool_size", Name: "base"},
			{ID: "interactive_pool_size", Name: "interactive"},
			{ID: "spool_pool_size", Name: "spool"},
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
			{ID: "disk_busy_percentage", Name: "busy", Div: precision},
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

	messageQueueDepthChart = module.Chart{
		ID:       "message_queue_depth",
		Title:    "System Message Queue Depth",
		Units:    "messages",
		Fam:      "system",
		Ctx:      "as400.message_queue_depth",
		Priority: module.Priority + 60,
		Dims: module.Dims{
			{ID: "system_message_queue_depth", Name: "system"},
			{ID: "qsysopr_message_queue_depth", Name: "qsysopr"},
		},
	}
)
