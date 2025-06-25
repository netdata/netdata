// SPDX-License-Identifier: GPL-3.0-or-later

package db2

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var (
	baseCharts = module.Charts{
		connectionsChart.Copy(),
		lockingChart.Copy(),
		deadlocksChart.Copy(),
		sortingChart.Copy(),
		rowActivityChart.Copy(),
		bufferpoolHitRatioChart.Copy(),
		logSpaceChart.Copy(),
	}
)

var (
	connectionsChart = module.Chart{
		ID:       "connections",
		Title:    "Database Connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "db2.connections",
		Priority: module.Priority,
		Dims: module.Dims{
			{ID: "conn_total", Name: "total"},
			{ID: "conn_active", Name: "active"},
			{ID: "conn_executing", Name: "executing"},
			{ID: "conn_idle", Name: "idle"},
		},
	}

	lockingChart = module.Chart{
		ID:       "locking",
		Title:    "Database Locking",
		Units:    "events/s",
		Fam:      "locking",
		Ctx:      "db2.locking",
		Priority: module.Priority + 10,
		Dims: module.Dims{
			{ID: "lock_waits", Name: "waits", Algo: module.Incremental},
			{ID: "lock_timeouts", Name: "timeouts", Algo: module.Incremental},
			{ID: "lock_escalations", Name: "escalations", Algo: module.Incremental},
		},
	}

	deadlocksChart = module.Chart{
		ID:       "deadlocks",
		Title:    "Database Deadlocks",
		Units:    "deadlocks/s",
		Fam:      "locking",
		Ctx:      "db2.deadlocks",
		Priority: module.Priority + 11,
		Dims: module.Dims{
			{ID: "deadlocks", Name: "deadlocks", Algo: module.Incremental},
		},
	}

	sortingChart = module.Chart{
		ID:       "sorting",
		Title:    "Database Sorting",
		Units:    "sorts/s",
		Fam:      "performance",
		Ctx:      "db2.sorting",
		Priority: module.Priority + 20,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "total_sorts", Name: "sorts", Algo: module.Incremental},
			{ID: "sort_overflows", Name: "overflows", Algo: module.Incremental},
		},
	}

	rowActivityChart = module.Chart{
		ID:       "row_activity",
		Title:    "Row Activity",
		Units:    "rows/s",
		Fam:      "activity",
		Ctx:      "db2.row_activity",
		Priority: module.Priority + 30,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "rows_read", Name: "read", Algo: module.Incremental},
			{ID: "rows_modified", Name: "modified", Algo: module.Incremental, Mul: -1},
		},
	}

	bufferpoolHitRatioChart = module.Chart{
		ID:       "bufferpool_hit_ratio",
		Title:    "Buffer Pool Hit Ratio",
		Units:    "percentage",
		Fam:      "performance",
		Ctx:      "db2.bufferpool_hit_ratio",
		Priority: module.Priority + 40,
		Dims: module.Dims{
			{ID: "bufferpool_hit_ratio", Name: "hit_ratio"},
		},
	}

	logSpaceChart = module.Chart{
		ID:       "log_space",
		Title:    "Log Space Usage",
		Units:    "bytes",
		Fam:      "storage",
		Ctx:      "db2.log_space",
		Priority: module.Priority + 50,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "log_used_space", Name: "used"},
			{ID: "log_available_space", Name: "available"},
		},
	}
)
