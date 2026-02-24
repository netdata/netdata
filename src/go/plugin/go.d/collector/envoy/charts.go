// SPDX-License-Identifier: GPL-3.0-or-later

package envoy

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

	"github.com/prometheus/prometheus/model/labels"
)

const (
	prioServerState = collectorapi.Priority + iota
	prioServerMemoryAllocatedSize
	prioServerMemoryHeapSize
	prioServerMemoryPhysicalSize
	prioServerConnectionsCount
	prioServerParentConnectionsCount

	prioClusterManagerClustersCount
	prioClusterManagerClusterChangesRate
	prioClusterManagerClusterUpdatesRate
	prioClusterManagerClusterUpdatesVieMergeRate
	prioClusterManagerClusterUpdatesMergeCancelledRate
	prioClusterManagerClusterUpdatesOufOfMergeWindowsRate

	prioClusterMembershipEndpointsCount
	prioClusterMembershipChangesRate
	prioClusterMembershipUpdatesRate

	prioClusterUpstreamActiveConnectionsCount
	prioClusterUpstreamConnectionsRate
	prioClusterUpstreamHTTPConnectionsRate
	prioClusterUpstreamDestroyedConnectionsRate
	prioClusterUpstreamFailedConnectionsRate
	prioClusterUpstreamTimedOutConnectionsRate
	prioClusterUpstreamTrafficRate
	prioClusterUpstreamBufferedSize

	prioClusterUpstreamActiveRequestsCount
	prioClusterUpstreamRequestsRate
	prioClusterUpstreamFailedRequestsRate
	prioClusterUpstreamActivePendingRequestsCount
	prioClusterUpstreamPendingRequestsRate
	prioClusterUpstreamPendingFailedRequestsRate
	prioClusterUpstreamRequestRetriesRate
	prioClusterUpstreamRequestSuccessRetriesRate
	prioClusterUpstreamRequestBackoffRetriesRate

	prioListenerManagerListenerCount
	prioListenerManagerListenerChangesRate
	prioListenerManagerListenerObjectEventsRate

	prioListenerAdminDownstreamActiveConnectionsCount
	prioListenerAdminDownstreamConnectionsRate
	prioListenerAdminDownstreamDestroyedConnectionsRate
	prioListenerAdminDownstreamTimedOutConnectionsRate
	prioListenerAdminDownstreamRejectedConnectionsRate
	prioListenerAdminDownstreamFilterClosedByRemoteConnectionsRate
	prioListenerAdminDownstreamFilterReadErrorsRate
	prioListenerAdminDownstreamActiveSocketsCount
	prioListenerAdminDownstreamTimedOutSocketsRate

	prioListenerDownstreamActiveConnectionsCount
	prioListenerDownstreamConnectionsRate
	prioListenerDownstreamDestroyedConnectionsRate
	prioListenerDownstreamTimedOutConnectionsRate
	prioListenerDownstreamRejectedConnectionsRate
	prioListenerDownstreamFilterClosedByRemoteConnectionsRate
	prioListenerDownstreamFilterReadErrorsRate
	prioListenerDownstreamActiveSocketsCount
	prioListenerDownstreamTimedOutSocketsRate

	prioServerUptime
)

var (
	serverChartsTmpl = collectorapi.Charts{
		serverStateChartTmpl.Copy(),

		serverMemoryAllocatedSizeChartTmpl.Copy(),
		serverMemoryHeapSizeChartTmpl.Copy(),
		serverMemoryPhysicalSizeChartTmpl.Copy(),

		serverConnectionsCountChartTmpl.Copy(),
		serverParentConnectionsCountChartTmpl.Copy(),

		serverUptimeChartTmpl.Copy(),
	}
	serverStateChartTmpl = collectorapi.Chart{
		ID:       "server_state_%s",
		Title:    "Server current state",
		Units:    "state",
		Fam:      "server",
		Ctx:      "envoy.server_state",
		Priority: prioServerState,
		Dims: collectorapi.Dims{
			{ID: "envoy_server_state_live_%s", Name: "live"},
			{ID: "envoy_server_state_draining_%s", Name: "draining"},
			{ID: "envoy_server_state_pre_initializing_%s", Name: "pre_initializing"},
			{ID: "envoy_server_state_initializing_%s", Name: "initializing"},
		},
	}
	serverConnectionsCountChartTmpl = collectorapi.Chart{
		ID:       "server_connections_%s",
		Title:    "Server current connections",
		Units:    "connections",
		Fam:      "server",
		Ctx:      "envoy.server_connections_count",
		Priority: prioServerConnectionsCount,
		Dims: collectorapi.Dims{
			{ID: "envoy_server_total_connections_%s", Name: "connections"},
		},
	}
	serverParentConnectionsCountChartTmpl = collectorapi.Chart{
		ID:       "server_parent_connections_%s",
		Title:    "Server current parent connections",
		Units:    "connections",
		Fam:      "server",
		Ctx:      "envoy.server_parent_connections_count",
		Priority: prioServerParentConnectionsCount,
		Dims: collectorapi.Dims{
			{ID: "envoy_server_parent_connections_%s", Name: "connections"},
		},
	}
	serverMemoryAllocatedSizeChartTmpl = collectorapi.Chart{
		ID:       "server_memory_allocated_size_%s",
		Title:    "Server memory allocated size",
		Units:    "bytes",
		Fam:      "server",
		Ctx:      "envoy.server_memory_allocated_size",
		Priority: prioServerMemoryAllocatedSize,
		Dims: collectorapi.Dims{
			{ID: "envoy_server_memory_allocated_%s", Name: "allocated"},
		},
	}
	serverMemoryHeapSizeChartTmpl = collectorapi.Chart{
		ID:       "server_memory_heap_size_%s",
		Title:    "Server memory heap size",
		Units:    "bytes",
		Fam:      "server",
		Ctx:      "envoy.server_memory_heap_size",
		Priority: prioServerMemoryHeapSize,
		Dims: collectorapi.Dims{
			{ID: "envoy_server_memory_heap_size_%s", Name: "heap"},
		},
	}
	serverMemoryPhysicalSizeChartTmpl = collectorapi.Chart{
		ID:       "server_memory_physical_size_%s",
		Title:    "Server memory physical size",
		Units:    "bytes",
		Fam:      "server",
		Ctx:      "envoy.server_memory_physical_size",
		Priority: prioServerMemoryPhysicalSize,
		Dims: collectorapi.Dims{
			{ID: "envoy_server_memory_physical_size_%s", Name: "physical"},
		},
	}
	serverUptimeChartTmpl = collectorapi.Chart{
		ID:       "server_uptime_%s",
		Title:    "Server uptime",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "envoy.server_uptime",
		Priority: prioServerUptime,
		Dims: collectorapi.Dims{
			{ID: "envoy_server_uptime_%s", Name: "uptime"},
		},
	}
)

