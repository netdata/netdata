// SPDX-License-Identifier: GPL-3.0-or-later

package beanstalk

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioCurrentJobs = module.Priority + iota
	prioJobsRate
	prioJobsTimeouts

	prioCurrentTubes

	prioCommandsRate

	prioCurrentConnections
	prioConnectionsRate

	prioBinlogRecords

	prioCpuUsage

	prioUptime

	prioTubeCurrentJobs
	prioTubeJobsRate

	prioTubeCommands

	prioTubeCurrentConnections

	prioTubePauseTime
)

var (
	statsCharts = module.Charts{
		currentJobs.Copy(),
		jobsRateChart.Copy(),
		jobsTimeoutsChart.Copy(),

		currentTubesChart.Copy(),

		commandsRateChart.Copy(),

		currentConnectionsChart.Copy(),
		connectionsRateChart.Copy(),

		binlogRecordsChart.Copy(),

		cpuUsageChart.Copy(),

		uptimeChart.Copy(),
	}

	currentJobs = module.Chart{
		ID:       "current_jobs",
		Title:    "Current Jobs",
		Units:    "jobs",
		Fam:      "jobs",
		Ctx:      "beanstalk.current_jobs",
		Type:     module.Stacked,
		Priority: prioCurrentJobs,
		Dims: module.Dims{
			{ID: "current-jobs-ready", Name: "ready"},
			{ID: "current-jobs-buried", Name: "buried"},
			{ID: "current-jobs-urgent", Name: "urgent"},
			{ID: "current-jobs-delayed", Name: "delayed"},
			{ID: "current-jobs-reserved", Name: "reserved"},
		},
	}
	jobsRateChart = module.Chart{
		ID:       "jobs_rate",
		Title:    "Jobs Rate",
		Units:    "jobs/s",
		Fam:      "jobs",
		Ctx:      "beanstalk.jobs_rate",
		Type:     module.Line,
		Priority: prioJobsRate,
		Dims: module.Dims{
			{ID: "total-jobs", Name: "created", Algo: module.Incremental},
		},
	}
	jobsTimeoutsChart = module.Chart{
		ID:       "jobs_timeouts",
		Title:    "Timed Out Jobs",
		Units:    "jobs/s",
		Fam:      "jobs",
		Ctx:      "beanstalk.jobs_timeouts",
		Type:     module.Line,
		Priority: prioJobsTimeouts,
		Dims: module.Dims{
			{ID: "job-timeouts", Name: "timeouts", Algo: module.Incremental},
		},
	}

	currentTubesChart = module.Chart{
		ID:       "current_tubes",
		Title:    "Current Tubes",
		Units:    "tubes",
		Fam:      "tubes",
		Ctx:      "beanstalk.current_tubes",
		Type:     module.Line,
		Priority: prioCurrentTubes,
		Dims: module.Dims{
			{ID: "current-tubes", Name: "tubes"},
		},
	}

	commandsRateChart = module.Chart{
		ID:       "commands_rate",
		Title:    "Commands Rate",
		Units:    "commands/s",
		Fam:      "commands",
		Ctx:      "beanstalk.commands_rate",
		Type:     module.Stacked,
		Priority: prioCommandsRate,
		Dims: module.Dims{
			{ID: "cmd-put", Name: "put", Algo: module.Incremental},
			{ID: "cmd-peek", Name: "peek", Algo: module.Incremental},
			{ID: "cmd-peek-ready", Name: "peek-ready", Algo: module.Incremental},
			{ID: "cmd-peek-delayed", Name: "peek-delayed", Algo: module.Incremental},
			{ID: "cmd-peek-buried", Name: "peek-buried", Algo: module.Incremental},
			{ID: "cmd-reserve", Name: "reserve", Algo: module.Incremental},
			{ID: "cmd-reserve-with-timeout", Name: "reserve-with-timeout", Algo: module.Incremental},
			{ID: "cmd-touch", Name: "touch", Algo: module.Incremental},
			{ID: "cmd-use", Name: "use", Algo: module.Incremental},
			{ID: "cmd-watch", Name: "watch", Algo: module.Incremental},
			{ID: "cmd-ignore", Name: "ignore", Algo: module.Incremental},
			{ID: "cmd-delete", Name: "delete", Algo: module.Incremental},
			{ID: "cmd-release", Name: "release", Algo: module.Incremental},
			{ID: "cmd-bury", Name: "bury", Algo: module.Incremental},
			{ID: "cmd-kick", Name: "kick", Algo: module.Incremental},
			{ID: "cmd-stats", Name: "stats", Algo: module.Incremental},
			{ID: "cmd-stats-job", Name: "stats-job", Algo: module.Incremental},
			{ID: "cmd-stats-tube", Name: "stats-tube", Algo: module.Incremental},
			{ID: "cmd-list-tubes", Name: "list-tubes", Algo: module.Incremental},
			{ID: "cmd-list-tube-used", Name: "list-tube-used", Algo: module.Incremental},
			{ID: "cmd-list-tubes-watched", Name: "list-tubes-watched", Algo: module.Incremental},
			{ID: "cmd-pause-tube", Name: "pause-tube", Algo: module.Incremental},
		},
	}

	currentConnectionsChart = module.Chart{
		ID:       "current_connections",
		Title:    "Current Connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "beanstalk.current_connections",
		Type:     module.Line,
		Priority: prioCurrentConnections,
		Dims: module.Dims{
			{ID: "current-connections", Name: "open"},
			{ID: "current-producers", Name: "producers"},
			{ID: "current-workers", Name: "workers"},
			{ID: "current-waiting", Name: "waiting"},
		},
	}
	connectionsRateChart = module.Chart{
		ID:       "connections_rate",
		Title:    "Connections Rate",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "beanstalk.connections_rate",
		Type:     module.Line,
		Priority: prioConnectionsRate,
		Dims: module.Dims{
			{ID: "total-connections", Name: "created", Algo: module.Incremental},
		},
	}

	binlogRecordsChart = module.Chart{
		ID:       "binlog_records",
		Title:    "Binlog Records",
		Units:    "records/s",
		Fam:      "binlog",
		Ctx:      "beanstalk.binlog_records",
		Type:     module.Line,
		Priority: prioBinlogRecords,
		Dims: module.Dims{
			{ID: "binlog-records-written", Name: "written", Algo: module.Incremental},
			{ID: "binlog-records-migrated", Name: "migrated", Algo: module.Incremental},
		},
	}

	cpuUsageChart = module.Chart{
		ID:       "cpu_usage",
		Title:    "CPU Usage",
		Units:    "percent",
		Fam:      "cpu usage",
		Ctx:      "beanstalk.cpu_usage",
		Type:     module.Stacked,
		Priority: prioCpuUsage,
		Dims: module.Dims{
			{ID: "rusage-utime", Name: "user", Algo: module.Incremental, Mul: 100, Div: 1000},
			{ID: "rusage-stime", Name: "system", Algo: module.Incremental, Mul: 100, Div: 1000},
		},
	}

	uptimeChart = module.Chart{
		ID:       "uptime",
		Title:    "Uptime",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "beanstalk.uptime",
		Type:     module.Line,
		Priority: prioUptime,
		Dims: module.Dims{
			{ID: "uptime"},
		},
	}
)

