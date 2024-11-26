// SPDX-License-Identifier: GPL-3.0-or-later

package zookeeper

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

type (
	Charts = module.Charts
	Dims   = module.Dims
	Vars   = module.Vars
)

var charts = Charts{
	{
		ID:    "requests",
		Title: "Outstanding Requests",
		Units: "requests",
		Fam:   "requests",
		Ctx:   "zookeeper.requests",
		Dims: Dims{
			{ID: "outstanding_requests", Name: "outstanding"},
		},
	},
	{
		ID:    "requests_latency",
		Title: "Requests Latency",
		Units: "ms",
		Fam:   "requests",
		Ctx:   "zookeeper.requests_latency",
		Dims: Dims{
			{ID: "min_latency", Name: "min", Div: 1000},
			{ID: "avg_latency", Name: "avg", Div: 1000},
			{ID: "max_latency", Name: "max", Div: 1000},
		},
	},
	{
		ID:    "connections",
		Title: "Alive Connections",
		Units: "connections",
		Fam:   "connections",
		Ctx:   "zookeeper.connections",
		Dims: Dims{
			{ID: "num_alive_connections", Name: "alive"},
		},
	},
	{
		ID:    "packets",
		Title: "Packets",
		Units: "pps",
		Fam:   "net",
		Ctx:   "zookeeper.packets",
		Dims: Dims{
			{ID: "packets_received", Name: "received", Algo: module.Incremental},
			{ID: "packets_sent", Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	},
	{
		ID:    "file_descriptor",
		Title: "Open File Descriptors",
		Units: "file descriptors",
		Fam:   "file descriptors",
		Ctx:   "zookeeper.file_descriptor",
		Dims: Dims{
			{ID: "open_file_descriptor_count", Name: "open"},
		},
		Vars: Vars{
			{ID: "max_file_descriptor_count"},
		},
	},
	{
		ID:    "nodes",
		Title: "Number of Nodes",
		Units: "nodes",
		Fam:   "data tree",
		Ctx:   "zookeeper.nodes",
		Dims: Dims{
			{ID: "znode_count", Name: "znode"},
			{ID: "ephemerals_count", Name: "ephemerals"},
		},
	},
	{
		ID:    "watches",
		Title: "Number of Watches",
		Units: "watches",
		Fam:   "data tree",
		Ctx:   "zookeeper.watches",
		Dims: Dims{
			{ID: "watch_count", Name: "watches"},
		},
	},
	{
		ID:    "approximate_data_size",
		Title: "Approximate Data Tree Size",
		Units: "KiB",
		Fam:   "data tree",
		Ctx:   "zookeeper.approximate_data_size",
		Dims: Dims{
			{ID: "approximate_data_size", Name: "size", Div: 1024},
		},
	},
	{
		ID:    "server_state",
		Title: "Server State",
		Units: "state",
		Fam:   "server state",
		Ctx:   "zookeeper.server_state",
		Dims: Dims{
			{ID: "server_state", Name: "state"},
		},
	},
}
