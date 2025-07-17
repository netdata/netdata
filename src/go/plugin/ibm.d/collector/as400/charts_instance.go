// SPDX-License-Identifier: GPL-3.0-or-later

package as400

import (
	"fmt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// Chart templates for per-instance metrics
var (
	// Disk charts
	diskBusyChartTmpl = module.Chart{
		ID:       "disk_%s_busy",
		Title:    "Disk %s Busy Percentage",
		Units:    "percentage",
		Fam:      "disk",
		Ctx:      "as400.disk_busy",
		Priority: module.Priority + 100,
		Dims: module.Dims{
			{ID: "disk_%s_busy_percent", Name: "busy", Div: precision},
		},
	}

	diskIORequestsChartTmpl = module.Chart{
		ID:       "disk_%s_io_requests",
		Title:    "Disk %s I/O Requests",
		Units:    "requests/s",
		Fam:      "disk",
		Ctx:      "as400.disk_io_requests",
		Priority: module.Priority + 101,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "disk_%s_read_requests", Name: "read", Algo: module.Incremental},
			{ID: "disk_%s_write_requests", Name: "write", Algo: module.Incremental, Mul: -1},
		},
	}

	diskSpaceUsageChartTmpl = module.Chart{
		ID:       "disk_%s_space_usage",
		Title:    "Disk %s Space Usage",
		Units:    "percentage",
		Fam:      "disk",
		Ctx:      "as400.disk_space_usage",
		Priority: module.Priority + 102,
		Dims: module.Dims{
			{ID: "disk_%s_percent_used", Name: "used", Div: precision},
		},
	}

	diskCapacityChartTmpl = module.Chart{
		ID:       "disk_%s_capacity",
		Title:    "Disk %s Capacity",
		Units:    "GB",
		Fam:      "disk",
		Ctx:      "as400.disk_capacity",
		Priority: module.Priority + 103,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "disk_%s_available_gb", Name: "available", Div: precision},
			{ID: "disk_%s_used_gb", Name: "used", Div: precision},
		},
	}

	diskBlocksChartTmpl = module.Chart{
		ID:       "disk_%s_blocks",
		Title:    "Disk %s Block Operations",
		Units:    "blocks/s",
		Fam:      "disk",
		Ctx:      "as400.disk_blocks",
		Priority: module.Priority + 104,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "disk_%s_blocks_read", Name: "read", Algo: module.Incremental},
			{ID: "disk_%s_blocks_written", Name: "write", Algo: module.Incremental, Mul: -1},
		},
	}

	diskSSDHealthChartTmpl = module.Chart{
		ID:       "disk_%s_ssd_health",
		Title:    "Disk %s SSD Health",
		Units:    "percentage",
		Fam:      "disk",
		Ctx:      "as400.disk_ssd_health",
		Priority: module.Priority + 105,
		Dims: module.Dims{
			{ID: "disk_%s_ssd_life_remaining", Name: "life_remaining"},
		},
	}

	diskSSDAgeChartTmpl = module.Chart{
		ID:       "disk_%s_ssd_age",
		Title:    "Disk %s SSD Power On Days",
		Units:    "days",
		Fam:      "disk",
		Ctx:      "as400.disk_ssd_age",
		Priority: module.Priority + 106,
		Dims: module.Dims{
			{ID: "disk_%s_ssd_power_on_days", Name: "power_on_days"},
		},
	}


	// Subsystem charts
	subsystemJobsChartTmpl = module.Chart{
		ID:       "subsystem_%s_jobs",
		Title:    "Subsystem %s Jobs",
		Units:    "jobs",
		Fam:      "subsystem",
		Ctx:      "as400.subsystem_jobs",
		Priority: module.Priority + 200,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "subsystem_%s_jobs_active", Name: "active"},
			{ID: "subsystem_%s_jobs_held", Name: "held"},
		},
	}

	subsystemStorageChartTmpl = module.Chart{
		ID:       "subsystem_%s_storage",
		Title:    "Subsystem %s Storage Usage",
		Units:    "bytes",
		Fam:      "subsystem",
		Ctx:      "as400.subsystem_storage",
		Priority: module.Priority + 201,
		Dims: module.Dims{
			{ID: "subsystem_%s_storage_used_kb", Name: "used", Mul: 1024},
		},
	}

	// Job queue charts
	jobQueueLengthChartTmpl = module.Chart{
		ID:       "jobqueue_%s_length",
		Title:    "Job Queue %s Length",
		Units:    "jobs",
		Fam:      "jobqueue",
		Ctx:      "as400.jobqueue_length",
		Priority: module.Priority + 300,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "jobqueue_%s_jobs_waiting", Name: "waiting"},
			{ID: "jobqueue_%s_jobs_held", Name: "held"},
			{ID: "jobqueue_%s_jobs_scheduled", Name: "scheduled"},
		},
	}

	// Active job charts
	activeJobCPUChartTmpl = module.Chart{
		ID:       "activejob_%s_cpu",
		Title:    "Active Job %s CPU Usage",
		Units:    "percentage",
		Fam:      "activejobs",
		Ctx:      "as400.activejob_cpu",
		Priority: module.Priority + 400,
		Dims: module.Dims{
			{ID: "activejob_%s_cpu_percentage", Name: "cpu", Div: precision},
		},
	}

	activeJobResourcesChartTmpl = module.Chart{
		ID:       "activejob_%s_resources",
		Title:    "Active Job %s Resource Usage",
		Units:    "MB",
		Fam:      "activejobs",
		Ctx:      "as400.activejob_resources",
		Priority: module.Priority + 401,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "activejob_%s_temporary_storage", Name: "temp_storage"},
		},
	}

	activeJobTimeChartTmpl = module.Chart{
		ID:       "activejob_%s_time",
		Title:    "Active Job %s Elapsed Time",
		Units:    "seconds",
		Fam:      "activejobs",
		Ctx:      "as400.activejob_time",
		Priority: module.Priority + 402,
		Dims: module.Dims{
			{ID: "activejob_%s_elapsed_cpu_time", Name: "cpu_time"},
			{ID: "activejob_%s_elapsed_time", Name: "total_time"},
		},
	}

	activeJobActivityChartTmpl = module.Chart{
		ID:       "activejob_%s_activity",
		Title:    "Active Job %s Activity",
		Units:    "operations/s",
		Fam:      "activejobs",
		Ctx:      "as400.activejob_activity",
		Priority: module.Priority + 403,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "activejob_%s_elapsed_disk_io", Name: "disk_io", Algo: module.Incremental},
			{ID: "activejob_%s_elapsed_interactive_transactions", Name: "interactive_transactions", Algo: module.Incremental, Mul: -1},
		},
	}

	activeJobThreadsChartTmpl = module.Chart{
		ID:       "activejob_%s_threads",
		Title:    "Active Job %s Thread Count",
		Units:    "threads",
		Fam:      "activejobs",
		Ctx:      "as400.activejob_threads",
		Priority: module.Priority + 404,
		Dims: module.Dims{
			{ID: "activejob_%s_thread_count", Name: "threads"},
		},
	}
)

