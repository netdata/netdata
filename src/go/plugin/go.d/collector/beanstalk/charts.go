// SPDX-License-Identifier: GPL-3.0-or-later

package beanstalk

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioCurrentJobs = collectorapi.Priority + iota
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
	statsCharts = collectorapi.Charts{
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

	currentJobs = collectorapi.Chart{
		ID:       "current_jobs",
		Title:    "Current Jobs",
		Units:    "jobs",
		Fam:      "jobs",
		Ctx:      "beanstalk.current_jobs",
		Type:     collectorapi.Stacked,
		Priority: prioCurrentJobs,
		Dims: collectorapi.Dims{
			{ID: "current-jobs-ready", Name: "ready"},
			{ID: "current-jobs-buried", Name: "buried"},
			{ID: "current-jobs-urgent", Name: "urgent"},
			{ID: "current-jobs-delayed", Name: "delayed"},
			{ID: "current-jobs-reserved", Name: "reserved"},
		},
	}
	jobsRateChart = collectorapi.Chart{
		ID:       "jobs_rate",
		Title:    "Jobs Rate",
		Units:    "jobs/s",
		Fam:      "jobs",
		Ctx:      "beanstalk.jobs_rate",
		Type:     collectorapi.Line,
		Priority: prioJobsRate,
		Dims: collectorapi.Dims{
			{ID: "total-jobs", Name: "created", Algo: collectorapi.Incremental},
		},
	}
	jobsTimeoutsChart = collectorapi.Chart{
		ID:       "jobs_timeouts",
		Title:    "Timed Out Jobs",
		Units:    "jobs/s",
		Fam:      "jobs",
		Ctx:      "beanstalk.jobs_timeouts",
		Type:     collectorapi.Line,
		Priority: prioJobsTimeouts,
		Dims: collectorapi.Dims{
			{ID: "job-timeouts", Name: "timeouts", Algo: collectorapi.Incremental},
		},
	}

	currentTubesChart = collectorapi.Chart{
		ID:       "current_tubes",
		Title:    "Current Tubes",
		Units:    "tubes",
		Fam:      "tubes",
		Ctx:      "beanstalk.current_tubes",
		Type:     collectorapi.Line,
		Priority: prioCurrentTubes,
		Dims: collectorapi.Dims{
			{ID: "current-tubes", Name: "tubes"},
		},
	}

	commandsRateChart = collectorapi.Chart{
		ID:       "commands_rate",
		Title:    "Commands Rate",
		Units:    "commands/s",
		Fam:      "commands",
		Ctx:      "beanstalk.commands_rate",
		Type:     collectorapi.Stacked,
		Priority: prioCommandsRate,
		Dims: collectorapi.Dims{
			{ID: "cmd-put", Name: "put", Algo: collectorapi.Incremental},
			{ID: "cmd-peek", Name: "peek", Algo: collectorapi.Incremental},
			{ID: "cmd-peek-ready", Name: "peek-ready", Algo: collectorapi.Incremental},
			{ID: "cmd-peek-delayed", Name: "peek-delayed", Algo: collectorapi.Incremental},
			{ID: "cmd-peek-buried", Name: "peek-buried", Algo: collectorapi.Incremental},
			{ID: "cmd-reserve", Name: "reserve", Algo: collectorapi.Incremental},
			{ID: "cmd-reserve-with-timeout", Name: "reserve-with-timeout", Algo: collectorapi.Incremental},
			{ID: "cmd-touch", Name: "touch", Algo: collectorapi.Incremental},
			{ID: "cmd-use", Name: "use", Algo: collectorapi.Incremental},
			{ID: "cmd-watch", Name: "watch", Algo: collectorapi.Incremental},
			{ID: "cmd-ignore", Name: "ignore", Algo: collectorapi.Incremental},
			{ID: "cmd-delete", Name: "delete", Algo: collectorapi.Incremental},
			{ID: "cmd-release", Name: "release", Algo: collectorapi.Incremental},
			{ID: "cmd-bury", Name: "bury", Algo: collectorapi.Incremental},
			{ID: "cmd-kick", Name: "kick", Algo: collectorapi.Incremental},
			{ID: "cmd-stats", Name: "stats", Algo: collectorapi.Incremental},
			{ID: "cmd-stats-job", Name: "stats-job", Algo: collectorapi.Incremental},
			{ID: "cmd-stats-tube", Name: "stats-tube", Algo: collectorapi.Incremental},
			{ID: "cmd-list-tubes", Name: "list-tubes", Algo: collectorapi.Incremental},
			{ID: "cmd-list-tube-used", Name: "list-tube-used", Algo: collectorapi.Incremental},
			{ID: "cmd-list-tubes-watched", Name: "list-tubes-watched", Algo: collectorapi.Incremental},
			{ID: "cmd-pause-tube", Name: "pause-tube", Algo: collectorapi.Incremental},
		},
	}

	currentConnectionsChart = collectorapi.Chart{
		ID:       "current_connections",
		Title:    "Current Connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "beanstalk.current_connections",
		Type:     collectorapi.Line,
		Priority: prioCurrentConnections,
		Dims: collectorapi.Dims{
			{ID: "current-connections", Name: "open"},
			{ID: "current-producers", Name: "producers"},
			{ID: "current-workers", Name: "workers"},
			{ID: "current-waiting", Name: "waiting"},
		},
	}
	connectionsRateChart = collectorapi.Chart{
		ID:       "connections_rate",
		Title:    "Connections Rate",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "beanstalk.connections_rate",
		Type:     collectorapi.Line,
		Priority: prioConnectionsRate,
		Dims: collectorapi.Dims{
			{ID: "total-connections", Name: "created", Algo: collectorapi.Incremental},
		},
	}

	binlogRecordsChart = collectorapi.Chart{
		ID:       "binlog_records",
		Title:    "Binlog Records",
		Units:    "records/s",
		Fam:      "binlog",
		Ctx:      "beanstalk.binlog_records",
		Type:     collectorapi.Line,
		Priority: prioBinlogRecords,
		Dims: collectorapi.Dims{
			{ID: "binlog-records-written", Name: "written", Algo: collectorapi.Incremental},
			{ID: "binlog-records-migrated", Name: "migrated", Algo: collectorapi.Incremental},
		},
	}

	cpuUsageChart = collectorapi.Chart{
		ID:       "cpu_usage",
		Title:    "CPU Usage",
		Units:    "percent",
		Fam:      "cpu usage",
		Ctx:      "beanstalk.cpu_usage",
		Type:     collectorapi.Stacked,
		Priority: prioCpuUsage,
		Dims: collectorapi.Dims{
			{ID: "rusage-utime", Name: "user", Algo: collectorapi.Incremental, Mul: 100, Div: 1000},
			{ID: "rusage-stime", Name: "system", Algo: collectorapi.Incremental, Mul: 100, Div: 1000},
		},
	}

	uptimeChart = collectorapi.Chart{
		ID:       "uptime",
		Title:    "Uptime",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "beanstalk.uptime",
		Type:     collectorapi.Line,
		Priority: prioUptime,
		Dims: collectorapi.Dims{
			{ID: "uptime"},
		},
	}
)

