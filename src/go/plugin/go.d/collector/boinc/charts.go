// SPDX-License-Identifier: GPL-3.0-or-later

package boinc

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioTasks = collectorapi.Priority + iota
	prioTasksState
	prioActiveTasksState
	prioActiveTasksSchedulerState
)

var charts = collectorapi.Charts{
	tasksChart.Copy(),
	tasksStateChart.Copy(),
	activeTasksStateChart.Copy(),
	activeTasksSchedulerStateChart.Copy(),
}
var (
	tasksChart = collectorapi.Chart{
		ID:       "tasks",
		Title:    "Overall Tasks",
		Units:    "tasks",
		Fam:      "tasks",
		Ctx:      "boinc.tasks",
		Priority: prioTasks,
		Dims: collectorapi.Dims{
			{ID: "total"},
			{ID: "active"},
		},
	}
	tasksStateChart = collectorapi.Chart{
		ID:       "tasks_state",
		Title:    "Tasks per State",
		Units:    "tasks",
		Fam:      "tasks",
		Ctx:      "boinc.tasks_per_state",
		Priority: prioTasksState,
		Dims: collectorapi.Dims{
			{ID: "new"},
			{ID: "files_downloading", Name: "downloading"},
			{ID: "files_downloaded", Name: "downloaded"},
			{ID: "compute_error"},
			{ID: "files_uploading", Name: "uploading"},
			{ID: "files_uploaded", Name: "uploaded"},
			{ID: "aborted"},
			{ID: "upload_failed"},
		},
	}
	activeTasksStateChart = collectorapi.Chart{
		ID:       "active_tasks_state",
		Title:    "Active Tasks per State",
		Units:    "tasks",
		Fam:      "tasks",
		Ctx:      "boinc.active_tasks_per_state",
		Priority: prioActiveTasksState,
		Dims: collectorapi.Dims{
			{ID: "uninitialized"},
			{ID: "executing"},
			{ID: "abort_pending"},
			{ID: "quit_pending"},
			{ID: "suspended"},
			{ID: "copy_pending"},
		},
	}
	activeTasksSchedulerStateChart = collectorapi.Chart{
		ID:       "active_tasks_scheduler_state",
		Title:    "Active Tasks per Scheduler State",
		Units:    "tasks",
		Fam:      "tasks",
		Ctx:      "boinc.active_tasks_per_scheduler_state",
		Priority: prioActiveTasksSchedulerState,
		Dims: collectorapi.Dims{
			{ID: "uninitialized"},
			{ID: "preempted"},
			{ID: "scheduled"},
		},
	}
)
