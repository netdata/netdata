// SPDX-License-Identifier: GPL-3.0-or-later

package elasticsearch

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioNodeIndicesIndexingOps = module.Priority + iota
	prioNodeIndicesIndexingOpsCurrent
	prioNodeIndicesIndexingOpsTime
	prioNodeIndicesSearchOps
	prioNodeIndicesSearchOpsCurrent
	prioNodeIndicesSearchOpsTime
	prioNodeIndicesRefreshOps
	prioNodeIndicesRefreshOpsTime
	prioNodeIndicesFlushOps
	prioNodeIndicesFlushOpsTime
	prioNodeIndicesFieldDataMemoryUsage
	prioNodeIndicesFieldDataEvictions
	prioNodeIndicesSegmentsCount
	prioNodeIndicesSegmentsMemoryUsageTotal
	prioNodeIndicesSegmentsMemoryUsage
	prioNodeIndicesTransLogOps
	prioNodeIndexTransLogSize
	prioNodeFileDescriptors
	prioNodeJVMMemHeap
	prioNodeJVMMemHeapBytes
	prioNodeJVMBufferPoolsCount
	prioNodeJVMBufferPoolDirectMemory
	prioNodeJVMBufferPoolMappedMemory
	prioNodeJVMGCCount
	prioNodeJVMGCTime
	prioNodeThreadPoolQueued
	prioNodeThreadPoolRejected
	prioNodeClusterCommunicationPackets
	prioNodeClusterCommunication
	prioNodeHTTPConnections
	prioNodeBreakersTrips

	prioClusterStatus
	prioClusterNodesCount
	prioClusterShardsCount
	prioClusterPendingTasks
	prioClusterInFlightFetchesCount

	prioClusterIndicesCount
	prioClusterIndicesShardsCount
	prioClusterIndicesDocsCount
	prioClusterIndicesStoreSize
	prioClusterIndicesQueryCache
	prioClusterNodesByRoleCount

	prioNodeIndexHealth
	prioNodeIndexShardsCount
	prioNodeIndexDocsCount
	prioNodeIndexStoreSize
)

var nodeChartsTmpl = module.Charts{
	nodeIndicesIndexingOpsChartTmpl.Copy(),
	nodeIndicesIndexingOpsCurrentChartTmpl.Copy(),
	nodeIndicesIndexingOpsTimeChartTmpl.Copy(),

	nodeIndicesSearchOpsChartTmpl.Copy(),
	nodeIndicesSearchOpsCurrentChartTmpl.Copy(),
	nodeIndicesSearchOpsTimeChartTmpl.Copy(),

	nodeIndicesRefreshOpsChartTmpl.Copy(),
	nodeIndicesRefreshOpsTimeChartTmpl.Copy(),

	nodeIndicesFlushOpsChartTmpl.Copy(),
	nodeIndicesFlushOpsTimeChartTmpl.Copy(),

	nodeIndicesFieldDataMemoryUsageChartTmpl.Copy(),
	nodeIndicesFieldDataEvictionsChartTmpl.Copy(),

	nodeIndicesSegmentsCountChartTmpl.Copy(),
	nodeIndicesSegmentsMemoryUsageTotalChartTmpl.Copy(),
	nodeIndicesSegmentsMemoryUsageChartTmpl.Copy(),

	nodeIndicesTransLogOpsChartTmpl.Copy(),
	nodeIndexTransLogSizeChartTmpl.Copy(),

	nodeFileDescriptorsChartTmpl.Copy(),

	nodeJVMMemHeapChartTmpl.Copy(),
	nodeJVMMemHeapBytesChartTmpl.Copy(),
	nodeJVMBufferPoolsCountChartTmpl.Copy(),
	nodeJVMBufferPoolDirectMemoryChartTmpl.Copy(),
	nodeJVMBufferPoolMappedMemoryChartTmpl.Copy(),
	nodeJVMGCCountChartTmpl.Copy(),
	nodeJVMGCTimeChartTmpl.Copy(),

	nodeThreadPoolQueuedChartTmpl.Copy(),
	nodeThreadPoolRejectedChartTmpl.Copy(),

	nodeClusterCommunicationPacketsChartTmpl.Copy(),
	nodeClusterCommunicationChartTmpl.Copy(),

	nodeHTTPConnectionsChartTmpl.Copy(),

	nodeBreakersTripsChartTmpl.Copy(),
}

