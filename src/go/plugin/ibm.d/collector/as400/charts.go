// SPDX-License-Identifier: GPL-3.0-or-later

package as400

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var (
	baseCharts = module.Charts{
		// CPU charts
		cpuUtilizationChart.Copy(),
		cpuDetailsChart.Copy(),
		cpuCapacityChart.Copy(),
		
		// Job charts
		totalJobsChart.Copy(),
		activeJobsByTypeChart.Copy(),
		jobQueueLengthChart.Copy(),
		
		// Memory charts
		mainStorageSizeChart.Copy(),
		temporaryStorageChart.Copy(),
		memoryPoolUsageChart.Copy(),
		memoryPoolDefinedChart.Copy(),
		memoryPoolReservedChart.Copy(),
		memoryPoolThreadsChart.Copy(),
		memoryPoolMaxThreadsChart.Copy(),
		
		// Storage charts
		systemAspUsageChart.Copy(),
		systemAspStorageChart.Copy(),
		totalAuxiliaryStorageChart.Copy(),
		
		// Thread charts
		systemThreadsChart.Copy(),
		
		// Disk aggregate charts
		diskBusyChart.Copy(),
		
		// Network charts
		networkConnectionsChart.Copy(),
		networkConnectionStatesChart.Copy(),
		
		// Temporary storage charts
		tempStorageTotalChart.Copy(),
		
		// System activity charts
		systemActivityCPURateChart.Copy(),
		systemActivityCPUUtilizationChart.Copy(),
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
		Title:    "CPU Configuration",
		Units:    "cpus",
		Fam:      "cpu",
		Ctx:      "as400.cpu_configuration",
		Priority: module.Priority + 10,
		Dims: module.Dims{
			{ID: "configured_cpus", Name: "configured"},
		},
	}

	cpuCapacityChart = module.Chart{
		ID:       "cpu_capacity",
		Title:    "Current CPU Capacity",
		Units:    "percentage",
		Fam:      "cpu",
		Ctx:      "as400.cpu_capacity",
		Priority: module.Priority + 20,
		Dims: module.Dims{
			{ID: "current_cpu_capacity", Name: "capacity", Div: precision},
		},
	}

	// Job charts
	totalJobsChart = module.Chart{
		ID:       "total_jobs",
		Title:    "Total Jobs in System",
		Units:    "jobs",
		Fam:      "jobs",
		Ctx:      "as400.total_jobs",
		Priority: module.Priority + 100,
		Dims: module.Dims{
			{ID: "total_jobs_in_system", Name: "total"},
		},
	}

	activeJobsByTypeChart = module.Chart{
		ID:       "active_jobs_by_type",
		Title:    "Active Jobs by Type",
		Units:    "jobs",
		Fam:      "jobs",
		Ctx:      "as400.active_jobs_by_type",
		Priority: module.Priority + 110,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "batch_jobs_running", Name: "batch"},
			{ID: "interactive_jobs_in_system", Name: "interactive"},
			{ID: "active_jobs_in_system", Name: "active"},
		},
	}

	systemAspUsageChart = module.Chart{
		ID:       "system_asp_usage",
		Title:    "System ASP Usage",
		Units:    "percentage",
		Fam:      "storage",
		Ctx:      "as400.system_asp_usage",
		Priority: module.Priority + 40,
		Dims: module.Dims{
			{ID: "system_asp_used", Name: "used", Div: precision},
		},
	}

	memoryPoolUsageChart = module.Chart{
		ID:       "memory_pool_usage",
		Title:    "Memory Pool Usage",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "as400.memory_pool_usage",
		Priority: module.Priority + 50,
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
		Title:    "Memory Pool Defined Size",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "as400.memory_pool_defined",
		Priority: module.Priority + 60,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "machine_pool_defined_size", Name: "machine"},
			{ID: "base_pool_defined_size", Name: "base"},
		},
	}

	memoryPoolReservedChart = module.Chart{
		ID:       "memory_pool_reserved",
		Title:    "Memory Pool Reserved Size",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "as400.memory_pool_reserved",
		Priority: module.Priority + 70,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "machine_pool_reserved_size", Name: "machine"},
			{ID: "base_pool_reserved_size", Name: "base"},
		},
	}

	diskBusyChart = module.Chart{
		ID:       "disk_busy_average",
		Title:    "Average Disk Busy Percentage",
		Units:    "percentage",
		Fam:      "disk_aggregate",
		Ctx:      "as400.disk_busy_average",
		Priority: module.Priority + 80,
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
		Priority: module.Priority + 120,
		Dims: module.Dims{
			{ID: "job_queue_length", Name: "waiting"},
		},
	}

	// Memory charts
	mainStorageSizeChart = module.Chart{
		ID:       "main_storage_size",
		Title:    "Main Storage Size",
		Units:    "KiB",
		Fam:      "memory",
		Ctx:      "as400.main_storage_size",
		Priority: module.Priority + 200,
		Dims: module.Dims{
			{ID: "main_storage_size", Name: "total"},
		},
	}

	temporaryStorageChart = module.Chart{
		ID:       "temporary_storage",
		Title:    "Temporary Storage",
		Units:    "MiB",
		Fam:      "memory",
		Ctx:      "as400.temporary_storage",
		Priority: module.Priority + 210,
		Dims: module.Dims{
			{ID: "current_temporary_storage", Name: "current"},
			{ID: "maximum_temporary_storage_used", Name: "maximum"},
		},
	}

	memoryPoolThreadsChart = module.Chart{
		ID:       "memory_pool_threads",
		Title:    "Memory Pool Threads",
		Units:    "threads",
		Fam:      "memory",
		Ctx:      "as400.memory_pool_threads",
		Priority: module.Priority + 280,
		Dims: module.Dims{
			{ID: "machine_pool_threads", Name: "machine"},
			{ID: "base_pool_threads", Name: "base"},
		},
	}

	memoryPoolMaxThreadsChart = module.Chart{
		ID:       "memory_pool_max_threads",
		Title:    "Memory Pool Maximum Active Threads",
		Units:    "threads",
		Fam:      "memory",
		Ctx:      "as400.memory_pool_max_threads",
		Priority: module.Priority + 290,
		Dims: module.Dims{
			{ID: "machine_pool_max_threads", Name: "machine"},
			{ID: "base_pool_max_threads", Name: "base"},
		},
	}

	// Storage charts
	systemAspStorageChart = module.Chart{
		ID:       "system_asp_storage",
		Title:    "System ASP Storage",
		Units:    "MiB",
		Fam:      "storage",
		Ctx:      "as400.system_asp_storage",
		Priority: module.Priority + 310,
		Dims: module.Dims{
			{ID: "system_asp_storage", Name: "total"},
		},
	}

	totalAuxiliaryStorageChart = module.Chart{
		ID:       "total_auxiliary_storage",
		Title:    "Total Auxiliary Storage",
		Units:    "MiB",
		Fam:      "storage",
		Ctx:      "as400.total_auxiliary_storage",
		Priority: module.Priority + 320,
		Dims: module.Dims{
			{ID: "total_auxiliary_storage", Name: "total"},
		},
	}

	// Thread charts
	systemThreadsChart = module.Chart{
		ID:       "system_threads",
		Title:    "System Threads",
		Units:    "threads",
		Fam:      "threads",
		Ctx:      "as400.system_threads",
		Priority: module.Priority + 400,
		Dims: module.Dims{
			{ID: "active_threads_in_system", Name: "active"},
			{ID: "threads_per_processor", Name: "per_processor"},
		},
	}

	// Network charts
	networkConnectionsChart = module.Chart{
		ID:       "network_connections",
		Title:    "Network Connections",
		Units:    "connections",
		Fam:      "network",
		Ctx:      "as400.network_connections",
		Priority: module.Priority + 500,
		Dims: module.Dims{
			{ID: "remote_connections", Name: "remote"},
			{ID: "total_connections", Name: "total"},
		},
	}

	networkConnectionStatesChart = module.Chart{
		ID:       "network_connection_states",
		Title:    "Network Connection States",
		Units:    "connections",
		Fam:      "network",
		Ctx:      "as400.network_connection_states",
		Priority: module.Priority + 510,
		Dims: module.Dims{
			{ID: "listen_connections", Name: "listen"},
			{ID: "closewait_connections", Name: "close_wait"},
		},
	}

	// Temporary storage charts
	tempStorageTotalChart = module.Chart{
		ID:       "temp_storage_total",
		Title:    "Temporary Storage Total",
		Units:    "bytes",
		Fam:      "temp_storage_total",
		Ctx:      "as400.temp_storage_total",
		Priority: module.Priority + 600,
		Dims: module.Dims{
			{ID: "temp_storage_current_total", Name: "current"},
			{ID: "temp_storage_peak_total", Name: "peak"},
		},
	}
)

// Dynamic chart creation function for temp storage

func (a *AS400) newTempStorageCharts(bucket *tempStorageMetrics) *module.Charts {
	charts := module.Charts{
		{
			ID:       "tempstorage_" + cleanName(bucket.name) + "_size",
			Title:    "Temporary Storage Bucket Usage",
			Units:    "bytes",
			Fam:      "temp_storage_buckets",
			Ctx:      "as400.temp_storage_bucket",
			Priority: module.Priority + 4000,
			Labels: []module.Label{
				{Key: "bucket", Value: bucket.name},
			},
			Dims: module.Dims{
				{ID: "tempstorage_" + cleanName(bucket.name) + "_current_size", Name: "current"},
				{ID: "tempstorage_" + cleanName(bucket.name) + "_peak_size", Name: "peak"},
			},
		},
	}

	return &charts
}