var (
	clusterManagerChartsTmpl = collectorapi.Charts{
		clusterManagerClusterCountChartTmpl.Copy(),
		clusterManagerClusterChangesRateChartTmpl.Copy(),
		clusterManagerClusterUpdatesRateChartTmpl.Copy(),
		clusterManagerClusterUpdatesViaMergeRateChartTmpl.Copy(),
		clusterManagerClusterUpdatesMergeCancelledRateChartTmpl.Copy(),
		clusterManagerClusterUpdatesOutOfMergeWindowRateChartTmpl.Copy(),
	}
	clusterManagerClusterCountChartTmpl = collectorapi.Chart{
		ID:       "cluster_manager_cluster_count_%s",
		Title:    "Cluster manager current clusters",
		Units:    "clusters",
		Fam:      "cluster mgr",
		Ctx:      "envoy.cluster_manager_cluster_count",
		Priority: prioClusterManagerClustersCount,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_manager_active_clusters_%s", Name: "active"},
			{ID: "envoy_cluster_manager_warming_clusters_%s", Name: "not_active"},
		},
	}
	clusterManagerClusterChangesRateChartTmpl = collectorapi.Chart{
		ID:       "cluster_manager_cluster_changes_%s",
		Title:    "Cluster manager cluster changes",
		Units:    "clusters/s",
		Fam:      "cluster mgr",
		Ctx:      "envoy.cluster_manager_cluster_changes_rate",
		Priority: prioClusterManagerClusterChangesRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_manager_cluster_added_%s", Name: "added", Algo: collectorapi.Incremental},
			{ID: "envoy_cluster_manager_cluster_modified_%s", Name: "modified", Algo: collectorapi.Incremental},
			{ID: "envoy_cluster_manager_cluster_removed_%s", Name: "removed", Algo: collectorapi.Incremental},
		},
	}
	clusterManagerClusterUpdatesRateChartTmpl = collectorapi.Chart{
		ID:       "cluster_manager_cluster_updates_%s",
		Title:    "Cluster manager updates",
		Units:    "updates/s",
		Fam:      "cluster mgr",
		Ctx:      "envoy.cluster_manager_cluster_updates_rate",
		Priority: prioClusterManagerClusterUpdatesRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_manager_cluster_updated_%s", Name: "cluster", Algo: collectorapi.Incremental},
		},
	}
	clusterManagerClusterUpdatesViaMergeRateChartTmpl = collectorapi.Chart{
		ID:       "cluster_manager_cluster_updated_via_merge_%s",
		Title:    "Cluster manager updates applied as merged updates",
		Units:    "updates/s",
		Fam:      "cluster mgr",
		Ctx:      "envoy.cluster_manager_cluster_updated_via_merge_rate",
		Priority: prioClusterManagerClusterUpdatesVieMergeRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_manager_cluster_updated_via_merge_%s", Name: "via_merge", Algo: collectorapi.Incremental},
		},
	}
	clusterManagerClusterUpdatesMergeCancelledRateChartTmpl = collectorapi.Chart{
		ID:       "cluster_manager_update_merge_cancelled_%s",
		Title:    "Cluster manager cancelled merged updates",
		Units:    "updates/s",
		Fam:      "cluster mgr",
		Ctx:      "envoy.cluster_manager_update_merge_cancelled_rate",
		Priority: prioClusterManagerClusterUpdatesMergeCancelledRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_manager_update_merge_cancelled_%s", Name: "merge_cancelled", Algo: collectorapi.Incremental},
		},
	}
	clusterManagerClusterUpdatesOutOfMergeWindowRateChartTmpl = collectorapi.Chart{
		ID:       "cluster_manager_update_out_of_merge_window_%s",
		Title:    "Cluster manager out of a merge window updates",
		Units:    "updates/s",
		Fam:      "cluster mgr",
		Ctx:      "envoy.cluster_manager_update_out_of_merge_window_rate",
		Priority: prioClusterManagerClusterUpdatesOufOfMergeWindowsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_manager_update_out_of_merge_window_%s", Name: "out_of_merge_window", Algo: collectorapi.Incremental},
		},
	}
)

