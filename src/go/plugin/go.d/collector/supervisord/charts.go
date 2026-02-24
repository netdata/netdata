// SPDX-License-Identifier: GPL-3.0-or-later

package supervisord

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	summaryChartsPriority = collectorapi.Priority
	groupChartsPriority   = summaryChartsPriority + 20
)

var summaryCharts = collectorapi.Charts{
	{
		ID:       "processes",
		Title:    "Processes",
		Units:    "processes",
		Fam:      "summary",
		Ctx:      "supervisord.summary_processes",
		Type:     collectorapi.Stacked,
		Priority: summaryChartsPriority,
		Dims: collectorapi.Dims{
			{ID: "running_processes", Name: "running"},
			{ID: "non_running_processes", Name: "non-running"},
		},
	},
}

var (
	groupChartsTmpl = collectorapi.Charts{
		groupProcessesChartTmpl.Copy(),
		groupProcessesStateCodeChartTmpl.Copy(),
		groupProcessesExitStatusChartTmpl.Copy(),
		groupProcessesUptimeChartTmpl.Copy(),
		groupProcessesDowntimeChartTmpl.Copy(),
	}

	groupProcessesChartTmpl = collectorapi.Chart{
		ID:    "group_%s_processes",
		Title: "Processes",
		Units: "processes",
		Fam:   "group %s",
		Ctx:   "supervisord.processes",
		Type:  collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "group_%s_running_processes", Name: "running"},
			{ID: "group_%s_non_running_processes", Name: "non-running"},
		},
	}
	groupProcessesStateCodeChartTmpl = collectorapi.Chart{
		ID:    "group_%s_processes_state_code",
		Title: "State code",
		Units: "code",
		Fam:   "group %s",
		Ctx:   "supervisord.process_state_code",
	}
	groupProcessesExitStatusChartTmpl = collectorapi.Chart{
		ID:    "group_%s_processes_exit_status",
		Title: "Exit status",
		Units: "status",
		Fam:   "group %s",
		Ctx:   "supervisord.process_exit_status",
	}
	groupProcessesUptimeChartTmpl = collectorapi.Chart{
		ID:    "group_%s_processes_uptime",
		Title: "Uptime",
		Units: "seconds",
		Fam:   "group %s",
		Ctx:   "supervisord.process_uptime",
	}
	groupProcessesDowntimeChartTmpl = collectorapi.Chart{
		ID:    "group_%s_processes_downtime",
		Title: "Downtime",
		Units: "seconds",
		Fam:   "group %s",
		Ctx:   "supervisord.process_downtime",
	}
)

func newProcGroupCharts(group string) *collectorapi.Charts {
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
