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

	diskIOBytesChartTmpl = module.Chart{
		ID:       "disk_%s_io_bytes",
		Title:    "Disk %s I/O Throughput",
		Units:    "bytes/s",
		Fam:      "disk",
		Ctx:      "as400.disk_io_bytes",
		Priority: module.Priority + 102,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "disk_%s_read_bytes", Name: "read", Algo: module.Incremental},
			{ID: "disk_%s_write_bytes", Name: "write", Algo: module.Incremental, Mul: -1},
		},
	}

	diskAvgTimeChartTmpl = module.Chart{
		ID:       "disk_%s_avg_time",
		Title:    "Disk %s Average Response Time",
		Units:    "milliseconds",
		Fam:      "disk",
		Ctx:      "as400.disk_avg_time",
		Priority: module.Priority + 103,
		Dims: module.Dims{
			{ID: "disk_%s_average_time", Name: "response_time"},
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
)

// Chart creation functions
func (a *AS400) newDiskCharts(disk *diskMetrics) *module.Charts {
	charts := module.Charts{
		diskBusyChartTmpl.Copy(),
		diskIORequestsChartTmpl.Copy(),
		diskIOBytesChartTmpl.Copy(),
		diskAvgTimeChartTmpl.Copy(),
	}

	cleanUnit := cleanName(disk.unit)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanUnit)
		chart.Labels = []module.Label{
			{Key: "disk_unit", Value: disk.unit},
			{Key: "disk_type", Value: disk.typeField},
			{Key: "disk_model", Value: disk.model},
			{Key: "ibmi_version", Value: a.osVersion},
			{Key: "system_name", Value: a.systemName},
			{Key: "serial_number", Value: a.serialNumber},
			{Key: "model", Value: a.model},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanUnit)
		}
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