var (
	tubeChartsTmpl = collectorapi.Charts{
		tubeCurrentJobsChartTmpl.Copy(),
		tubeJobsRateChartTmpl.Copy(),

		tubeCommandsRateChartTmpl.Copy(),

		tubeCurrentConnectionsChartTmpl.Copy(),

		tubePauseTimeChartTmpl.Copy(),
	}

	tubeCurrentJobsChartTmpl = collectorapi.Chart{
		ID:       "tube_%s_current_jobs",
		Title:    "Tube Current Jobs",
		Units:    "jobs",
		Fam:      "tube jobs",
		Ctx:      "beanstalk.tube_current_jobs",
		Type:     collectorapi.Stacked,
		Priority: prioTubeCurrentJobs,
		Dims: collectorapi.Dims{
			{ID: "tube_%s_current-jobs-ready", Name: "ready"},
			{ID: "tube_%s_current-jobs-buried", Name: "buried"},
			{ID: "tube_%s_current-jobs-urgent", Name: "urgent"},
			{ID: "tube_%s_current-jobs-delayed", Name: "delayed"},
			{ID: "tube_%s_current-jobs-reserved", Name: "reserved"},
		},
	}
	tubeJobsRateChartTmpl = collectorapi.Chart{
		ID:       "tube_%s_jobs_rate",
		Title:    "Tube Jobs Rate",
		Units:    "jobs/s",
		Fam:      "tube jobs",
		Ctx:      "beanstalk.tube_jobs_rate",
		Type:     collectorapi.Line,
		Priority: prioTubeJobsRate,
		Dims: collectorapi.Dims{
			{ID: "tube_%s_total-jobs", Name: "created", Algo: collectorapi.Incremental},
		},
	}
	tubeCommandsRateChartTmpl = collectorapi.Chart{
		ID:       "tube_%s_commands_rate",
		Title:    "Tube Commands",
		Units:    "commands/s",
		Fam:      "tube commands",
		Ctx:      "beanstalk.tube_commands_rate",
		Type:     collectorapi.Stacked,
		Priority: prioTubeCommands,
		Dims: collectorapi.Dims{
			{ID: "tube_%s_cmd-delete", Name: "delete", Algo: collectorapi.Incremental},
			{ID: "tube_%s_cmd-pause-tube", Name: "pause-tube", Algo: collectorapi.Incremental},
		},
	}
	tubeCurrentConnectionsChartTmpl = collectorapi.Chart{
		ID:       "tube_%s_current_connections",
		Title:    "Tube Current Connections",
		Units:    "connections",
		Fam:      "tube connections",
		Ctx:      "beanstalk.tube_current_connections",
		Type:     collectorapi.Stacked,
		Priority: prioTubeCurrentConnections,
		Dims: collectorapi.Dims{
			{ID: "tube_%s_current-using", Name: "using"},
			{ID: "tube_%s_current-waiting", Name: "waiting"},
			{ID: "tube_%s_current-watching", Name: "watching"},
		},
	}
	tubePauseTimeChartTmpl = collectorapi.Chart{
		ID:       "tube_%s_pause_time",
		Title:    "Tube Pause Time",
		Units:    "seconds",
		Fam:      "tube pause",
		Ctx:      "beanstalk.tube_pause",
		Type:     collectorapi.Line,
		Priority: prioTubePauseTime,
		Dims: collectorapi.Dims{
			{ID: "tube_%s_pause", Name: "since"},
			{ID: "tube_%s_pause-time-left", Name: "left"},
		},
	}
)

func (c *Collector) addTubeCharts(name string) {
	charts := tubeChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanTubeName(name))
		chart.Labels = []collectorapi.Label{
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
