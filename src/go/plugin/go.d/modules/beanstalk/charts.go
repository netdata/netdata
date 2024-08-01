// SPDX-License-Identifier: GPL-3.0-or-later

package beanstalk

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioCpuUsage = module.Priority + iota
	prioJobsRate
	prioConnectionsRate
	prioCommandsRate
	prioCurrentTubes
	prioCurrentJobs
	prioCurrentConnections
	prioBinlog
	prioUptime

	prioJobsRateTubeTmpl
	prioJobsTubeTmpl
	prioConnectionsTubeTmpl
	prioCommandsTubeTmpl
	prioPauseTubeTmpl
)

var (
	baseCharts = module.Charts{
		cpuUsageChart.Copy(),
		jobsRateChart.Copy(),
		connectionsRateChart.Copy(),
		commandsRateChart.Copy(),
		currentTubesChart.Copy(),
		currentJobs.Copy(),
		currentConnectionsChart.Copy(),
		binlogChart.Copy(),
		uptimeChart.Copy(),
	}
	cpuUsageChart = module.Chart{
		ID:       "cpu_usage",
		Title:    "CPU Usage",
		Units:    "cpu time",
		Fam:      "server statistics",
		Ctx:      "beanstalk.cpu_usage",
		Type:     module.Area,
		Priority: prioCpuUsage,
		Dims: module.Dims{
			{ID: "rusage-utime", Name: "user", Algo: module.Incremental},
			{ID: "rusage-stime", Name: "system", Algo: module.Incremental},
		},
	}
	jobsRateChart = module.Chart{
		ID:       "jobs_rate",
		Title:    "Jobs Rate",
		Units:    "jobs/s",
		Fam:      "server statistics",
		Ctx:      "beanstalk.jobs_rate",
		Type:     module.Line,
		Priority: prioJobsRate,
		Dims: module.Dims{
			{ID: "total-jobs", Name: "total", Algo: module.Incremental},
			{ID: "job-timeouts", Name: "timeouts", Algo: module.Incremental},
		},
	}
	connectionsRateChart = module.Chart{
		ID:       "connections_rate",
		Title:    "Connections Rate",
		Units:    "connections/s",
		Fam:      "server statistics",
		Ctx:      "beanstalk.connections_rate",
		Type:     module.Area,
		Priority: prioConnectionsRate,
		Dims: module.Dims{
			{ID: "total-connections", Name: "connections", Algo: module.Incremental},
		},
	}
	commandsRateChart = module.Chart{
		ID:       "commands_rate",
		Title:    "Commands Rate",
		Units:    "commands/s",
		Fam:      "server statistics",
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
	currentTubesChart = module.Chart{
		ID:       "current_tubes",
		Title:    "Current Tubes",
		Units:    "tubes",
		Fam:      "server statistics",
		Ctx:      "beanstalk.current_tubes",
		Type:     module.Area,
		Priority: prioCurrentTubes,
		Dims: module.Dims{
			{ID: "current-tubes", Name: "tubes"},
		},
	}
	currentJobs = module.Chart{
		ID:       "current_jobs",
		Title:    "Current Jobs",
		Units:    "jobs",
		Fam:      "server statistics",
		Ctx:      "beanstalk.current_jobs",
		Type:     module.Stacked,
		Priority: prioCurrentJobs,
		Dims: module.Dims{
			{ID: "current-jobs-urgent", Name: "urgent"},
			{ID: "current-jobs-ready", Name: "ready"},
			{ID: "current-jobs-reserved", Name: "reserved"},
			{ID: "current-jobs-delayed", Name: "delayed"},
			{ID: "current-jobs-buried", Name: "buried"},
		},
	}
	currentConnectionsChart = module.Chart{
		ID:       "current_connections",
		Title:    "Current Connections",
		Units:    "connections",
		Fam:      "server statistics",
		Ctx:      "beanstalk.current_connections",
		Type:     module.Line,
		Priority: prioCurrentConnections,
		Dims: module.Dims{
			{ID: "current-connections", Name: "written"},
			{ID: "current-producers", Name: "producers"},
			{ID: "current-workers", Name: "workers"},
			{ID: "current-waiting", Name: "waiting"},
		},
	}
	binlogChart = module.Chart{
		ID:       "binlog",
		Title:    "Binlog",
		Units:    "records/s",
		Fam:      "server statistics",
		Ctx:      "beanstalk.binlog",
		Type:     module.Line,
		Priority: prioBinlog,
		Dims: module.Dims{
			{ID: "binlog-records-written", Name: "written", Algo: module.Incremental},
			{ID: "binlog-records-migrated", Name: "migrated", Algo: module.Incremental},
		},
	}
	uptimeChart = module.Chart{
		ID:       "uptime",
		Title:    "Uptime",
		Units:    "seconds",
		Fam:      "server statistics",
		Ctx:      "beanstalk.uptime",
		Type:     module.Line,
		Priority: prioUptime,
		Dims: module.Dims{
			{ID: "uptime"},
		},
	}
)

// templates
var (
	chartsTmplTube = module.Charts{
		tubeJobsRateChartTmpl.Copy(),
		tubeJobsChartTmpl.Copy(),
		tubeConnectionsChartTmpl.Copy(),
		tubeCommandsChartTmpl.Copy(),
		tubePauseChartTmpl.Copy(),
	}

	tubeJobsRateChartTmpl = module.Chart{
		ID:       "%s_jobs_rate",
		Title:    "Job Rate",
		Units:    "jobs/s",
		Fam:      "tube %s",
		Ctx:      "beanstalk.jobs_rate",
		Type:     module.Area,
		Priority: prioJobsRateTubeTmpl,
		Dims: module.Dims{
			{ID: "%s_total-jobs", Name: "jobs", Algo: module.Incremental},
		},
	}
	tubeJobsChartTmpl = module.Chart{
		ID:       "%s_jobs",
		Title:    "Jobs",
		Units:    "jobs",
		Fam:      "tube %s",
		Ctx:      "beanstalk.jobs_rate",
		Type:     module.Stacked,
		Priority: prioJobsTubeTmpl,
		Dims: module.Dims{
			{ID: "%s_current-jobs-urgent", Name: "urgent"},
			{ID: "%s_current-jobs-ready", Name: "ready"},
			{ID: "%s_current-jobs-reserved", Name: "reserved"},
			{ID: "%s_current-jobs-delayed", Name: "delayed"},
			{ID: "%s_current-jobs-buried", Name: "buried"},
		},
	}
	tubeConnectionsChartTmpl = module.Chart{
		ID:       "%s_connections",
		Title:    "Connections",
		Units:    "connections",
		Fam:      "tube %s",
		Ctx:      "beanstalk.connections",
		Type:     module.Stacked,
		Priority: prioConnectionsTubeTmpl,
		Dims: module.Dims{
			{ID: "%s_current-using", Name: "using"},
			{ID: "%s_current-waiting", Name: "waiting"},
			{ID: "%s_current-watching", Name: "watching"},
		},
	}
	tubeCommandsChartTmpl = module.Chart{
		ID:       "%s_commands",
		Title:    "Commands",
		Units:    "commands/s",
		Fam:      "tube %s",
		Ctx:      "beanstalk.commands",
		Type:     module.Stacked,
		Priority: prioCommandsTubeTmpl,
		Dims: module.Dims{
			{ID: "%s_cmd-delete", Name: "deletes", Algo: module.Incremental},
			{ID: "%s_cmd-pause-tube", Name: "pauses", Algo: module.Incremental},
		},
	}
	tubePauseChartTmpl = module.Chart{
		ID:       "%s_pause",
		Title:    "Pause",
		Units:    "seconds",
		Fam:      "tube %s",
		Ctx:      "beanstalk.pause",
		Type:     module.Stacked,
		Priority: prioPauseTubeTmpl,
		Dims: module.Dims{
			{ID: "%s_pause", Name: "since"},
			{ID: "%s_pause-time-left", Name: "left"},
		},
	}
)

func (b *Beanstalk) addBaseCharts() {
	charts := baseCharts.Copy()

	if err := b.Charts().Add(*charts...); err != nil {
		b.Warning(err)
	}
}

func (b *Beanstalk) addTubeCharts(name string) {
	charts := chartsTmplTube.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, name)
		chart.Labels = []module.Label{
			{Key: "tube", Value: name},
		}
		chart.Fam = fmt.Sprintf(chart.Fam, name)

		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, name)
		}
	}

	if err := b.Charts().Add(*charts...); err != nil {
		b.Warning(err)
	}
}

func (b *Beanstalk) removeTubeCharts(name string) {
	px := fmt.Sprintf("%s_", name)

	for _, chart := range *b.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
