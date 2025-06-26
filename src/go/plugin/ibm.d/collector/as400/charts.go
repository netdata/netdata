// SPDX-License-Identifier: GPL-3.0-or-later

package as400

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var (
	baseCharts = module.Charts{
		cpuUtilizationChart.Copy(),
		cpuDetailsChart.Copy(),
		cpuByTypeChart.Copy(),
		activeJobsChart.Copy(),
		jobTypeBreakdownChart.Copy(),
		systemAspUsageChart.Copy(),
		ifsUsageChart.Copy(),
		ifsFilesChart.Copy(),
		memoryPoolUsageChart.Copy(),
		memoryPoolDefinedChart.Copy(),
		memoryPoolReservedChart.Copy(),
		diskBusyChart.Copy(),
		jobQueueLengthChart.Copy(),
		messageQueueDepthChart.Copy(),
		messageQueueCriticalChart.Copy(),
		ifsDirectoryUsageChart.Copy(),
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

	cpuDetailsChart = module.Chart{
		ID:       "cpu_details",
		Title:    "CPU Configuration and Capacity",
		Units:    "cpus",
		Fam:      "cpu",
		Ctx:      "as400.cpu_details",
		Priority: module.Priority + 1,
		Dims: module.Dims{
			{ID: "configured_cpus", Name: "configured"},
			{ID: "current_processing_capacity", Name: "capacity", Div: precision},
		},
	}

	cpuByTypeChart = module.Chart{
		ID:       "cpu_by_type",
		Title:    "CPU Utilization by Type",
		Units:    "percentage",
		Fam:      "cpu",
		Ctx:      "as400.cpu_by_type",
		Priority: module.Priority + 2,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "partition_cpu_utilization", Name: "partition", Div: precision},
			{ID: "interactive_cpu_utilization", Name: "interactive", Div: precision},
			{ID: "database_cpu_utilization", Name: "database", Div: precision},
			{ID: "shared_processor_pool_usage", Name: "shared_pool", Div: precision},
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

	ifsDirectoryUsageChart = module.Chart{
		ID:       "ifs_directory_usage",
		Title:    "IFS Directory Usage",
		Units:    "bytes",
		Fam:      "storage",
		Ctx:      "as400.ifs_directory_usage",
		Priority: module.Priority + 27,
		Type:     module.Stacked,
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

	memoryPoolDefinedChart = module.Chart{
		ID:       "memory_pool_defined",
		Title:    "Memory Pool Defined Sizes",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "as400.memory_pool_defined",
		Priority: module.Priority + 31,
		Dims: module.Dims{
			{ID: "machine_pool_defined_size", Name: "machine"},
			{ID: "base_pool_defined_size", Name: "base"},
		},
	}

	memoryPoolReservedChart = module.Chart{
		ID:       "memory_pool_reserved",
		Title:    "Memory Pool Reserved Sizes",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "as400.memory_pool_reserved",
		Priority: module.Priority + 32,
		Dims: module.Dims{
			{ID: "machine_pool_reserved_size", Name: "machine"},
			{ID: "base_pool_reserved_size", Name: "base"},
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

	messageQueueCriticalChart = module.Chart{
		ID:       "message_queue_critical",
		Title:    "Critical Messages in System Queues",
		Units:    "messages",
		Fam:      "system",
		Ctx:      "as400.message_queue_critical",
		Priority: module.Priority + 61,
		Dims: module.Dims{
			{ID: "system_critical_messages", Name: "system"},
			{ID: "qsysopr_critical_messages", Name: "qsysopr"},
		},
	}
)
