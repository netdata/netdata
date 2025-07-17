// SPDX-License-Identifier: GPL-3.0-or-later

package as400

import (
	"fmt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func (a *AS400) newActiveJobCharts(job *activeJobMetrics) *module.Charts {
	charts := module.Charts{
		activeJobCPUChartTmpl.Copy(),
		activeJobResourcesChartTmpl.Copy(),
		activeJobTimeChartTmpl.Copy(),
		activeJobActivityChartTmpl.Copy(),
		activeJobThreadsChartTmpl.Copy(),
	}

	cleanJobName := cleanName(job.jobName)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanJobName)
		chart.Labels = []module.Label{
			{Key: "job_name", Value: job.jobName},
			{Key: "job_status", Value: job.jobStatus},
			{Key: "subsystem", Value: job.subsystem},
			{Key: "job_type", Value: job.jobType},
			{Key: "run_priority", Value: fmt.Sprintf("%d", job.runPriority)},
			{Key: "ibmi_version", Value: a.osVersion},
			{Key: "system_name", Value: a.systemName},
			{Key: "serial_number", Value: a.serialNumber},
			{Key: "model", Value: a.model},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanJobName)
		}
	}

	return &charts
}

func (a *AS400) addActiveJobCharts(job *activeJobMetrics) {
	charts := a.newActiveJobCharts(job)
	
	if err := a.charts.Add(*charts...); err != nil {
		a.Warningf("failed to add active job charts for %s: %v", job.jobName, err)
	}
}

func (a *AS400) removeActiveJobCharts(jobName string) {
	cleanName := cleanName(jobName)
	
	// Mark all charts for this job as obsolete
	for _, chart := range *a.charts {
		if chart.ID == fmt.Sprintf("activejob_%s_cpu", cleanName) ||
		   chart.ID == fmt.Sprintf("activejob_%s_resources", cleanName) ||
		   chart.ID == fmt.Sprintf("activejob_%s_time", cleanName) ||
		   chart.ID == fmt.Sprintf("activejob_%s_activity", cleanName) ||
		   chart.ID == fmt.Sprintf("activejob_%s_threads", cleanName) {
			if !chart.Obsolete {
				chart.Obsolete = true
				chart.MarkNotCreated()
				a.Debugf("Marked chart %s as obsolete", chart.ID)
			}
		}
	}
}