// Chart creation functions
func (a *AS400) newDiskCharts(disk *diskMetrics) *module.Charts {
	charts := module.Charts{
		diskBusyChartTmpl.Copy(),
		diskIORequestsChartTmpl.Copy(),
		diskSpaceUsageChartTmpl.Copy(),
		diskCapacityChartTmpl.Copy(),
		diskBlocksChartTmpl.Copy(),
	}

	cleanUnit := cleanName(disk.unit)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanUnit)
		chart.Labels = []module.Label{
			{Key: "disk_unit", Value: disk.unit},
			{Key: "disk_type", Value: disk.typeField},
			{Key: "disk_model", Value: disk.diskModel},
			{Key: "hardware_status", Value: disk.hardwareStatus},
			{Key: "disk_serial_number", Value: disk.serialNumber},
			{Key: "ibmi_version", Value: a.osVersion},
			{Key: "system_name", Value: a.systemName},
			{Key: "serial_number", Value: a.serialNumber},
			{Key: "model", Value: a.model},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanUnit)
		}
	}

	// Add SSD-specific charts if this is an SSD
	if disk.ssdLifeRemaining >= 0 {
		ssdHealthChart := diskSSDHealthChartTmpl.Copy()
		ssdHealthChart.ID = fmt.Sprintf(ssdHealthChart.ID, cleanUnit)
		ssdHealthChart.Labels = []module.Label{
			{Key: "disk_unit", Value: disk.unit},
			{Key: "disk_type", Value: disk.typeField},
			{Key: "disk_model", Value: disk.diskModel},
			{Key: "hardware_status", Value: disk.hardwareStatus},
			{Key: "disk_serial_number", Value: disk.serialNumber},
			{Key: "ibmi_version", Value: a.osVersion},
			{Key: "system_name", Value: a.systemName},
			{Key: "serial_number", Value: a.serialNumber},
			{Key: "model", Value: a.model},
		}
		for _, dim := range ssdHealthChart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanUnit)
		}
		charts = append(charts, ssdHealthChart)
	}

	if disk.ssdPowerOnDays >= 0 {
		ssdAgeChart := diskSSDAgeChartTmpl.Copy()
		ssdAgeChart.ID = fmt.Sprintf(ssdAgeChart.ID, cleanUnit)
		ssdAgeChart.Labels = []module.Label{
			{Key: "disk_unit", Value: disk.unit},
			{Key: "disk_type", Value: disk.typeField},
			{Key: "disk_model", Value: disk.diskModel},
			{Key: "hardware_status", Value: disk.hardwareStatus},
			{Key: "disk_serial_number", Value: disk.serialNumber},
			{Key: "ibmi_version", Value: a.osVersion},
			{Key: "system_name", Value: a.systemName},
			{Key: "serial_number", Value: a.serialNumber},
			{Key: "model", Value: a.model},
		}
		for _, dim := range ssdAgeChart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanUnit)
		}
		charts = append(charts, ssdAgeChart)
	}

	return &charts
}

