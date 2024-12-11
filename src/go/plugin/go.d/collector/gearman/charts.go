// SPDX-License-Identifier: GPL-3.0-or-later

package gearman

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioQueuedJobsByActivity = module.Priority + iota
	prioQueuedJobsByPriority

	prioFunctionQueuedJobsByActivity
	prioFunctionQueuedJobsByPriority
	prioFunctionAvailableWorkers
)

var summaryCharts = module.Charts{
	chartQueuedJobsActivity.Copy(),
	chartQueuedJobsPriority.Copy(),
}

var (
	chartQueuedJobsActivity = module.Chart{
		ID:       "queued_jobs_by_activity",
		Title:    "Jobs Activity",
		Units:    "jobs",
		Fam:      "jobs",
		Ctx:      "gearman.queued_jobs_activity",
		Priority: prioQueuedJobsByActivity,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "total_jobs_running", Name: "running"},
			{ID: "total_jobs_waiting", Name: "waiting"},
		},
	}
	chartQueuedJobsPriority = module.Chart{
		ID:       "queued_jobs_by_priority",
		Title:    "Jobs Priority",
		Units:    "jobs",
		Fam:      "jobs",
		Ctx:      "gearman.queued_jobs_priority",
		Priority: prioQueuedJobsByPriority,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "total_high_priority_jobs", Name: "high"},
			{ID: "total_normal_priority_jobs", Name: "normal"},
			{ID: "total_low_priority_jobs", Name: "low"},
		},
	}
)

var functionStatusChartsTmpl = module.Charts{
	functionQueuedJobsActivityChartTmpl.Copy(),
	functionWorkersChartTmpl.Copy(),
}

var (
	functionQueuedJobsActivityChartTmpl = module.Chart{
		ID:       "function_%s_queued_jobs_by_activity",
		Title:    "Function Jobs Activity",
		Units:    "jobs",
		Fam:      "fn jobs",
		Ctx:      "gearman.function_queued_jobs_activity",
		Priority: prioFunctionQueuedJobsByActivity,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "function_%s_jobs_running", Name: "running"},
			{ID: "function_%s_jobs_waiting", Name: "waiting"},
		},
	}
	functionWorkersChartTmpl = module.Chart{
		ID:       "function_%s_workers",
		Title:    "Function Workers",
		Units:    "workers",
		Fam:      "fn workers",
		Ctx:      "gearman.function_workers",
		Priority: prioFunctionAvailableWorkers,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "function_%s_workers_available", Name: "available"},
		},
	}
)

var functionPriorityStatusChartsTmpl = module.Charts{
	functionQueuedJobsByPriorityChartTmpl.Copy(),
}

var (
	functionQueuedJobsByPriorityChartTmpl = module.Chart{
		ID:       "prio_function_%s_queued_jobs_by_priority",
		Title:    "Function Jobs Priority",
		Units:    "jobs",
		Fam:      "fn jobs",
		Ctx:      "gearman.function_queued_jobs_priority",
		Priority: prioFunctionQueuedJobsByPriority,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "function_%s_high_priority_jobs", Name: "high"},
			{ID: "function_%s_normal_priority_jobs", Name: "normal"},
			{ID: "function_%s_low_priority_jobs", Name: "low"},
		},
	}
)

func (c *Collector) addFunctionStatusCharts(name string) {
	c.addFunctionCharts(name, functionStatusChartsTmpl.Copy())
}

func (c *Collector) removeFunctionStatusCharts(name string) {
	px := fmt.Sprintf("function_%s_", cleanFunctionName(name))
	c.removeCharts(px)
}

func (c *Collector) addFunctionPriorityStatusCharts(name string) {
	c.addFunctionCharts(name, functionPriorityStatusChartsTmpl.Copy())
}

func (c *Collector) removeFunctionPriorityStatusCharts(name string) {
	px := fmt.Sprintf("prio_function_%s_", cleanFunctionName(name))
	c.removeCharts(px)
}

func (c *Collector) addFunctionCharts(name string, charts *module.Charts) {
	charts = charts.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanFunctionName(name))
		chart.Labels = []module.Label{
			{Key: "function_name", Value: name},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, name)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeCharts(px string) {
	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func cleanFunctionName(name string) string {
	r := strings.NewReplacer(".", "_", ",", "_", " ", "_")
	return r.Replace(name)
}