var (
	clusterUpstreamChartsTmpl = collectorapi.Charts{
		clusterUpstreamActiveConnectionsCountChartTmpl.Copy(),
		clusterUpstreamConnectionsRateChartTmpl.Copy(),
		clusterUpstreamHTTPConnectionsRateChartTmpl.Copy(),
		clusterUpstreamDestroyedConnectionsRateChartTmpl.Copy(),
		clusterUpstreamFailedConnectionsRateChartTmpl.Copy(),
		clusterUpstreamTimedOutConnectionsRateChartTmpl.Copy(),
		clusterUpstreamTrafficRateChartTmpl.Copy(),
		clusterUpstreamBufferedSizeChartTmpl.Copy(),

		clusterUpstreamActiveRequestsCountChartTmpl.Copy(),
		clusterUpstreamRequestsRateChartTmpl.Copy(),
		clusterUpstreamFailedRequestsRateChartTmpl.Copy(),
		clusterUpstreamActivePendingRequestsCountChartTmpl.Copy(),
		clusterUpstreamPendingRequestsRateChartTmpl.Copy(),
		clusterUpstreamPendingFailedRequestsRateChartTmpl.Copy(),
		clusterUpstreamRequestRetriesRateChartTmpl.Copy(),
		clusterUpstreamRequestSuccessRetriesRateChartTmpl.Copy(),
		clusterUpstreamRequestRetriesBackoffRateChartTmpl.Copy(),

		clusterMembershipEndpointsCountChartTmpl.Copy(),
		clusterMembershipChangesRateChartTmpl.Copy(),
		clusterMembershipUpdatesRateChartTmpl.Copy(),
	}

	clusterUpstreamActiveConnectionsCountChartTmpl = collectorapi.Chart{
		ID:       "cluster_upstream_cx_active_%s",
		Title:    "Cluster upstream current active connections",
		Units:    "connections",
		Fam:      "upstream conns",
		Ctx:      "envoy.cluster_upstream_cx_active_count",
		Priority: prioClusterUpstreamActiveConnectionsCount,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_upstream_cx_active_%s", Name: "active"},
		},
	}
	clusterUpstreamConnectionsRateChartTmpl = collectorapi.Chart{
		ID:       "cluster_upstream_cx_total_%s",
		Title:    "Cluster upstream connections",
		Units:    "connections/s",
		Fam:      "upstream conns",
		Ctx:      "envoy.cluster_upstream_cx_rate",
		Priority: prioClusterUpstreamConnectionsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_upstream_cx_total_%s", Name: "created", Algo: collectorapi.Incremental},
		},
	}
	clusterUpstreamHTTPConnectionsRateChartTmpl = collectorapi.Chart{
		ID:       "cluster_upstream_cx_http_total_%s",
		Title:    "Cluster upstream connections by HTTP version",
		Units:    "connections/s",
		Fam:      "upstream conns",
		Ctx:      "envoy.cluster_upstream_cx_http_rate",
		Priority: prioClusterUpstreamHTTPConnectionsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_upstream_cx_http1_total_%s", Name: "http1", Algo: collectorapi.Incremental},
			{ID: "envoy_cluster_upstream_cx_http2_total_%s", Name: "http2", Algo: collectorapi.Incremental},
			{ID: "envoy_cluster_upstream_cx_http3_total_%s", Name: "http3", Algo: collectorapi.Incremental},
		},
	}
	clusterUpstreamDestroyedConnectionsRateChartTmpl = collectorapi.Chart{
		ID:       "cluster_upstream_cx_destroy_%s",
		Title:    "Cluster upstream destroyed connections",
		Units:    "connections/s",
		Fam:      "upstream conns",
		Ctx:      "envoy.cluster_upstream_cx_destroy_rate",
		Priority: prioClusterUpstreamDestroyedConnectionsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_upstream_cx_destroy_local_%s", Name: "local", Algo: collectorapi.Incremental},
			{ID: "envoy_cluster_upstream_cx_destroy_remote_%s", Name: "remote", Algo: collectorapi.Incremental},
		},
	}
	clusterUpstreamFailedConnectionsRateChartTmpl = collectorapi.Chart{
		ID:       "cluster_upstream_cx_connect_fail_%s",
		Title:    "Cluster upstream failed connections",
		Units:    "connections/s",
		Fam:      "upstream conns",
		Ctx:      "envoy.cluster_upstream_cx_connect_fail_rate",
		Priority: prioClusterUpstreamFailedConnectionsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_upstream_cx_connect_fail_%s", Name: "failed", Algo: collectorapi.Incremental},
		},
	}
	clusterUpstreamTimedOutConnectionsRateChartTmpl = collectorapi.Chart{
		ID:       "cluster_upstream_cx_connect_timeout_%s",
		Title:    "Cluster upstream timed out connections",
		Units:    "connections/s",
		Fam:      "upstream conns",
		Ctx:      "envoy.cluster_upstream_cx_connect_timeout_rate",
		Priority: prioClusterUpstreamTimedOutConnectionsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_upstream_cx_connect_timeout_%s", Name: "timeout", Algo: collectorapi.Incremental},
		},
	}
	clusterUpstreamTrafficRateChartTmpl = collectorapi.Chart{
		ID:       "cluster_upstream_cx_bytes_total_%s",
		Title:    "Cluster upstream connection traffic",
		Units:    "bytes/s",
		Fam:      "upstream traffic",
		Ctx:      "envoy.cluster_upstream_cx_bytes_rate",
		Priority: prioClusterUpstreamTrafficRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_upstream_cx_rx_bytes_total_%s", Name: "received", Algo: collectorapi.Incremental},
			{ID: "envoy_cluster_upstream_cx_tx_bytes_total_%s", Name: "sent", Algo: collectorapi.Incremental},
		},
	}
	clusterUpstreamBufferedSizeChartTmpl = collectorapi.Chart{
		ID:       "cluster_upstream_cx_bytes_buffered_%s",
		Title:    "Cluster upstream current connection buffered size",
		Units:    "bytes",
		Fam:      "upstream traffic",
		Ctx:      "envoy.cluster_upstream_cx_bytes_buffered_size",
		Priority: prioClusterUpstreamBufferedSize,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_upstream_cx_rx_bytes_buffered_%s", Name: "received"},
			{ID: "envoy_cluster_upstream_cx_tx_bytes_buffered_%s", Name: "send"},
		},
	}

	clusterUpstreamActiveRequestsCountChartTmpl = collectorapi.Chart{
		ID:       "cluster_upstream_rq_active_%s",
		Title:    "Cluster upstream current active requests",
		Units:    "requests",
		Fam:      "upstream requests",
		Ctx:      "envoy.cluster_upstream_rq_active_count",
		Priority: prioClusterUpstreamActiveRequestsCount,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_upstream_rq_active_%s", Name: "active"},
		},
	}
	clusterUpstreamRequestsRateChartTmpl = collectorapi.Chart{
		ID:       "cluster_upstream_rq_total_%s",
		Title:    "Cluster upstream requests",
		Units:    "requests/s",
		Fam:      "upstream requests",
		Ctx:      "envoy.cluster_upstream_rq_rate",
		Priority: prioClusterUpstreamRequestsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_upstream_rq_total_%s", Name: "requests", Algo: collectorapi.Incremental},
		},
	}
	clusterUpstreamFailedRequestsRateChartTmpl = collectorapi.Chart{
		ID:       "cluster_upstream_rq_failed_total_%s",
		Title:    "Cluster upstream failed requests",
		Units:    "requests/s",
		Fam:      "upstream requests",
		Ctx:      "envoy.cluster_upstream_rq_failed_rate",
		Priority: prioClusterUpstreamFailedRequestsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_upstream_rq_cancelled_%s", Name: "cancelled", Algo: collectorapi.Incremental},
			{ID: "envoy_cluster_upstream_rq_maintenance_mode_%s", Name: "maintenance_mode", Algo: collectorapi.Incremental},
			{ID: "envoy_cluster_upstream_rq_timeout_%s", Name: "timeout", Algo: collectorapi.Incremental},
			{ID: "envoy_cluster_upstream_rq_max_duration_reached_%s", Name: "max_duration_reached", Algo: collectorapi.Incremental},
			{ID: "envoy_cluster_upstream_rq_per_try_timeout_%s", Name: "per_try_timeout", Algo: collectorapi.Incremental},
			{ID: "envoy_cluster_upstream_rq_rx_reset_%s", Name: "reset_local", Algo: collectorapi.Incremental},
			{ID: "envoy_cluster_upstream_rq_tx_reset_%s", Name: "reset_remote", Algo: collectorapi.Incremental},
		},
	}
	clusterUpstreamActivePendingRequestsCountChartTmpl = collectorapi.Chart{
		ID:       "cluster_upstream_rq_pending_active_%s",
		Title:    "Cluster upstream current active pending requests",
		Units:    "requests",
		Fam:      "upstream requests",
		Ctx:      "envoy.cluster_upstream_rq_pending_active_count",
		Priority: prioClusterUpstreamActivePendingRequestsCount,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_upstream_rq_pending_active_%s", Name: "active_pending"},
		},
	}
	clusterUpstreamPendingRequestsRateChartTmpl = collectorapi.Chart{
		ID:       "cluster_upstream_rq_pending_total_%s",
		Title:    "Cluster upstream pending requests",
		Units:    "requests/s",
		Fam:      "upstream requests",
		Ctx:      "envoy.cluster_upstream_rq_pending_rate",
		Priority: prioClusterUpstreamPendingRequestsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_upstream_rq_pending_total_%s", Name: "pending", Algo: collectorapi.Incremental},
		},
	}
	clusterUpstreamPendingFailedRequestsRateChartTmpl = collectorapi.Chart{
		ID:       "cluster_upstream_rq_pending_failed_total_%s",
		Title:    "Cluster upstream failed pending requests",
		Units:    "requests/s",
		Fam:      "upstream requests",
		Ctx:      "envoy.cluster_upstream_rq_pending_failed_rate",
		Priority: prioClusterUpstreamPendingFailedRequestsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_upstream_rq_pending_overflow_%s", Name: "overflow", Algo: collectorapi.Incremental},
			{ID: "envoy_cluster_upstream_rq_pending_failure_eject_%s", Name: "failure_eject", Algo: collectorapi.Incremental},
		},
	}
	clusterUpstreamRequestRetriesRateChartTmpl = collectorapi.Chart{
		ID:       "cluster_upstream_rq_retry_%s",
		Title:    "Cluster upstream request retries",
		Units:    "retries/s",
		Fam:      "upstream requests",
		Ctx:      "envoy.cluster_upstream_rq_retry_rate",
		Priority: prioClusterUpstreamRequestRetriesRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_upstream_rq_retry_%s", Name: "request", Algo: collectorapi.Incremental},
		},
	}
	clusterUpstreamRequestSuccessRetriesRateChartTmpl = collectorapi.Chart{
		ID:       "cluster_upstream_rq_retry_success_%s",
		Title:    "Cluster upstream request successful retries",
		Units:    "retries/s",
		Fam:      "upstream requests",
		Ctx:      "envoy.cluster_upstream_rq_retry_success_rate",
		Priority: prioClusterUpstreamRequestSuccessRetriesRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_upstream_rq_retry_success_%s", Name: "success", Algo: collectorapi.Incremental},
		},
	}
	clusterUpstreamRequestRetriesBackoffRateChartTmpl = collectorapi.Chart{
		ID:       "cluster_upstream_rq_retry_backoff_%s",
		Title:    "Cluster upstream request backoff retries",
		Units:    "retries/s",
		Fam:      "upstream requests",
		Ctx:      "envoy.cluster_upstream_rq_retry_backoff_rate",
		Priority: prioClusterUpstreamRequestBackoffRetriesRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_upstream_rq_retry_backoff_exponential_%s", Name: "exponential", Algo: collectorapi.Incremental},
			{ID: "envoy_cluster_upstream_rq_retry_backoff_ratelimited_%s", Name: "ratelimited", Algo: collectorapi.Incremental},
		},
	}

	clusterMembershipEndpointsCountChartTmpl = collectorapi.Chart{
		ID:       "cluster_membership_endpoints_count_%s",
		Title:    "Cluster membership current endpoints",
		Units:    "endpoints",
		Fam:      "cluster membership",
		Ctx:      "envoy.cluster_membership_endpoints_count",
		Priority: prioClusterMembershipEndpointsCount,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_membership_healthy_%s", Name: "healthy"},
			{ID: "envoy_cluster_membership_degraded_%s", Name: "degraded"},
			{ID: "envoy_cluster_membership_excluded_%s", Name: "excluded"},
		},
	}
	clusterMembershipChangesRateChartTmpl = collectorapi.Chart{
		ID:       "cluster_membership_change_%s",
		Title:    "Cluster membership changes",
		Units:    "changes/s",
		Fam:      "cluster membership",
		Ctx:      "envoy.cluster_membership_changes_rate",
		Priority: prioClusterMembershipChangesRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_membership_change_%s", Name: "membership", Algo: collectorapi.Incremental},
		},
	}
	clusterMembershipUpdatesRateChartTmpl = collectorapi.Chart{
		ID:       "cluster_membership_updates_%s",
		Title:    "Cluster membership updates",
		Units:    "updates/s",
		Fam:      "cluster membership",
		Ctx:      "envoy.cluster_membership_updates_rate",
		Priority: prioClusterMembershipUpdatesRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_cluster_update_success_%s", Name: "success", Algo: collectorapi.Incremental},
			{ID: "envoy_cluster_update_failure_%s", Name: "failure", Algo: collectorapi.Incremental},
			{ID: "envoy_cluster_update_empty_%s", Name: "empty", Algo: collectorapi.Incremental},
			{ID: "envoy_cluster_update_no_rebuild_%s", Name: "no_rebuild", Algo: collectorapi.Incremental},
		},
	}
)

