// SPDX-License-Identifier: GPL-3.0-or-later

package as400

// defineQueries sets up all the version-adaptive query definitions
func (cm *CapabilityManager) defineQueries() {
	
	// Disk I/O Statistics - with legacy fallback
	cm.queries["disk_instances"] = &QueryDefinition{
		MetricName: "disk_instances",
		Variants: []QueryVariant{
			{
				Name:       "Modern Disk Stats",
				Query:      queryDiskInstances,
				MinVersion: "V7R3+",
				Tables:     []string{"DISK_STATUS"},
				Required:   false,
				Handler:    "collectDiskInstances",
			},
			{
				Name:       "Legacy Disk Stats",
				Query:      queryDiskInstancesLegacy,
				MinVersion: "V6R1+", 
				Tables:     []string{"SYSDISKSTAT"},
				Required:   false,
				Handler:    "collectDiskInstancesLegacy",
			},
		},
	}
	
	// Aggregate disk busy percentage
	cm.queries["disk_status"] = &QueryDefinition{
		MetricName: "disk_status",
		Variants: []QueryVariant{
			{
				Name:       "Modern Disk Status",
				Query:      queryDiskStatus,
				MinVersion: "V7R3+",
				Tables:     []string{"DISK_STATUS"},
				Required:   false,
				Handler:    "collectDiskStatus",
			},
			{
				Name:       "Legacy Disk Status",
				Query:      queryDiskStatusLegacy,
				MinVersion: "V6R1+",
				Tables:     []string{"SYSDISKSTAT"},
				Required:   false,
				Handler:    "collectDiskStatusLegacy",
			},
		},
	}
	
	// Job queue length - with fallback aggregation
	cm.queries["job_info"] = &QueryDefinition{
		MetricName: "job_info",
		Variants: []QueryVariant{
			{
				Name:       "Modern Job Info",
				Query:      queryJobInfo,
				MinVersion: "V7R3+",
				Tables:     []string{"JOB_INFO"},
				Required:   false,
				Handler:    "collectJobInfo",
			},
			{
				Name:       "Job Queue Aggregation",
				Query:      queryJobInfoFallback,
				MinVersion: "V6R1+",
				Tables:     []string{"JOB_QUEUE_INFO"},
				Required:   false,
				Handler:    "collectJobInfoFallback",
			},
		},
	}
	
	// Job type breakdown - with fallback
	cm.queries["job_type_breakdown"] = &QueryDefinition{
		MetricName: "job_type_breakdown",
		Variants: []QueryVariant{
			{
				Name:       "Table Function Job Types",
				Query:      queryJobTypeBreakdown,
				MinVersion: "V7R1+",
				Functions:  []string{"ACTIVE_JOB_INFO"},
				Required:   false,
				Handler:    "collectJobTypeBreakdown",
			},
			{
				Name:       "System Status Job Types",
				Query:      queryJobTypeBreakdownFallback,
				MinVersion: "V6R1+",
				Tables:     []string{"SYSTEM_STATUS_INFO"},
				Required:   false,
				Handler:    "collectJobTypeBreakdownFallback",
			},
		},
	}
	
	// Active job details - with fallback
	cm.queries["active_jobs"] = &QueryDefinition{
		MetricName: "active_jobs",
		Variants: []QueryVariant{
			{
				Name:       "Table Function Active Jobs",
				Query:      queryActiveJobsDetails,
				MinVersion: "V7R1+",
				Functions:  []string{"ACTIVE_JOB_INFO"},
				Required:   false,
				Handler:    "collectActiveJobs",
			},
			{
				Name:       "Subsystem Active Jobs",
				Query:      queryActiveJobsFallback,
				MinVersion: "V6R1+",
				Tables:     []string{"SUBSYSTEM_INFO"},
				Required:   false,
				Handler:    "collectActiveJobsFallback",
			},
		},
	}
	
	// IFS monitoring - with fallback
	cm.queries["ifs_usage"] = &QueryDefinition{
		MetricName: "ifs_usage",
		Variants: []QueryVariant{
			{
				Name:       "IFS Object Statistics",
				Query:      queryIFSUsage,
				MinVersion: "V7R1+",
				Functions:  []string{"IFS_OBJECT_STATISTICS"},
				Required:   false,
				Handler:    "collectIFSUsage",
			},
			{
				Name:       "ASP IFS Estimation",
				Query:      queryIFSUsageFallback,
				MinVersion: "V6R1+",
				Tables:     []string{"ASP_INFO"},
				Required:   false,
				Handler:    "collectIFSUsageFallback",
			},
		},
	}
	
	// Core system metrics - always required
	cm.queries["system_status"] = &QueryDefinition{
		MetricName: "system_status",
		Variants: []QueryVariant{
			{
				Name:       "System Status Info",
				Query:      "", // Special case - handled in collectSystemStatus
				MinVersion: "V6R1+",
				Tables:     []string{"SYSTEM_STATUS_INFO"},
				Required:   true,
				Handler:    "collectSystemStatus",
			},
		},
	}
	
	// Memory pools - always required
	cm.queries["memory_pools"] = &QueryDefinition{
		MetricName: "memory_pools",
		Variants: []QueryVariant{
			{
				Name:       "Memory Pool Info",
				Query:      queryMemoryPools,
				MinVersion: "V6R1+",
				Tables:     []string{"MEMORY_POOL_INFO"},
				Required:   true,
				Handler:    "collectMemoryPools",
			},
		},
	}
	
	// Network interface monitoring - new capability
	cm.queries["network_interfaces"] = &QueryDefinition{
		MetricName: "network_interfaces",
		Variants: []QueryVariant{
			{
				Name:       "Network Interface Stats",
				Query:      queryNetworkInterfaces,
				MinVersion: "V6R1+",
				Tables:     []string{"NETSTAT_INTERFACE_INFO"},
				Required:   false,
				Handler:    "collectNetworkInterfaces",
			},
		},
	}
	
	// Database performance monitoring - new capability
	cm.queries["database_performance"] = &QueryDefinition{
		MetricName: "database_performance",
		Variants: []QueryVariant{
			{
				Name:       "Table Statistics",
				Query:      queryDatabasePerformance,
				MinVersion: "V6R1+",
				Tables:     []string{"SYSTABLESTAT"},
				Required:   false,
				Handler:    "collectDatabasePerformance",
			},
		},
	}
	
	// Hardware resource monitoring - new capability
	cm.queries["hardware_resources"] = &QueryDefinition{
		MetricName: "hardware_resources",
		Variants: []QueryVariant{
			{
				Name:       "Hardware Resource Info",
				Query:      queryHardwareResources,
				MinVersion: "V6R1+",
				Tables:     []string{"HARDWARE_RESOURCE_INFO"},
				Required:   false,
				Handler:    "collectHardwareResources",
			},
		},
	}
}