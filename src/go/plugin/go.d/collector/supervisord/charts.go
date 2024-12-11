// SPDX-License-Identifier: GPL-3.0-or-later

package supervisord

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	summaryChartsPriority = module.Priority
	groupChartsPriority   = summaryChartsPriority + 20
)

var summaryCharts = module.Charts{
	{
		ID:       "processes",
		Title:    "Processes",
		Units:    "processes",
		Fam:      "summary",
		Ctx:      "supervisord.summary_processes",
		Type:     module.Stacked,
		Priority: summaryChartsPriority,
		Dims: module.Dims{
			{ID: "running_processes", Name: "running"},
			{ID: "non_running_processes", Name: "non-running"},
		},
	},
}

var (
	groupChartsTmpl = module.Charts{
		groupProcessesChartTmpl.Copy(),
		groupProcessesStateCodeChartTmpl.Copy(),
		groupProcessesExitStatusChartTmpl.Copy(),
		groupProcessesUptimeChartTmpl.Copy(),
		groupProcessesDowntimeChartTmpl.Copy(),
	}

	groupProcessesChartTmpl = module.Chart{
		ID:    "group_%s_processes",
		Title: "Processes",
		Units: "processes",
		Fam:   "group %s",
		Ctx:   "supervisord.processes",
		Type:  module.Stacked,
		Dims: module.Dims{
			{ID: "group_%s_running_processes", Name: "running"},
			{ID: "group_%s_non_running_processes", Name: "non-running"},
		},
	}
	groupProcessesStateCodeChartTmpl = module.Chart{
		ID:    "group_%s_processes_state_code",
		Title: "State code",
		Units: "code",
		Fam:   "group %s",
		Ctx:   "supervisord.process_state_code",
	}
	groupProcessesExitStatusChartTmpl = module.Chart{
		ID:    "group_%s_processes_exit_status",
		Title: "Exit status",
		Units: "status",
		Fam:   "group %s",
		Ctx:   "supervisord.process_exit_status",
	}
	groupProcessesUptimeChartTmpl = module.Chart{
		ID:    "group_%s_processes_uptime",
		Title: "Uptime",
		Units: "seconds",
		Fam:   "group %s",
		Ctx:   "supervisord.process_uptime",
	}
	groupProcessesDowntimeChartTmpl = module.Chart{
		ID:    "group_%s_processes_downtime",
		Title: "Downtime",
		Units: "seconds",
		Fam:   "group %s",
		Ctx:   "supervisord.process_downtime",
	}
)

func newProcGroupCharts(group string) *module.Charts {
	charts := groupChartsTmpl.Copy()
	for i, c := range *charts {
		c.ID = fmt.Sprintf(c.ID, group)
		c.Fam = fmt.Sprintf(c.Fam, group)
		c.Priority = groupChartsPriority + i
		for _, d := range c.Dims {
			d.ID = fmt.Sprintf(d.ID, group)
		}
	}
	return charts
}