var (
	nodeIndicesIndexingOpsChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_indices_indexing_operations",
		Title:    "Indexing Operations",
		Units:    "operations/s",
		Fam:      "indices indexing",
		Ctx:      "elasticsearch.node_indices_indexing",
		Priority: prioNodeIndicesIndexingOps,
		Dims: module.Dims{
			{ID: "node_%s_indices_indexing_index_total", Name: "index", Algo: module.Incremental},
		},
	}
	nodeIndicesIndexingOpsCurrentChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_indices_indexing_operations_current",
		Title:    "Indexing Operations Current",
		Units:    "operations",
		Fam:      "indices indexing",
		Ctx:      "elasticsearch.node_indices_indexing_current",
		Priority: prioNodeIndicesIndexingOpsCurrent,
		Dims: module.Dims{
			{ID: "node_%s_indices_indexing_index_current", Name: "index"},
		},
	}
	nodeIndicesIndexingOpsTimeChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_indices_indexing_operations_time",
		Title:    "Time Spent On Indexing Operations",
		Units:    "milliseconds",
		Fam:      "indices indexing",
		Ctx:      "elasticsearch.node_indices_indexing_time",
		Priority: prioNodeIndicesIndexingOpsTime,
		Dims: module.Dims{
			{ID: "node_%s_indices_indexing_index_time_in_millis", Name: "index", Algo: module.Incremental},
		},
	}

	nodeIndicesSearchOpsChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_indices_search_operations",
		Title:    "Search Operations",
		Units:    "operations/s",
		Fam:      "indices search",
		Ctx:      "elasticsearch.node_indices_search",
		Type:     module.Stacked,
		Priority: prioNodeIndicesSearchOps,
		Dims: module.Dims{
			{ID: "node_%s_indices_search_query_total", Name: "queries", Algo: module.Incremental},
			{ID: "node_%s_indices_search_fetch_total", Name: "fetches", Algo: module.Incremental},
		},
	}
	nodeIndicesSearchOpsCurrentChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_indices_search_operations_current",
		Title:    "Search Operations Current",
		Units:    "operations",
		Fam:      "indices search",
		Ctx:      "elasticsearch.node_indices_search_current",
		Type:     module.Stacked,
		Priority: prioNodeIndicesSearchOpsCurrent,
		Dims: module.Dims{
			{ID: "node_%s_indices_search_query_current", Name: "queries"},
			{ID: "node_%s_indices_search_fetch_current", Name: "fetches"},
		},
	}
	nodeIndicesSearchOpsTimeChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_indices_search_operations_time",
		Title:    "Time Spent On Search Operations",
		Units:    "milliseconds",
		Fam:      "indices search",
		Ctx:      "elasticsearch.node_indices_search_time",
		Type:     module.Stacked,
		Priority: prioNodeIndicesSearchOpsTime,
		Dims: module.Dims{
			{ID: "node_%s_indices_search_query_time_in_millis", Name: "query", Algo: module.Incremental},
			{ID: "node_%s_indices_search_fetch_time_in_millis", Name: "fetch", Algo: module.Incremental},
		},
	}

	nodeIndicesRefreshOpsChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_indices_refresh_operations",
		Title:    "Refresh Operations",
		Units:    "operations/s",
		Fam:      "indices refresh",
		Ctx:      "elasticsearch.node_indices_refresh",
		Priority: prioNodeIndicesRefreshOps,
		Dims: module.Dims{
			{ID: "node_%s_indices_refresh_total", Name: "refresh", Algo: module.Incremental},
		},
	}
	nodeIndicesRefreshOpsTimeChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_indices_refresh_operations_time",
		Title:    "Time Spent On Refresh Operations",
		Units:    "milliseconds",
		Fam:      "indices refresh",
		Ctx:      "elasticsearch.node_indices_refresh_time",
		Priority: prioNodeIndicesRefreshOpsTime,
		Dims: module.Dims{
			{ID: "node_%s_indices_refresh_total_time_in_millis", Name: "refresh", Algo: module.Incremental},
		},
	}

	nodeIndicesFlushOpsChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_indices_flush_operations",
		Title:    "Flush Operations",
		Units:    "operations/s",
		Fam:      "indices flush",
		Ctx:      "elasticsearch.node_indices_flush",
		Priority: prioNodeIndicesFlushOps,
		Dims: module.Dims{
			{ID: "node_%s_indices_flush_total", Name: "flush", Algo: module.Incremental},
		},
	}
	nodeIndicesFlushOpsTimeChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_indices_flush_operations_time",
		Title:    "Time Spent On Flush Operations",
		Units:    "milliseconds",
		Fam:      "indices flush",
		Ctx:      "elasticsearch.node_indices_flush_time",
		Priority: prioNodeIndicesFlushOpsTime,
		Dims: module.Dims{
			{ID: "node_%s_indices_flush_total_time_in_millis", Name: "flush", Algo: module.Incremental},
		},
	}

	nodeIndicesFieldDataMemoryUsageChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_indices_fielddata_memory_usage",
		Title:    "Fielddata Cache Memory Usage",
		Units:    "bytes",
		Fam:      "indices fielddata",
		Ctx:      "elasticsearch.node_indices_fielddata_memory_usage",
		Type:     module.Area,
		Priority: prioNodeIndicesFieldDataMemoryUsage,
		Dims: module.Dims{
			{ID: "node_%s_indices_fielddata_memory_size_in_bytes", Name: "used"},
		},
	}
	nodeIndicesFieldDataEvictionsChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_indices_fielddata_evictions",
		Title:    "Fielddata Evictions",
		Units:    "operations/s",
		Fam:      "indices fielddata",
		Ctx:      "elasticsearch.node_indices_fielddata_evictions",
		Priority: prioNodeIndicesFieldDataEvictions,
		Dims: module.Dims{
			{ID: "node_%s_indices_fielddata_evictions", Name: "evictions", Algo: module.Incremental},
		},
	}

	nodeIndicesSegmentsCountChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_indices_segments_count",
		Title:    "Segments Count",
		Units:    "segments",
		Fam:      "indices segments",
		Ctx:      "elasticsearch.node_indices_segments_count",
		Priority: prioNodeIndicesSegmentsCount,
		Dims: module.Dims{
			{ID: "node_%s_indices_segments_count", Name: "segments"},
		},
	}
	nodeIndicesSegmentsMemoryUsageTotalChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_indices_segments_memory_usage_total",
		Title:    "Segments Memory Usage Total",
		Units:    "bytes",
		Fam:      "indices segments",
		Ctx:      "elasticsearch.node_indices_segments_memory_usage_total",
		Priority: prioNodeIndicesSegmentsMemoryUsageTotal,
		Dims: module.Dims{
			{ID: "node_%s_indices_segments_memory_in_bytes", Name: "used"},
		},
	}
	nodeIndicesSegmentsMemoryUsageChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_indices_segments_memory_usage",
		Title:    "Segments Memory Usage",
		Units:    "bytes",
		Fam:      "indices segments",
		Ctx:      "elasticsearch.node_indices_segments_memory_usage",
		Type:     module.Stacked,
		Priority: prioNodeIndicesSegmentsMemoryUsage,
		Dims: module.Dims{
			{ID: "node_%s_indices_segments_terms_memory_in_bytes", Name: "terms"},
			{ID: "node_%s_indices_segments_stored_fields_memory_in_bytes", Name: "stored_fields"},
			{ID: "node_%s_indices_segments_term_vectors_memory_in_bytes", Name: "term_vectors"},
			{ID: "node_%s_indices_segments_norms_memory_in_bytes", Name: "norms"},
			{ID: "node_%s_indices_segments_points_memory_in_bytes", Name: "points"},
			{ID: "node_%s_indices_segments_doc_values_memory_in_bytes", Name: "doc_values"},
			{ID: "node_%s_indices_segments_index_writer_memory_in_bytes", Name: "index_writer"},
			{ID: "node_%s_indices_segments_version_map_memory_in_bytes", Name: "version_map"},
			{ID: "node_%s_indices_segments_fixed_bit_set_memory_in_bytes", Name: "fixed_bit_set"},
		},
	}

	nodeIndicesTransLogOpsChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_indices_translog_operations",
		Title:    "Translog Operations",
		Units:    "operations",
		Fam:      "indices translog",
		Ctx:      "elasticsearch.node_indices_translog_operations",
		Type:     module.Area,
		Priority: prioNodeIndicesTransLogOps,
		Dims: module.Dims{
			{ID: "node_%s_indices_translog_operations", Name: "total"},
			{ID: "node_%s_indices_translog_uncommitted_operations", Name: "uncommitted"},
		},
	}
	nodeIndexTransLogSizeChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_index_translog_size",
		Title:    "Translog Size",
		Units:    "bytes",
		Fam:      "indices translog",
		Ctx:      "elasticsearch.node_indices_translog_size",
		Type:     module.Area,
		Priority: prioNodeIndexTransLogSize,
		Dims: module.Dims{
			{ID: "node_%s_indices_translog_size_in_bytes", Name: "total"},
			{ID: "node_%s_indices_translog_uncommitted_size_in_bytes", Name: "uncommitted"},
		},
	}

	nodeFileDescriptorsChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_file_descriptors",
		Title:    "Process File Descriptors",
		Units:    "fd",
		Fam:      "process",
		Ctx:      "elasticsearch.node_file_descriptors",
		Priority: prioNodeFileDescriptors,
		Dims: module.Dims{
			{ID: "node_%s_process_open_file_descriptors", Name: "open"},
		},
	}

	nodeJVMMemHeapChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_jvm_mem_heap",
		Title:    "JVM Heap Percentage Currently in Use",
		Units:    "percentage",
		Fam:      "jvm",
		Ctx:      "elasticsearch.node_jvm_heap",
		Type:     module.Area,
		Priority: prioNodeJVMMemHeap,
		Dims: module.Dims{
			{ID: "node_%s_jvm_mem_heap_used_percent", Name: "inuse"},
		},
	}
	nodeJVMMemHeapBytesChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_jvm_mem_heap_bytes",
		Title:    "JVM Heap Commit And Usage",
		Units:    "bytes",
		Fam:      "jvm",
		Ctx:      "elasticsearch.node_jvm_heap_bytes",
		Type:     module.Area,
		Priority: prioNodeJVMMemHeapBytes,
		Dims: module.Dims{
			{ID: "node_%s_jvm_mem_heap_committed_in_bytes", Name: "committed"},
			{ID: "node_%s_jvm_mem_heap_used_in_bytes", Name: "used"},
		},
	}
	nodeJVMBufferPoolsCountChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_jvm_buffer_pools_count",
		Title:    "JVM Buffer Pools Count",
		Units:    "pools",
		Fam:      "jvm",
		Ctx:      "elasticsearch.node_jvm_buffer_pools_count",
		Priority: prioNodeJVMBufferPoolsCount,
		Dims: module.Dims{
			{ID: "node_%s_jvm_buffer_pools_direct_count", Name: "direct"},
			{ID: "node_%s_jvm_buffer_pools_mapped_count", Name: "mapped"},
		},
	}
	nodeJVMBufferPoolDirectMemoryChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_jvm_buffer_pool_direct_memory",
		Title:    "JVM Buffer Pool Direct Memory",
		Units:    "bytes",
		Fam:      "jvm",
		Ctx:      "elasticsearch.node_jvm_buffer_pool_direct_memory",
		Type:     module.Area,
		Priority: prioNodeJVMBufferPoolDirectMemory,
		Dims: module.Dims{
			{ID: "node_%s_jvm_buffer_pools_direct_total_capacity_in_bytes", Name: "total"},
			{ID: "node_%s_jvm_buffer_pools_direct_used_in_bytes", Name: "used"},
		},
	}
	nodeJVMBufferPoolMappedMemoryChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_jvm_buffer_pool_mapped_memory",
		Title:    "JVM Buffer Pool Mapped Memory",
		Units:    "bytes",
		Fam:      "jvm",
		Ctx:      "elasticsearch.node_jvm_buffer_pool_mapped_memory",
		Type:     module.Area,
		Priority: prioNodeJVMBufferPoolMappedMemory,
		Dims: module.Dims{
			{ID: "node_%s_jvm_buffer_pools_mapped_total_capacity_in_bytes", Name: "total"},
			{ID: "node_%s_jvm_buffer_pools_mapped_used_in_bytes", Name: "used"},
		},
	}
	nodeJVMGCCountChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_jvm_gc_count",
		Title:    "JVM Garbage Collections",
		Units:    "gc/s",
		Fam:      "jvm",
		Ctx:      "elasticsearch.node_jvm_gc_count",
		Type:     module.Stacked,
		Priority: prioNodeJVMGCCount,
		Dims: module.Dims{
			{ID: "node_%s_jvm_gc_collectors_young_collection_count", Name: "young", Algo: module.Incremental},
			{ID: "node_%s_jvm_gc_collectors_old_collection_count", Name: "old", Algo: module.Incremental},
		},
	}
	nodeJVMGCTimeChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_jvm_gc_time",
		Title:    "JVM Time Spent On Garbage Collections",
		Units:    "milliseconds",
		Fam:      "jvm",
		Ctx:      "elasticsearch.node_jvm_gc_time",
		Type:     module.Stacked,
		Priority: prioNodeJVMGCTime,
		Dims: module.Dims{
			{ID: "node_%s_jvm_gc_collectors_young_collection_time_in_millis", Name: "young", Algo: module.Incremental},
			{ID: "node_%s_jvm_gc_collectors_old_collection_time_in_millis", Name: "old", Algo: module.Incremental},
		},
	}

	nodeThreadPoolQueuedChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_thread_pool_queued",
		Title:    "Thread Pool Queued Threads Count",
		Units:    "threads",
		Fam:      "thread pool",
		Ctx:      "elasticsearch.node_thread_pool_queued",
		Type:     module.Stacked,
		Priority: prioNodeThreadPoolQueued,
		Dims: module.Dims{
			{ID: "node_%s_thread_pool_generic_queue", Name: "generic"},
			{ID: "node_%s_thread_pool_search_queue", Name: "search"},
			{ID: "node_%s_thread_pool_search_throttled_queue", Name: "search_throttled"},
			{ID: "node_%s_thread_pool_get_queue", Name: "get"},
			{ID: "node_%s_thread_pool_analyze_queue", Name: "analyze"},
			{ID: "node_%s_thread_pool_write_queue", Name: "write"},
			{ID: "node_%s_thread_pool_snapshot_queue", Name: "snapshot"},
			{ID: "node_%s_thread_pool_warmer_queue", Name: "warmer"},
			{ID: "node_%s_thread_pool_refresh_queue", Name: "refresh"},
			{ID: "node_%s_thread_pool_listener_queue", Name: "listener"},
			{ID: "node_%s_thread_pool_fetch_shard_started_queue", Name: "fetch_shard_started"},
			{ID: "node_%s_thread_pool_fetch_shard_store_queue", Name: "fetch_shard_store"},
			{ID: "node_%s_thread_pool_flush_queue", Name: "flush"},
			{ID: "node_%s_thread_pool_force_merge_queue", Name: "force_merge"},
			{ID: "node_%s_thread_pool_management_queue", Name: "management"},
		},
	}
	nodeThreadPoolRejectedChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_thread_pool_rejected",
		Title:    "Thread Pool Rejected Threads Count",
		Units:    "threads",
		Fam:      "thread pool",
		Ctx:      "elasticsearch.node_thread_pool_rejected",
		Type:     module.Stacked,
		Priority: prioNodeThreadPoolRejected,
		Dims: module.Dims{
			{ID: "node_%s_thread_pool_generic_rejected", Name: "generic"},
			{ID: "node_%s_thread_pool_search_rejected", Name: "search"},
			{ID: "node_%s_thread_pool_search_throttled_rejected", Name: "search_throttled"},
			{ID: "node_%s_thread_pool_get_rejected", Name: "get"},
			{ID: "node_%s_thread_pool_analyze_rejected", Name: "analyze"},
			{ID: "node_%s_thread_pool_write_rejected", Name: "write"},
			{ID: "node_%s_thread_pool_snapshot_rejected", Name: "snapshot"},
			{ID: "node_%s_thread_pool_warmer_rejected", Name: "warmer"},
			{ID: "node_%s_thread_pool_refresh_rejected", Name: "refresh"},
			{ID: "node_%s_thread_pool_listener_rejected", Name: "listener"},
			{ID: "node_%s_thread_pool_fetch_shard_started_rejected", Name: "fetch_shard_started"},
			{ID: "node_%s_thread_pool_fetch_shard_store_rejected", Name: "fetch_shard_store"},
			{ID: "node_%s_thread_pool_flush_rejected", Name: "flush"},
			{ID: "node_%s_thread_pool_force_merge_rejected", Name: "force_merge"},
			{ID: "node_%s_thread_pool_management_rejected", Name: "management"},
		},
	}

	nodeClusterCommunicationPacketsChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_cluster_communication_packets",
		Title:    "Node Cluster Communication",
		Units:    "pps",
		Fam:      "transport",
		Ctx:      "elasticsearch.node_cluster_communication_packets",
		Priority: prioNodeClusterCommunicationPackets,
		Dims: module.Dims{
			{ID: "node_%s_transport_rx_count", Name: "received", Algo: module.Incremental},
			{ID: "node_%s_transport_tx_count", Name: "sent", Mul: -1, Algo: module.Incremental},
		},
	}
	nodeClusterCommunicationChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_cluster_communication_traffic",
		Title:    "Cluster Communication Bandwidth",
		Units:    "bytes/s",
		Fam:      "transport",
		Ctx:      "elasticsearch.node_cluster_communication_traffic",
		Priority: prioNodeClusterCommunication,
		Dims: module.Dims{
			{ID: "node_%s_transport_rx_size_in_bytes", Name: "received", Algo: module.Incremental},
			{ID: "node_%s_transport_tx_size_in_bytes", Name: "sent", Mul: -1, Algo: module.Incremental},
		},
	}

	nodeHTTPConnectionsChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_http_connections",
		Title:    "HTTP Connections",
		Units:    "connections",
		Fam:      "http",
		Ctx:      "elasticsearch.node_http_connections",
		Priority: prioNodeHTTPConnections,
		Dims: module.Dims{
			{ID: "node_%s_http_current_open", Name: "open"},
		},
	}

	nodeBreakersTripsChartTmpl = module.Chart{
		ID:       "node_%s_cluster_%s_breakers_trips",
		Title:    "Circuit Breaker Trips Count",
		Units:    "trips/s",
		Fam:      "circuit breakers",
		Ctx:      "elasticsearch.node_breakers_trips",
		Type:     module.Stacked,
		Priority: prioNodeBreakersTrips,
		Dims: module.Dims{
			{ID: "node_%s_breakers_request_tripped", Name: "requests", Algo: module.Incremental},
			{ID: "node_%s_breakers_fielddata_tripped", Name: "fielddata", Algo: module.Incremental},
			{ID: "node_%s_breakers_in_flight_requests_tripped", Name: "in_flight_requests", Algo: module.Incremental},
			{ID: "node_%s_breakers_model_inference_tripped", Name: "model_inference", Algo: module.Incremental},
			{ID: "node_%s_breakers_accounting_tripped", Name: "accounting", Algo: module.Incremental},
			{ID: "node_%s_breakers_parent_tripped", Name: "parent", Algo: module.Incremental},
		},
	}
)