var (
	tubeChartsTmpl = module.Charts{
		tubeCurrentJobsChartTmpl.Copy(),
		tubeJobsRateChartTmpl.Copy(),

		tubeCommandsRateChartTmpl.Copy(),

		tubeCurrentConnectionsChartTmpl.Copy(),

		tubePauseTimeChartTmpl.Copy(),
	}

	tubeCurrentJobsChartTmpl = module.Chart{
		ID:       "tube_%s_current_jobs",
		Title:    "Tube Current Jobs",
		Units:    "jobs",
		Fam:      "tube jobs",
		Ctx:      "beanstalk.tube_current_jobs",
		Type:     module.Stacked,
		Priority: prioTubeCurrentJobs,
		Dims: module.Dims{
			{ID: "tube_%s_current-jobs-ready", Name: "ready"},
			{ID: "tube_%s_current-jobs-buried", Name: "buried"},
			{ID: "tube_%s_current-jobs-urgent", Name: "urgent"},
			{ID: "tube_%s_current-jobs-delayed", Name: "delayed"},
			{ID: "tube_%s_current-jobs-reserved", Name: "reserved"},
		},
	}
	tubeJobsRateChartTmpl = module.Chart{
		ID:       "tube_%s_jobs_rate",
		Title:    "Tube Jobs Rate",
		Units:    "jobs/s",
		Fam:      "tube jobs",
		Ctx:      "beanstalk.tube_jobs_rate",
		Type:     module.Line,
		Priority: prioTubeJobsRate,
		Dims: module.Dims{
			{ID: "tube_%s_total-jobs", Name: "created", Algo: module.Incremental},
		},
	}
	tubeCommandsRateChartTmpl = module.Chart{
		ID:       "tube_%s_commands_rate",
		Title:    "Tube Commands",
		Units:    "commands/s",
		Fam:      "tube commands",
		Ctx:      "beanstalk.tube_commands_rate",
		Type:     module.Stacked,
		Priority: prioTubeCommands,
		Dims: module.Dims{
			{ID: "tube_%s_cmd-delete", Name: "delete", Algo: module.Incremental},
			{ID: "tube_%s_cmd-pause-tube", Name: "pause-tube", Algo: module.Incremental},
		},
	}
	tubeCurrentConnectionsChartTmpl = module.Chart{
		ID:       "tube_%s_current_connections",
		Title:    "Tube Current Connections",
		Units:    "connections",
		Fam:      "tube connections",
		Ctx:      "beanstalk.tube_current_connections",
		Type:     module.Stacked,
		Priority: prioTubeCurrentConnections,
		Dims: module.Dims{
			{ID: "tube_%s_current-using", Name: "using"},
			{ID: "tube_%s_current-waiting", Name: "waiting"},
			{ID: "tube_%s_current-watching", Name: "watching"},
		},
	}
	tubePauseTimeChartTmpl = module.Chart{
		ID:       "tube_%s_pause_time",
		Title:    "Tube Pause Time",
		Units:    "seconds",
		Fam:      "tube pause",
		Ctx:      "beanstalk.tube_pause",
		Type:     module.Line,
		Priority: prioTubePauseTime,
		Dims: module.Dims{
			{ID: "tube_%s_pause", Name: "since"},
			{ID: "tube_%s_pause-time-left", Name: "left"},
		},
	}
)

func (c *Collector) addTubeCharts(name string) {
	charts := tubeChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanTubeName(name))
		chart.Labels = []module.Label{
			{Key: "tube_name", Value: name},
		}

		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, name)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeTubeCharts(name string) {
	px := fmt.Sprintf("tube_%s_", cleanTubeName(name))

	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func cleanTubeName(name string) string {
	r := strings.NewReplacer(" ", "_", ".", "_", ",", "_")
	return r.Replace(name)
}