var (
	listenerManagerChartsTmpl = collectorapi.Charts{
		listenerManagerListenersByStateCountChartTmpl.Copy(),
		listenerManagerListenerChangesRateChartTmpl.Copy(),
		listenerManagerListenerObjectEventsRateChartTmpl.Copy(),
	}
	listenerManagerListenersByStateCountChartTmpl = collectorapi.Chart{
		ID:       "listener_manager_listeners_count_%s",
		Title:    "Listener manager current listeners",
		Units:    "listeners",
		Fam:      "downstream mgr",
		Ctx:      "envoy.listener_manager_listeners_count",
		Priority: prioListenerManagerListenerCount,
		Dims: collectorapi.Dims{
			{ID: "envoy_listener_manager_total_listeners_active_%s", Name: "active"},
			{ID: "envoy_listener_manager_total_listeners_warming_%s", Name: "warming"},
			{ID: "envoy_listener_manager_total_listeners_draining_%s", Name: "draining"},
		},
	}
	listenerManagerListenerChangesRateChartTmpl = collectorapi.Chart{
		ID:       "listener_manager_listener_changes_%s",
		Title:    "Listener manager listener changes",
		Units:    "listeners/s",
		Fam:      "downstream mgr",
		Ctx:      "envoy.listener_manager_listener_changes_rate",
		Priority: prioListenerManagerListenerChangesRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_listener_manager_listener_added_%s", Name: "added", Algo: collectorapi.Incremental},
			{ID: "envoy_listener_manager_listener_modified_%s", Name: "modified", Algo: collectorapi.Incremental},
			{ID: "envoy_listener_manager_listener_removed_%s", Name: "removed", Algo: collectorapi.Incremental},
			{ID: "envoy_listener_manager_listener_stopped_%s", Name: "stopped", Algo: collectorapi.Incremental},
		},
	}
	listenerManagerListenerObjectEventsRateChartTmpl = collectorapi.Chart{
		ID:       "listener_manager_listener_object_events_%s",
		Title:    "Listener manager listener object events",
		Units:    "objects/s",
		Fam:      "downstream mgr",
		Ctx:      "envoy.listener_manager_listener_object_events_rate",
		Priority: prioListenerManagerListenerObjectEventsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_listener_manager_listener_create_success_%s", Name: "create_success", Algo: collectorapi.Incremental},
			{ID: "envoy_listener_manager_listener_create_failure_%s", Name: "create_failure", Algo: collectorapi.Incremental},
			{ID: "envoy_listener_manager_listener_in_place_updated_%s", Name: "in_place_updated", Algo: collectorapi.Incremental},
		},
	}
)

