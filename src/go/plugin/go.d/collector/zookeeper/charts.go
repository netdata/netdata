// SPDX-License-Identifier: GPL-3.0-or-later

package zookeeper

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

const (
	prioRequests = collectorapi.Priority + iota
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

var charts = collectorapi.Charts{
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
	chartRequests = collectorapi.Chart{
		ID:       "requests",
		Title:    "Outstanding Requests",
		Units:    "requests",
		Fam:      "requests",
		Ctx:      "zookeeper.requests",
		Priority: prioRequests,
		Dims: collectorapi.Dims{
			{ID: "outstanding_requests", Name: "outstanding"},
		},
	}
	chartRequestsLatency = collectorapi.Chart{
		ID:       "requests_latency",
		Title:    "Requests Latency",
		Units:    "ms",
		Fam:      "requests",
		Ctx:      "zookeeper.requests_latency",
		Priority: prioRequestsLatency,
		Dims: collectorapi.Dims{
			{ID: "min_latency", Name: "min", Div: 1000},
			{ID: "avg_latency", Name: "avg", Div: 1000},
			{ID: "max_latency", Name: "max", Div: 1000},
		},
	}
	chartStaleRequests = collectorapi.Chart{
		ID:       "stale_requests",
		Title:    "Stale Requests",
		Units:    "requests/s",
		Fam:      "requests",
		Ctx:      "zookeeper.stale_requests",
		Priority: prioStaleRequests,
		Dims: collectorapi.Dims{
			{ID: "stale_requests", Name: "stale", Algo: collectorapi.Incremental},
		},
	}
	chartStaleRequestsDropped = collectorapi.Chart{
		ID:       "stale_requests_dropped",
		Title:    "Stale Requests Dropped",
		Units:    "requests/s",
		Fam:      "requests",
		Ctx:      "zookeeper.stale_requests_dropped",
		Priority: prioStaleRequestsDropped,
		Dims: collectorapi.Dims{
			{ID: "stale_requests_dropped", Name: "dropped", Algo: collectorapi.Incremental},
		},
	}

	chartAliveConnections = collectorapi.Chart{
		ID:       "connections",
		Title:    "Alive Connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "zookeeper.connections",
		Priority: prioAliveConnections,
		Dims: collectorapi.Dims{
			{ID: "num_alive_connections", Name: "alive"},
		},
	}
	chartConnectionsDropped = collectorapi.Chart{
		ID:       "connections_dropped",
		Title:    "Dropped Connections",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "zookeeper.connections_dropped",
		Priority: prioConnectionsDropped,
		Dims: collectorapi.Dims{
			{ID: "connection_drop_count", Name: "dropped", Algo: collectorapi.Incremental},
		},
	}
	chartConnectionsRejected = collectorapi.Chart{
		ID:       "connections_rejected",
		Title:    "Rejected Connections",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "zookeeper.connections_rejected",
		Priority: prioConnectionsRejected,
		Dims: collectorapi.Dims{
			{ID: "connection_rejected", Name: "rejected", Algo: collectorapi.Incremental},
		},
	}
	chartAuthFailed = collectorapi.Chart{
		ID:       "auth_failed_count",
		Title:    "Auth Fails",
		Units:    "fails/s",
		Fam:      "connections",
		Ctx:      "zookeeper.auth_fails",
		Priority: prioAuthFails,
		Dims: collectorapi.Dims{
			{ID: "auth_failed_count", Name: "auth", Algo: collectorapi.Incremental},
		},
	}
	chartGlobalSessions = collectorapi.Chart{
		ID:       "global_sessions",
		Title:    "Global Sessions",
		Units:    "sessions",
		Fam:      "connections",
		Ctx:      "zookeeper.global_sessions",
		Priority: prioGlobalSessions,
		Dims: collectorapi.Dims{
			{ID: "global_sessions", Name: "global"},
		},
	}

	chartServerState = collectorapi.Chart{
		ID:       "server_state",
		Title:    "Server State",
		Units:    "state",
		Fam:      "server state",
		Ctx:      "zookeeper.server_state",
		Priority: prioServerState,
		Dims: collectorapi.Dims{
			{ID: "server_state_leader", Name: "leader"},
			{ID: "server_state_follower", Name: "follower"},
			{ID: "server_state_observer", Name: "observer"},
			{ID: "server_state_standalone", Name: "standalone"},
		},
	}
	chartThrottledOps = collectorapi.Chart{
		ID:       "throttled_ops",
		Title:    "Throttled Operations",
		Units:    "ops/s",
		Fam:      "server state",
		Ctx:      "zookeeper.throttled_ops",
		Priority: prioThrottledOps,
		Dims: collectorapi.Dims{
			{ID: "throttled_ops", Name: "throttled", Algo: collectorapi.Incremental},
		},
	}

	chartPackets = collectorapi.Chart{
		ID:       "packets",
		Title:    "Packets",
		Units:    "pps",
		Fam:      "net",
		Ctx:      "zookeeper.packets",
		Priority: prioPackets,
		Dims: collectorapi.Dims{
			{ID: "packets_received", Name: "received", Algo: collectorapi.Incremental},
			{ID: "packets_sent", Name: "sent", Algo: collectorapi.Incremental, Mul: -1},
		},
	}

	chartFileDescriptors = collectorapi.Chart{
		ID:       "file_descriptor",
		Title:    "Open File Descriptors",
		Units:    "file descriptors",
		Fam:      "file descriptors",
		Ctx:      "zookeeper.file_descriptor",
		Priority: prioFileDescriptors,
		Dims: collectorapi.Dims{
			{ID: "open_file_descriptor_count", Name: "open"},
		},
		Vars: collectorapi.Vars{
			{ID: "max_file_descriptor_count"},
		},
	}

	chartNodesCount = collectorapi.Chart{
		ID:       "nodes",
		Title:    "Number of Nodes",
		Units:    "nodes",
		Fam:      "data tree",
		Ctx:      "zookeeper.nodes",
		Priority: prioNodesCount,
		Dims: collectorapi.Dims{
			{ID: "znode_count", Name: "znode"},
			{ID: "ephemerals_count", Name: "ephemerals"},
		},
	}
	chartWatchesCount = collectorapi.Chart{
		ID:       "watches",
		Title:    "Number of Watches",
		Units:    "watches",
		Fam:      "data tree",
		Ctx:      "zookeeper.watches",
		Priority: prioWatchesCount,
		Dims: collectorapi.Dims{
			{ID: "watch_count", Name: "watches"},
		},
	}
	chartApproxDataSize = collectorapi.Chart{
		ID:       "approximate_data_size",
		Title:    "Approximate Data Tree Size",
		Units:    "KiB",
		Fam:      "data tree",
		Ctx:      "zookeeper.approximate_data_size",
		Priority: prioApproxDataSize,
		Dims: collectorapi.Dims{
			{ID: "approximate_data_size", Name: "size", Div: 1024},
		},
	}

	chartUptime = collectorapi.Chart{
		ID:       "uptime",
		Title:    "Uptime",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "zookeeper.uptime",
		Priority: prioUptime,
		Dims: collectorapi.Dims{
			{ID: "uptime"},
		},
	}
)
