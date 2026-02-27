// SPDX-License-Identifier: GPL-3.0-or-later

package lighttpd

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

type (
	// Charts is an alias for collectorapi.Charts
	Charts = collectorapi.Charts
	// Dims is an alias for collectorapi.Dims
	Dims = collectorapi.Dims
)

var charts = Charts{
	{
		ID:    "requests",
		Title: "Requests",
		Units: "requests/s",
		Fam:   "requests",
		Ctx:   "lighttpd.requests",
		Dims: Dims{
			{ID: "total_accesses", Name: "requests", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "net",
		Title: "Bandwidth",
		Units: "kilobits/s",
		Fam:   "bandwidth",
		Ctx:   "lighttpd.net",
		Type:  collectorapi.Area,
		Dims: Dims{
			{ID: "total_kBytes", Name: "sent", Algo: collectorapi.Incremental, Mul: 8},
		},
	},
	{
		ID:    "servers",
		Title: "Servers",
		Units: "servers",
		Fam:   "servers",
		Ctx:   "lighttpd.workers",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: "idle_servers", Name: "idle"},
			{ID: "busy_servers", Name: "busy"},
		},
	},
	{
		ID:    "scoreboard",
		Title: "ScoreBoard",
		Units: "connections",
		Fam:   "connections",
		Ctx:   "lighttpd.scoreboard",
		Dims: Dims{
			{ID: "scoreboard_waiting", Name: "waiting"},
			{ID: "scoreboard_open", Name: "open"},
			{ID: "scoreboard_close", Name: "close"},
			{ID: "scoreboard_hard_error", Name: "hard error"},
			{ID: "scoreboard_keepalive", Name: "keepalive"},
			{ID: "scoreboard_read", Name: "read"},
			{ID: "scoreboard_read_post", Name: "read post"},
			{ID: "scoreboard_write", Name: "write"},
			{ID: "scoreboard_handle_request", Name: "handle request"},
			{ID: "scoreboard_request_start", Name: "request start"},
			{ID: "scoreboard_request_end", Name: "request end"},
			{ID: "scoreboard_response_start", Name: "response start"},
			{ID: "scoreboard_response_end", Name: "response end"},
		},
	},
	{
		ID:    "uptime",
		Title: "Uptime",
		Units: "seconds",
		Fam:   "uptime",
		Ctx:   "lighttpd.uptime",
		Dims: Dims{
			{ID: "uptime"},
		},
	},
}
