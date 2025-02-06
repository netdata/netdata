// SPDX-License-Identifier: GPL-3.0-or-later

package zookeeper

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

const (
	prioRequests = module.Priority + iota
	prioRequestsLatency
	prioStaleRequests
	prioStaleRequestsDropped

	prioAliveConnections
	prioConnectionsDropped
	prioConnectionsRejected
	prioAuthFails
	prioGlobalSessions

	prioServerState

	prioThrottledOps

	prioPackets

	prioFileDescriptors

	prioNodesCount
	prioWatchesCount
	prioApproxDataSize

	prioUptime
)

var charts = module.Charts{
	chartRequests.Copy(),
	chartRequestsLatency.Copy(),
	chartStaleRequests.Copy(),
	chartStaleRequestsDropped.Copy(),

	chartAliveConnections.Copy(),
	chartConnectionsDropped.Copy(),
	chartConnectionsRejected.Copy(),
	chartAuthFailed.Copy(),
	chartGlobalSessions.Copy(),

	chartServerState.Copy(),
	chartThrottledOps.Copy(),

	chartPackets.Copy(),

	chartFileDescriptors.Copy(),

	chartNodesCount.Copy(),
	chartWatchesCount.Copy(),
	chartApproxDataSize.Copy(),

	chartUptime.Copy(),
}