var clusterHealthChartsTmpl = module.Charts{
	clusterStatusChartTmpl.Copy(),
	clusterNodesCountChartTmpl.Copy(),
	clusterShardsCountChartTmpl.Copy(),
	clusterPendingTasksChartTmpl.Copy(),
	clusterInFlightFetchesCountChartTmpl.Copy(),
}

var (
	clusterStatusChartTmpl = module.Chart{
		ID:       "cluster_%s_status",
		Title:    "Cluster Status",
		Units:    "status",
		Fam:      "cluster health",
		Ctx:      "elasticsearch.cluster_health_status",
		Priority: prioClusterStatus,
		Dims: module.Dims{
			{ID: "cluster_status_green", Name: "green"},
			{ID: "cluster_status_red", Name: "red"},
			{ID: "cluster_status_yellow", Name: "yellow"},
		},
	}
	clusterNodesCountChartTmpl = module.Chart{
		ID:       "cluster_%s_number_of_nodes",
		Title:    "Cluster Nodes Count",
		Units:    "nodes",
		Fam:      "cluster health",
		Ctx:      "elasticsearch.cluster_number_of_nodes",
		Priority: prioClusterNodesCount,
		Dims: module.Dims{
			{ID: "cluster_number_of_nodes", Name: "nodes"},
			{ID: "cluster_number_of_data_nodes", Name: "data_nodes"},
		},
	}
	clusterShardsCountChartTmpl = module.Chart{
		ID:       "cluster_%s_shards_count",
		Title:    "Cluster Shards Count",
		Units:    "shards",
		Fam:      "cluster health",
		Ctx:      "elasticsearch.cluster_shards_count",
		Priority: prioClusterShardsCount,
		Dims: module.Dims{
			{ID: "cluster_active_primary_shards", Name: "active_primary"},
			{ID: "cluster_active_shards", Name: "active"},
			{ID: "cluster_relocating_shards", Name: "relocating"},
			{ID: "cluster_initializing_shards", Name: "initializing"},
			{ID: "cluster_unassigned_shards", Name: "unassigned"},
			{ID: "cluster_delayed_unassigned_shards", Name: "delayed_unassigned"},
		},
	}
	clusterPendingTasksChartTmpl = module.Chart{
		ID:       "cluster_%s_pending_tasks",
		Title:    "Cluster Pending Tasks",
		Units:    "tasks",
		Fam:      "cluster health",
		Ctx:      "elasticsearch.cluster_pending_tasks",
		Priority: prioClusterPendingTasks,
		Dims: module.Dims{
			{ID: "cluster_number_of_pending_tasks", Name: "pending"},
		},
	}
	clusterInFlightFetchesCountChartTmpl = module.Chart{
		ID:       "cluster_%s_number_of_in_flight_fetch",
		Title:    "Cluster Unfinished Fetches",
		Units:    "fetches",
		Fam:      "cluster health",
		Ctx:      "elasticsearch.cluster_number_of_in_flight_fetch",
		Priority: prioClusterInFlightFetchesCount,
		Dims: module.Dims{
			{ID: "cluster_number_of_in_flight_fetch", Name: "in_flight_fetch"},
		},
	}
)

