// SPDX-License-Identifier: GPL-3.0-or-later

package nginx

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

type (
	// Charts is an alias for module.Charts
	Charts = module.Charts
	// Dims is an alias for module.Dims
	Dims = module.Dims
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
			{ID: "accepts", Name: "accepted", Algo: module.Incremental},
			{ID: "handled", Algo: module.Incremental},
		},
	},
	{
		ID:    "requests",
		Title: "Client Requests",
		Units: "requests/s",
		Fam:   "requests",
		Ctx:   "nginx.requests",
		Dims: Dims{
			{ID: "requests", Algo: module.Incremental},
		},
	},
}