var (
	listenerAdminDownstreamChartsTmpl = collectorapi.Charts{
		listenerAdminDownstreamActiveConnectionsCountChartTmpl.Copy(),
		listenerAdminDownstreamConnectionsRateChartTmpl.Copy(),
		listenerAdminDownstreamDestroyedConnectionsRateChartTmpl.Copy(),
		listenerAdminDownstreamTimedOutConnectionsRateChartTmpl.Copy(),
		listenerAdminDownstreamRejectedConnectionsRateChartTmpl.Copy(),
		listenerAdminDownstreamFilterClosedByRemoteConnectionsRateChartTmpl.Copy(),
		listenerAdminDownstreamFilterReadErrorsRateChartTmpl.Copy(),

		listenerAdminDownstreamActiveSocketsCountChartTmpl.Copy(),
		listenerAdminDownstreamTimedOutSocketsRateChartTmpl.Copy(),
	}

	listenerAdminDownstreamActiveConnectionsCountChartTmpl = collectorapi.Chart{
		ID:       "listener_admin_downstream_cx_active_%s",
		Title:    "Listener admin downstream current active connections",
		Units:    "connections",
		Fam:      "downstream adm conns",
		Ctx:      "envoy.listener_admin_downstream_cx_active_count",
		Priority: prioListenerAdminDownstreamActiveConnectionsCount,
		Dims: collectorapi.Dims{
			{ID: "envoy_listener_admin_downstream_cx_active_%s", Name: "active"},
		},
	}
	listenerAdminDownstreamConnectionsRateChartTmpl = collectorapi.Chart{
		ID:       "listener_admin_downstream_cx_total_%s",
		Title:    "Listener admin downstream connections",
		Units:    "connections/s",
		Fam:      "downstream adm conns",
		Ctx:      "envoy.listener_admin_downstream_cx_rate",
		Priority: prioListenerAdminDownstreamConnectionsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_listener_admin_downstream_cx_total_%s", Name: "created", Algo: collectorapi.Incremental},
		},
	}
	listenerAdminDownstreamDestroyedConnectionsRateChartTmpl = collectorapi.Chart{
		ID:       "listener_admin_downstream_cx_destroy_%s",
		Title:    "Listener admin downstream destroyed connections",
		Units:    "connections/s",
		Fam:      "downstream adm conns",
		Ctx:      "envoy.listener_admin_downstream_cx_destroy_rate",
		Priority: prioListenerAdminDownstreamDestroyedConnectionsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_listener_admin_downstream_cx_destroy_%s", Name: "destroyed", Algo: collectorapi.Incremental},
		},
	}
	listenerAdminDownstreamTimedOutConnectionsRateChartTmpl = collectorapi.Chart{
		ID:       "listener_admin_downstream_cx_transport_socket_connect_timeout_%s",
		Title:    "Listener admin downstream timed out connections",
		Units:    "connections/s",
		Fam:      "downstream adm conns",
		Ctx:      "envoy.listener_admin_downstream_cx_transport_socket_connect_timeout_rate",
		Priority: prioListenerAdminDownstreamTimedOutConnectionsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_listener_admin_downstream_cx_transport_socket_connect_timeout_%s", Name: "timeout", Algo: collectorapi.Incremental},
		},
	}
	listenerAdminDownstreamRejectedConnectionsRateChartTmpl = collectorapi.Chart{
		ID:       "listener_admin_downstream_cx_rejected_%s",
		Title:    "Listener admin downstream rejected connections",
		Units:    "connections/s",
		Fam:      "downstream adm conns",
		Ctx:      "envoy.listener_admin_downstream_cx_rejected_rate",
		Priority: prioListenerAdminDownstreamRejectedConnectionsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_listener_admin_downstream_cx_overflow_%s", Name: "overflow", Algo: collectorapi.Incremental},
			{ID: "envoy_listener_admin_downstream_cx_overload_reject_%s", Name: "overload", Algo: collectorapi.Incremental},
			{ID: "envoy_listener_admin_downstream_global_cx_overflow_%s", Name: "global_overflow", Algo: collectorapi.Incremental},
		},
	}
	listenerAdminDownstreamFilterClosedByRemoteConnectionsRateChartTmpl = collectorapi.Chart{
		ID:       "listener_admin_downstream_listener_filter_remote_close_%s",
		Title:    "Listener admin downstream connections closed by remote when peek data for listener filters",
		Units:    "connections/s",
		Fam:      "downstream adm conns",
		Ctx:      "envoy.listener_admin_downstream_listener_filter_remote_close_rate",
		Priority: prioListenerAdminDownstreamFilterClosedByRemoteConnectionsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_listener_admin_downstream_listener_filter_remote_close_%s", Name: "closed", Algo: collectorapi.Incremental},
		},
	}
	listenerAdminDownstreamFilterReadErrorsRateChartTmpl = collectorapi.Chart{
		ID:       "listener_admin_downstream_listener_filter_error_%s",
		Title:    "Listener admin downstream read errors when peeking data for listener filters",
		Units:    "errors/s",
		Fam:      "downstream adm conns",
		Ctx:      "envoy.listener_admin_downstream_listener_filter_error_rate",
		Priority: prioListenerAdminDownstreamFilterReadErrorsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_listener_admin_downstream_listener_filter_error_%s", Name: "read", Algo: collectorapi.Incremental},
		},
	}

	listenerAdminDownstreamActiveSocketsCountChartTmpl = collectorapi.Chart{
		ID:       "listener_admin_downstream_pre_cx_active_%s",
		Title:    "Listener admin downstream current active sockets",
		Units:    "sockets",
		Fam:      "downstream adm sockets",
		Ctx:      "envoy.listener_admin_downstream_pre_cx_active_count",
		Priority: prioListenerAdminDownstreamActiveSocketsCount,
		Dims: collectorapi.Dims{
			{ID: "envoy_listener_admin_downstream_pre_cx_active_%s", Name: "active"},
		},
	}
	listenerAdminDownstreamTimedOutSocketsRateChartTmpl = collectorapi.Chart{
		ID:       "listener_admin_downstream_pre_cx_timeout_%s",
		Title:    "Listener admin downstream timed out sockets",
		Units:    "sockets/s",
		Fam:      "downstream adm sockets",
		Ctx:      "envoy.listener_admin_downstream_pre_cx_timeout_rate",
		Priority: prioListenerAdminDownstreamTimedOutSocketsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_listener_admin_downstream_pre_cx_timeout_%s", Name: "timeout", Algo: collectorapi.Incremental},
		},
	}
)