var clusterStatsChartsTmpl = module.Charts{
	clusterIndicesCountChartTmpl.Copy(),
	clusterIndicesShardsCountChartTmpl.Copy(),
	clusterIndicesDocsCountChartTmpl.Copy(),
	clusterIndicesStoreSizeChartTmpl.Copy(),
	clusterIndicesQueryCacheChartTmpl.Copy(),
	clusterNodesByRoleCountChartTmpl.Copy(),
}

var (
	clusterIndicesCountChartTmpl = module.Chart{
		ID:       "cluster_%s_indices_count",
		Title:    "Cluster Indices Count",
		Units:    "indices",
		Fam:      "cluster stats",
		Ctx:      "elasticsearch.cluster_indices_count",
		Priority: prioClusterIndicesCount,
		Dims: module.Dims{
			{ID: "cluster_indices_count", Name: "indices"},
		},
	}
	clusterIndicesShardsCountChartTmpl = module.Chart{
		ID:       "cluster_%s_indices_shards_count",
		Title:    "Cluster Indices Shards Count",
		Units:    "shards",
		Fam:      "cluster stats",
		Ctx:      "elasticsearch.cluster_indices_shards_count",
		Priority: prioClusterIndicesShardsCount,
		Dims: module.Dims{
			{ID: "cluster_indices_shards_total", Name: "total"},
			{ID: "cluster_indices_shards_primaries", Name: "primaries"},
			{ID: "cluster_indices_shards_replication", Name: "replication"},
		},
	}
	clusterIndicesDocsCountChartTmpl = module.Chart{
		ID:       "cluster_%s_indices_docs_count",
		Title:    "Cluster Indices Docs Count",
		Units:    "docs",
		Fam:      "cluster stats",
		Ctx:      "elasticsearch.cluster_indices_docs_count",
		Priority: prioClusterIndicesDocsCount,
		Dims: module.Dims{
			{ID: "cluster_indices_docs_count", Name: "docs"},
		},
	}
	clusterIndicesStoreSizeChartTmpl = module.Chart{
		ID:       "cluster_%s_indices_store_size",
		Title:    "Cluster Indices Store Size",
		Units:    "bytes",
		Fam:      "cluster stats",
		Ctx:      "elasticsearch.cluster_indices_store_size",
		Priority: prioClusterIndicesStoreSize,
		Dims: module.Dims{
			{ID: "cluster_indices_store_size_in_bytes", Name: "size"},
		},
	}
	clusterIndicesQueryCacheChartTmpl = module.Chart{
		ID:       "cluster_%s_indices_query_cache",
		Title:    "Cluster Indices Query Cache",
		Units:    "events/s",
		Fam:      "cluster stats",
		Ctx:      "elasticsearch.cluster_indices_query_cache",
		Type:     module.Stacked,
		Priority: prioClusterIndicesQueryCache,
		Dims: module.Dims{
			{ID: "cluster_indices_query_cache_hit_count", Name: "hit", Algo: module.Incremental},
			{ID: "cluster_indices_query_cache_miss_count", Name: "miss", Algo: module.Incremental},
		},
	}
	clusterNodesByRoleCountChartTmpl = module.Chart{
		ID:       "cluster_%s_nodes_by_role_count",
		Title:    "Cluster Nodes By Role Count",
		Units:    "nodes",
		Fam:      "cluster stats",
		Ctx:      "elasticsearch.cluster_nodes_by_role_count",
		Priority: prioClusterNodesByRoleCount,
		Dims: module.Dims{
			{ID: "cluster_nodes_count_coordinating_only", Name: "coordinating_only"},
			{ID: "cluster_nodes_count_data", Name: "data"},
			{ID: "cluster_nodes_count_data_cold", Name: "data_cold"},
			{ID: "cluster_nodes_count_data_content", Name: "data_content"},
			{ID: "cluster_nodes_count_data_frozen", Name: "data_frozen"},
			{ID: "cluster_nodes_count_data_hot", Name: "data_hot"},
			{ID: "cluster_nodes_count_data_warm", Name: "data_warm"},
			{ID: "cluster_nodes_count_ingest", Name: "ingest"},
			{ID: "cluster_nodes_count_master", Name: "master"},
			{ID: "cluster_nodes_count_ml", Name: "ml"},
			{ID: "cluster_nodes_count_remote_cluster_client", Name: "remote_cluster_client"},
			{ID: "cluster_nodes_count_voting_only", Name: "voting_only"},
		},
	}
)

