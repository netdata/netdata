// SPDX-License-Identifier: GPL-3.0-or-later

package boinc

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioTasks = module.Priority + iota
	prioTasksState
	prioActiveTasksState
	prioActiveTasksSchedulerState
)

var charts = module.Charts{
	tasksChart.Copy(),
	tasksStateChart.Copy(),
	activeTasksStateChart.Copy(),
	activeTasksSchedulerStateChart.Copy(),
}
var (
	tasksChart = module.Chart{
		ID:       "tasks",
		Title:    "Overall Tasks",
		Units:    "tasks",
		Fam:      "tasks",
		Ctx:      "boinc.tasks",
		Priority: prioTasks,
		Dims: module.Dims{
			{ID: "total"},
			{ID: "active"},
		},
	}
	tasksStateChart = module.Chart{
		ID:       "tasks_state",
		Title:    "Tasks per State",
		Units:    "tasks",
		Fam:      "tasks",
		Ctx:      "boinc.tasks_per_state",
		Priority: prioTasksState,
		Dims: module.Dims{
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
	activeTasksStateChart = module.Chart{
		ID:       "active_tasks_state",
		Title:    "Active Tasks per State",
		Units:    "tasks",
		Fam:      "tasks",
		Ctx:      "boinc.active_tasks_per_state",
		Priority: prioActiveTasksState,
		Dims: module.Dims{
			{ID: "uninitialized"},
			{ID: "executing"},
			{ID: "abort_pending"},
			{ID: "quit_pending"},
			{ID: "suspended"},
			{ID: "copy_pending"},
		},
	}
	activeTasksSchedulerStateChart = module.Chart{
		ID:       "active_tasks_scheduler_state",
		Title:    "Active Tasks per Scheduler State",
		Units:    "tasks",
		Fam:      "tasks",
		Ctx:      "boinc.active_tasks_per_scheduler_state",
		Priority: prioActiveTasksSchedulerState,
		Dims: module.Dims{
			{ID: "uninitialized"},
			{ID: "preempted"},
			{ID: "scheduled"},
		},
	}
)
