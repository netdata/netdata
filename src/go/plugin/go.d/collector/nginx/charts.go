// SPDX-License-Identifier: GPL-3.0-or-later

package nginx

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

type (
	// Charts is an alias for collectorapi.Charts
	Charts = collectorapi.Charts
	// Dims is an alias for collectorapi.Dims
	Dims = collectorapi.Dims
)

var charts = Charts{
	{
		ID:    "connections",
		Title: "Active Client Connections Including Waiting Connections",
		Units: "connections",
		Fam:   "connections",
		Ctx:   "nginx.connections",
		Dims: Dims{
			{ID: "active"},
		},
	},
	{
		ID:    "connections_statuses",
		Title: "Active Connections Per Status",
		Units: "connections",
		Fam:   "connections",
		Ctx:   "nginx.connections_status",
		Dims: Dims{
			{ID: "reading"},
			{ID: "writing"},
			{ID: "waiting", Name: "idle"},
		},
	},
	{
		ID:    "connections_accepted_handled",
		Title: "Accepted And Handled Connections",
		Units: "connections/s",
		Fam:   "connections",
		Ctx:   "nginx.connections_accepted_handled",
		Dims: Dims{
			{ID: "accepts", Name: "accepted", Algo: collectorapi.Incremental},
			{ID: "handled", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "requests",
		Title: "Client Requests",
		Units: "requests/s",
		Fam:   "requests",
		Ctx:   "nginx.requests",
		Dims: Dims{
			{ID: "requests", Algo: collectorapi.Incremental},
		},
	},
}
