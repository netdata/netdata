// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package logind

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

const (
	prioSessions = module.Priority + iota
	prioSessionsType
	prioSessionsState
	prioUsersState
)

var charts = module.Charts{
	sessionsChart.Copy(),
	sessionsTypeChart.Copy(),
	sessionsStateChart.Copy(),
	usersStateChart.Copy(),
}

var sessionsChart = module.Chart{
	ID:       "sessions",
	Title:    "Logind Sessions",
	Units:    "sessions",
	Fam:      "sessions",
	Ctx:      "logind.sessions",
	Priority: prioSessions,
	Type:     module.Stacked,
	Dims: module.Dims{
		{ID: "sessions_remote", Name: "remote"},
		{ID: "sessions_local", Name: "local"},
	},
}

var sessionsTypeChart = module.Chart{
	ID:       "sessions_type",
	Title:    "Logind Sessions By Type",
	Units:    "sessions",
	Fam:      "sessions",
	Ctx:      "logind.sessions_type",
	Priority: prioSessionsType,
	Type:     module.Stacked,
	Dims: module.Dims{
		{ID: "sessions_type_console", Name: "console"},
		{ID: "sessions_type_graphical", Name: "graphical"},
		{ID: "sessions_type_other", Name: "other"},
	},
}

var sessionsStateChart = module.Chart{
	ID:       "sessions_state",
	Title:    "Logind Sessions By State",
	Units:    "sessions",
	Fam:      "sessions",
	Ctx:      "logind.sessions_state",
	Priority: prioSessionsState,
	Type:     module.Stacked,
	Dims: module.Dims{
		{ID: "sessions_state_online", Name: "online"},
		{ID: "sessions_state_closing", Name: "closing"},
		{ID: "sessions_state_active", Name: "active"},
	},
}

var usersStateChart = module.Chart{
	ID:       "users_state",
	Title:    "Logind Users By State",
	Units:    "users",
	Fam:      "users",
	Ctx:      "logind.users_state",
	Priority: prioUsersState,
	Type:     module.Stacked,
	Dims: module.Dims{
		{ID: "users_state_offline", Name: "offline"},
		{ID: "users_state_closing", Name: "closing"},
		{ID: "users_state_online", Name: "online"},
		{ID: "users_state_lingering", Name: "lingering"},
		{ID: "users_state_active", Name: "active"},
	},
}