func (a *AS400) newSubsystemCharts(subsystem *subsystemMetrics) *module.Charts {
	charts := module.Charts{
		subsystemJobsChartTmpl.Copy(),
		subsystemStorageChartTmpl.Copy(),
	}

	cleanSubsystem := cleanName(subsystem.name)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanSubsystem)
		chart.Labels = []module.Label{
			{Key: "subsystem", Value: subsystem.name},
			{Key: "library", Value: subsystem.library},
			{Key: "status", Value: subsystem.status},
			{Key: "ibmi_version", Value: a.osVersion},
			{Key: "system_name", Value: a.systemName},
			{Key: "serial_number", Value: a.serialNumber},
			{Key: "model", Value: a.model},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanSubsystem)
		}
	}

	return &charts
}

func (a *AS400) newJobQueueCharts(jobQueue *jobQueueMetrics, key string) *module.Charts {
	charts := module.Charts{
		jobQueueLengthChartTmpl.Copy(),
	}

	cleanKey := cleanName(key)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanKey)
		chart.Labels = []module.Label{
			{Key: "job_queue", Value: jobQueue.name},
			{Key: "library", Value: jobQueue.library},
			{Key: "status", Value: jobQueue.status},
			{Key: "ibmi_version", Value: a.osVersion},
			{Key: "system_name", Value: a.systemName},
			{Key: "serial_number", Value: a.serialNumber},
			{Key: "model", Value: a.model},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanKey)
		}
	}

	return &charts
}