var (
	chartRequests = module.Chart{
		ID:       "requests",
		Title:    "Outstanding Requests",
		Units:    "requests",
		Fam:      "requests",
		Ctx:      "zookeeper.requests",
		Priority: prioRequests,
		Dims: module.Dims{
			{ID: "outstanding_requests", Name: "outstanding"},
		},
	}
	chartRequestsLatency = module.Chart{
		ID:       "requests_latency",
		Title:    "Requests Latency",
		Units:    "ms",
		Fam:      "requests",
		Ctx:      "zookeeper.requests_latency",
		Priority: prioRequestsLatency,
		Dims: module.Dims{
			{ID: "min_latency", Name: "min", Div: 1000},
			{ID: "avg_latency", Name: "avg", Div: 1000},
			{ID: "max_latency", Name: "max", Div: 1000},
		},
	}
	chartStaleRequests = module.Chart{
		ID:       "stale_requests",
		Title:    "Stale Requests",
		Units:    "requests/s",
		Fam:      "requests",
		Ctx:      "zookeeper.stale_requests",
		Priority: prioStaleRequests,
		Dims: module.Dims{
			{ID: "stale_requests", Name: "stale", Algo: module.Incremental},
		},
	}
	chartStaleRequestsDropped = module.Chart{
		ID:       "stale_requests_dropped",
		Title:    "Stale Requests Dropped",
		Units:    "requests/s",
		Fam:      "requests",
		Ctx:      "zookeeper.stale_requests_dropped",
		Priority: prioStaleRequestsDropped,
		Dims: module.Dims{
			{ID: "stale_requests_dropped", Name: "dropped", Algo: module.Incremental},
		},
	}

	chartAliveConnections = module.Chart{
		ID:       "connections",
		Title:    "Alive Connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "zookeeper.connections",
		Priority: prioAliveConnections,
		Dims: module.Dims{
			{ID: "num_alive_connections", Name: "alive"},
		},
	}
	chartConnectionsDropped = module.Chart{
		ID:       "connections_dropped",
		Title:    "Dropped Connections",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "zookeeper.connections_dropped",
		Priority: prioConnectionsDropped,
		Dims: module.Dims{
			{ID: "connection_drop_count", Name: "dropped", Algo: module.Incremental},
		},
	}
	chartConnectionsRejected = module.Chart{
		ID:       "connections_rejected",
		Title:    "Rejected Connections",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "zookeeper.connections_rejected",
		Priority: prioConnectionsRejected,
		Dims: module.Dims{
			{ID: "connection_rejected", Name: "rejected", Algo: module.Incremental},
		},
	}
	chartAuthFailed = module.Chart{
		ID:       "auth_failed_count",
		Title:    "Auth Fails",
		Units:    "fails/s",
		Fam:      "connections",
		Ctx:      "zookeeper.auth_fails",
		Priority: prioAuthFails,
		Dims: module.Dims{
			{ID: "auth_failed_count", Name: "auth", Algo: module.Incremental},
		},
	}
	chartGlobalSessions = module.Chart{
		ID:       "global_sessions",
		Title:    "Global Sessions",
		Units:    "sessions",
		Fam:      "connections",
		Ctx:      "zookeeper.global_sessions",
		Priority: prioGlobalSessions,
		Dims: module.Dims{
			{ID: "global_sessions", Name: "global"},
		},
	}

	chartServerState = module.Chart{
		ID:       "server_state",
		Title:    "Server State",
		Units:    "state",
		Fam:      "server state",
		Ctx:      "zookeeper.server_state",
		Priority: prioServerState,
		Dims: module.Dims{
			{ID: "server_state_leader", Name: "leader"},
			{ID: "server_state_follower", Name: "follower"},
			{ID: "server_state_observer", Name: "observer"},
			{ID: "server_state_standalone", Name: "standalone"},
		},
	}
	chartThrottledOps = module.Chart{
		ID:       "throttled_ops",
		Title:    "Throttled Operations",
		Units:    "ops/s",
		Fam:      "server state",
		Ctx:      "zookeeper.throttled_ops",
		Priority: prioThrottledOps,
		Dims: module.Dims{
			{ID: "throttled_ops", Name: "throttled", Algo: module.Incremental},
		},
	}

	chartPackets = module.Chart{
		ID:       "packets",
		Title:    "Packets",
		Units:    "pps",
		Fam:      "net",
		Ctx:      "zookeeper.packets",
		Priority: prioPackets,
		Dims: module.Dims{
			{ID: "packets_received", Name: "received", Algo: module.Incremental},
			{ID: "packets_sent", Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}

	chartFileDescriptors = module.Chart{
		ID:       "file_descriptor",
		Title:    "Open File Descriptors",
		Units:    "file descriptors",
		Fam:      "file descriptors",
		Ctx:      "zookeeper.file_descriptor",
		Priority: prioFileDescriptors,
		Dims: module.Dims{
			{ID: "open_file_descriptor_count", Name: "open"},
		},
		Vars: module.Vars{
			{ID: "max_file_descriptor_count"},
		},
	}

	chartNodesCount = module.Chart{
		ID:       "nodes",
		Title:    "Number of Nodes",
		Units:    "nodes",
		Fam:      "data tree",
		Ctx:      "zookeeper.nodes",
		Priority: prioNodesCount,
		Dims: module.Dims{
			{ID: "znode_count", Name: "znode"},
			{ID: "ephemerals_count", Name: "ephemerals"},
		},
	}
	chartWatchesCount = module.Chart{
		ID:       "watches",
		Title:    "Number of Watches",
		Units:    "watches",
		Fam:      "data tree",
		Ctx:      "zookeeper.watches",
		Priority: prioWatchesCount,
		Dims: module.Dims{
			{ID: "watch_count", Name: "watches"},
		},
	}
	chartApproxDataSize = module.Chart{
		ID:       "approximate_data_size",
		Title:    "Approximate Data Tree Size",
		Units:    "KiB",
		Fam:      "data tree",
		Ctx:      "zookeeper.approximate_data_size",
		Priority: prioApproxDataSize,
		Dims: module.Dims{
			{ID: "approximate_data_size", Name: "size", Div: 1024},
		},
	}

	chartUptime = module.Chart{
		ID:       "uptime",
		Title:    "Uptime",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "zookeeper.uptime",
		Priority: prioUptime,
		Dims: module.Dims{
			{ID: "uptime"},
		},
	}
)