var (
	listenerDownstreamChartsTmpl = collectorapi.Charts{
		listenerDownstreamActiveConnectionsCountChartTmpl.Copy(),
		listenerDownstreamConnectionsRateChartTmpl.Copy(),
		listenerDownstreamDestroyedConnectionsRateChartTmpl.Copy(),
		listenerDownstreamTimedOutConnectionsRateChartTmpl.Copy(),
		listenerDownstreamRejectedConnectionsRateChartTmpl.Copy(),
		listenerDownstreamFilterClosedByRemoteConnectionsRateChartTmpl.Copy(),
		listenerDownstreamFilterReadErrorsRateChartTmpl.Copy(),

		listenerDownstreamActiveSocketsCountChartTmpl.Copy(),
		listenerDownstreamTimedOutSocketsRateChartTmpl.Copy(),
	}

	listenerDownstreamActiveConnectionsCountChartTmpl = collectorapi.Chart{
		ID:       "listener_downstream_cx_active_%s",
		Title:    "Listener downstream current active connections",
		Units:    "connections",
		Fam:      "downstream conns",
		Ctx:      "envoy.listener_downstream_cx_active_count",
		Priority: prioListenerDownstreamActiveConnectionsCount,
		Dims: collectorapi.Dims{
			{ID: "envoy_listener_downstream_cx_active_%s", Name: "active"},
		},
	}
	listenerDownstreamConnectionsRateChartTmpl = collectorapi.Chart{
		ID:       "listener_downstream_cx_total_%s",
		Title:    "Listener downstream connections",
		Units:    "connections/s",
		Fam:      "downstream conns",
		Ctx:      "envoy.listener_downstream_cx_rate",
		Priority: prioListenerDownstreamConnectionsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_listener_downstream_cx_total_%s", Name: "created", Algo: collectorapi.Incremental},
		},
	}
	listenerDownstreamDestroyedConnectionsRateChartTmpl = collectorapi.Chart{
		ID:       "listener_downstream_cx_destroy_%s",
		Title:    "Listener downstream destroyed connections",
		Units:    "connections/s",
		Fam:      "downstream conns",
		Ctx:      "envoy.listener_downstream_cx_destroy_rate",
		Priority: prioListenerDownstreamDestroyedConnectionsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_listener_downstream_cx_destroy_%s", Name: "destroyed", Algo: collectorapi.Incremental},
		},
	}
	listenerDownstreamTimedOutConnectionsRateChartTmpl = collectorapi.Chart{
		ID:       "listener_downstream_cx_transport_socket_connect_timeout_%s",
		Title:    "Listener downstream timed out connections",
		Units:    "connections/s",
		Fam:      "downstream conns",
		Ctx:      "envoy.listener_downstream_cx_transport_socket_connect_timeout_rate",
		Priority: prioListenerDownstreamTimedOutConnectionsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_listener_downstream_cx_transport_socket_connect_timeout_%s", Name: "timeout", Algo: collectorapi.Incremental},
		},
	}
	listenerDownstreamRejectedConnectionsRateChartTmpl = collectorapi.Chart{
		ID:       "listener_downstream_cx_rejected_%s",
		Title:    "Listener downstream rejected connections",
		Units:    "connections/s",
		Fam:      "downstream conns",
		Ctx:      "envoy.listener_downstream_cx_rejected_rate",
		Priority: prioListenerDownstreamRejectedConnectionsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_listener_downstream_cx_overflow_%s", Name: "overflow", Algo: collectorapi.Incremental},
			{ID: "envoy_listener_downstream_cx_overload_reject_%s", Name: "overload", Algo: collectorapi.Incremental},
			{ID: "envoy_listener_downstream_global_cx_overflow_%s", Name: "global_overflow", Algo: collectorapi.Incremental},
		},
	}
	listenerDownstreamFilterClosedByRemoteConnectionsRateChartTmpl = collectorapi.Chart{
		ID:       "listener_downstream_listener_filter_remote_close_%s",
		Title:    "Listener downstream connections closed by remote when peek data for listener filters",
		Units:    "connections/s",
		Fam:      "downstream conns",
		Ctx:      "envoy.listener_downstream_listener_filter_remote_close_rate",
		Priority: prioListenerDownstreamFilterClosedByRemoteConnectionsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_listener_downstream_listener_filter_remote_close_%s", Name: "closed", Algo: collectorapi.Incremental},
		},
	}
	listenerDownstreamFilterReadErrorsRateChartTmpl = collectorapi.Chart{
		ID:       "listener_downstream_listener_filter_error_%s",
		Title:    "Listener downstream read errors when peeking data for listener filters",
		Units:    "errors/s",
		Fam:      "downstream conns",
		Ctx:      "envoy.listener_downstream_listener_filter_error_rate",
		Priority: prioListenerDownstreamFilterReadErrorsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_listener_downstream_listener_filter_error_%s", Name: "read", Algo: collectorapi.Incremental},
		},
	}

	listenerDownstreamActiveSocketsCountChartTmpl = collectorapi.Chart{
		ID:       "listener_downstream_pre_cx_active_%s",
		Title:    "Listener downstream current active sockets",
		Units:    "sockets",
		Fam:      "downstream sockets",
		Ctx:      "envoy.listener_downstream_pre_cx_active_count",
		Priority: prioListenerDownstreamActiveSocketsCount,
		Dims: collectorapi.Dims{
			{ID: "envoy_listener_downstream_pre_cx_active_%s", Name: "active"},
		},
	}
	listenerDownstreamTimedOutSocketsRateChartTmpl = collectorapi.Chart{
		ID:       "listener_downstream_pre_cx_timeout_%s",
		Title:    "Listener downstream timed out sockets",
		Units:    "sockets/s",
		Fam:      "downstream sockets",
		Ctx:      "envoy.listener_downstream_pre_cx_timeout_rate",
		Priority: prioListenerDownstreamTimedOutSocketsRate,
		Dims: collectorapi.Dims{
			{ID: "envoy_listener_downstream_pre_cx_timeout_%s", Name: "timeout", Algo: collectorapi.Incremental},
		},
	}
)