var nodeIndexChartsTmpl = module.Charts{
	nodeIndexHealthChartTmpl.Copy(),
	nodeIndexShardsCountChartTmpl.Copy(),
	nodeIndexDocsCountChartTmpl.Copy(),
	nodeIndexStoreSizeChartTmpl.Copy(),
}

var (
	nodeIndexHealthChartTmpl = module.Chart{
		ID:       "node_index_%s_cluster_%s_health",
		Title:    "Index Health",
		Units:    "status",
		Fam:      "index stats",
		Ctx:      "elasticsearch.node_index_health",
		Priority: prioNodeIndexHealth,
		Dims: module.Dims{
			{ID: "node_index_%s_stats_health_green", Name: "green"},
			{ID: "node_index_%s_stats_health_red", Name: "red"},
			{ID: "node_index_%s_stats_health_yellow", Name: "yellow"},
		},
	}
	nodeIndexShardsCountChartTmpl = module.Chart{
		ID:       "node_index_%s_cluster_%s_shards_count",
		Title:    "Index Shards Count",
		Units:    "shards",
		Fam:      "index stats",
		Ctx:      "elasticsearch.node_index_shards_count",
		Priority: prioNodeIndexShardsCount,
		Dims: module.Dims{
			{ID: "node_index_%s_stats_shards_count", Name: "shards"},
		},
	}
	nodeIndexDocsCountChartTmpl = module.Chart{
		ID:       "node_index_%s_cluster_%s_docs_count",
		Title:    "Index Docs Count",
		Units:    "docs",
		Fam:      "index stats",
		Ctx:      "elasticsearch.node_index_docs_count",
		Priority: prioNodeIndexDocsCount,
		Dims: module.Dims{
			{ID: "node_index_%s_stats_docs_count", Name: "docs"},
		},
	}
	nodeIndexStoreSizeChartTmpl = module.Chart{
		ID:       "node_index_%s_cluster_%s_store_size",
		Title:    "Index Store Size",
		Units:    "bytes",
		Fam:      "index stats",
		Ctx:      "elasticsearch.node_index_store_size",
		Priority: prioNodeIndexStoreSize,
		Dims: module.Dims{
			{ID: "node_index_%s_stats_store_size_in_bytes", Name: "store_size"},
		},
	}
)

