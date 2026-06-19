// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package logind

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

const (
	prioSessions = collectorapi.Priority + iota
	prioSessionsType
	prioSessionsState
	prioUsersState
)

var charts = collectorapi.Charts{
	sessionsChart.Copy(),
	sessionsTypeChart.Copy(),
	sessionsStateChart.Copy(),
	usersStateChart.Copy(),
}

var sessionsChart = collectorapi.Chart{
	ID:       "sessions",
	Title:    "Logind Sessions",
	Units:    "sessions",
	Fam:      "sessions",
	Ctx:      "logind.sessions",
	Priority: prioSessions,
	Type:     collectorapi.Stacked,
	Dims: collectorapi.Dims{
		{ID: "sessions_remote", Name: "remote"},
		{ID: "sessions_local", Name: "local"},
	},
}

var sessionsTypeChart = collectorapi.Chart{
	ID:       "sessions_type",
	Title:    "Logind Sessions By Type",
	Units:    "sessions",
	Fam:      "sessions",
	Ctx:      "logind.sessions_type",
	Priority: prioSessionsType,
	Type:     collectorapi.Stacked,
	Dims: collectorapi.Dims{
		{ID: "sessions_type_console", Name: "console"},
		{ID: "sessions_type_graphical", Name: "graphical"},
		{ID: "sessions_type_other", Name: "other"},
	},
}

var sessionsStateChart = collectorapi.Chart{
	ID:       "sessions_state",
	Title:    "Logind Sessions By State",
	Units:    "sessions",
	Fam:      "sessions",
	Ctx:      "logind.sessions_state",
	Priority: prioSessionsState,
	Type:     collectorapi.Stacked,
	Dims: collectorapi.Dims{
		{ID: "sessions_state_online", Name: "online"},
		{ID: "sessions_state_closing", Name: "closing"},
		{ID: "sessions_state_active", Name: "active"},
	},
}

var usersStateChart = collectorapi.Chart{
	ID:       "users_state",
	Title:    "Logind Users By State",
	Units:    "users",
	Fam:      "users",
	Ctx:      "logind.users_state",
	Priority: prioUsersState,
	Type:     collectorapi.Stacked,
	Dims: collectorapi.Dims{
		{ID: "users_state_offline", Name: "offline"},
		{ID: "users_state_closing", Name: "closing"},
		{ID: "users_state_online", Name: "online"},
		{ID: "users_state_lingering", Name: "lingering"},
		{ID: "users_state_active", Name: "active"},
	},
}