func (c *Collector) addServerCharts(id string, labels labels.Labels) {
	c.addCharts(serverChartsTmpl.Copy(), id, labels)
}

func (c *Collector) addClusterManagerCharts(id string, labels labels.Labels) {
	c.addCharts(clusterManagerChartsTmpl.Copy(), id, labels)
}

func (c *Collector) addClusterUpstreamCharts(id string, labels labels.Labels) {
	c.addCharts(clusterUpstreamChartsTmpl.Copy(), id, labels)
}

func (c *Collector) addListenerManagerCharts(id string, labels labels.Labels) {
	c.addCharts(listenerManagerChartsTmpl.Copy(), id, labels)
}

func (c *Collector) addListenerAdminDownstreamCharts(id string, labels labels.Labels) {
	c.addCharts(listenerAdminDownstreamChartsTmpl.Copy(), id, labels)
}

func (c *Collector) addListenerDownstreamCharts(id string, labels labels.Labels) {
	c.addCharts(listenerDownstreamChartsTmpl.Copy(), id, labels)
}

func (c *Collector) addCharts(charts *collectorapi.Charts, id string, labels labels.Labels) {
	charts = charts.Copy()

	for _, chart := range *charts {
		if id == "" {
			chart.ID = strings.Replace(chart.ID, "_%s", "", 1)
			for _, dim := range chart.Dims {
				dim.ID = strings.Replace(dim.ID, "_%s", "", 1)
			}
		} else {
			chart.ID = fmt.Sprintf(chart.ID, dotReplacer.Replace(id))
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, id)
			}
		}

		for _, lbl := range labels {
			chart.Labels = append(chart.Labels, collectorapi.Label{Key: lbl.Name, Value: lbl.Value})
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeCharts(id string) {
	if id == "" {
		return
	}

	id = dotReplacer.Replace(id)
	for _, chart := range *c.Charts() {
		if strings.HasSuffix(chart.ID, id) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

var dotReplacer = strings.NewReplacer(".", "_")