func (c *Collector) addClusterStatsCharts() {
	charts := clusterStatsChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, c.clusterName)
		chart.Labels = []module.Label{
			{Key: "cluster_name", Value: c.clusterName},
		}
	}

	if err := c.charts.Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addClusterHealthCharts() {
	charts := clusterHealthChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, c.clusterName)
		chart.Labels = []module.Label{
			{Key: "cluster_name", Value: c.clusterName},
		}
	}

	if err := c.charts.Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addNodeCharts(nodeID string, node *esNodeStats) {
	charts := nodeChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, nodeID, c.clusterName)
		chart.Labels = []module.Label{
			{Key: "cluster_name", Value: c.clusterName},
			{Key: "node_name", Value: node.Name},
			{Key: "host", Value: node.Host},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, nodeID)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeNodeCharts(nodeID string) {
	px := fmt.Sprintf("node_%s_cluster_%s_", nodeID, c.clusterName)
	c.removeCharts(px)
}

func (c *Collector) addIndexCharts(index string) {
	charts := nodeIndexChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, index, c.clusterName)
		chart.Labels = []module.Label{
			{Key: "cluster_name", Value: c.clusterName},
			{Key: "index", Value: index},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, index)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeIndexCharts(index string) {
	px := fmt.Sprintf("node_index_%s_cluster_%s_", index, c.clusterName)
	c.removeCharts(px)
}

func (c *Collector) removeCharts(prefix string) {
	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
