// SPDX-License-Identifier: GPL-3.0-or-later

package cassandra

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioClientRequestsRate = collectorapi.Priority + iota

	prioClientRequestReadLatency
	prioClientRequestWriteLatency
	prioClientRequestsLatency

	prioKeyCacheHitRatio
	prioRowCacheHitRatio
	prioKeyCacheHitRate
	prioRowCacheHitRate
	prioKeyCacheUtilization
	prioRowCacheUtilization
	prioKeyCacheSize
	prioRowCacheSize

	prioStorageLiveDiskSpaceUsed

	prioCompactionCompletedTasksRate
	prioCompactionPendingTasksCount
	prioCompactionBytesCompactedRate

	prioThreadPoolActiveTasksCount
	prioThreadPoolPendingTasksCount
	prioThreadPoolBlockedTasksCount
	prioThreadPoolBlockedTasksRate

	prioJVMMemoryUsed
	prioJVMGCCount
	prioJVMGCTime

	prioDroppedMessagesRate
	prioRequestsTimeoutsRate
	prioRequestsUnavailablesRate
	prioRequestsFailuresRate
	prioStorageExceptionsRate
)

var baseCharts = collectorapi.Charts{
	chartClientRequestsRate.Copy(),

	chartClientRequestsLatency.Copy(),
	chartClientRequestReadLatencyHistogram.Copy(),
	chartClientRequestWriteLatencyHistogram.Copy(),

	chartKeyCacheHitRatio.Copy(),
	chartRowCacheHitRatio.Copy(),
	chartKeyCacheHitRate.Copy(),
	chartRowCacheHitRate.Copy(),
	chartKeyCacheUtilization.Copy(),
	chartRowCacheUtilization.Copy(),
	chartKeyCacheSize.Copy(),
	chartRowCacheSize.Copy(),

	chartStorageLiveDiskSpaceUsed.Copy(),

	chartCompactionCompletedTasksRate.Copy(),
	chartCompactionPendingTasksCount.Copy(),
	chartCompactionBytesCompactedRate.Copy(),

	chartJVMMemoryUsed.Copy(),
	chartJVMGCRate.Copy(),
	chartJVMGCTime.Copy(),

	chartDroppedMessagesRate.Copy(),
	chartClientRequestTimeoutsRate.Copy(),
	chartClientRequestUnavailablesRate.Copy(),
	chartClientRequestFailuresRate.Copy(),
	chartStorageExceptionsRate.Copy(),
}

var (
	chartClientRequestsRate = collectorapi.Chart{
		ID:       "client_requests_rate",
		Title:    "Client requests rate",
		Units:    "requests/s",
		Fam:      "throughput",
		Ctx:      "cassandra.client_requests_rate",
		Priority: prioClientRequestsRate,
		Dims: collectorapi.Dims{
			{ID: "client_request_latency_reads", Name: "read", Algo: collectorapi.Incremental},
			{ID: "client_request_latency_writes", Name: "write", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
)

var (
	chartClientRequestReadLatencyHistogram = collectorapi.Chart{
		ID:       "client_request_read_latency_histogram",
		Title:    "Client request read latency histogram",
		Units:    "seconds",
		Fam:      "latency",
		Ctx:      "cassandra.client_request_read_latency_histogram",
		Priority: prioClientRequestReadLatency,
		Dims: collectorapi.Dims{
			{ID: "client_request_read_latency_p50", Name: "p50", Div: 1e6},
			{ID: "client_request_read_latency_p75", Name: "p75", Div: 1e6},
			{ID: "client_request_read_latency_p95", Name: "p95", Div: 1e6},
			{ID: "client_request_read_latency_p98", Name: "p98", Div: 1e6},
			{ID: "client_request_read_latency_p99", Name: "p99", Div: 1e6},
			{ID: "client_request_read_latency_p999", Name: "p999", Div: 1e6},
		},
	}
	chartClientRequestWriteLatencyHistogram = collectorapi.Chart{
		ID:       "client_request_write_latency_histogram",
		Title:    "Client request write latency histogram",
		Units:    "seconds",
		Fam:      "latency",
		Ctx:      "cassandra.client_request_write_latency_histogram",
		Priority: prioClientRequestWriteLatency,
		Dims: collectorapi.Dims{
			{ID: "client_request_write_latency_p50", Name: "p50", Div: 1e6},
			{ID: "client_request_write_latency_p75", Name: "p75", Div: 1e6},
			{ID: "client_request_write_latency_p95", Name: "p95", Div: 1e6},
			{ID: "client_request_write_latency_p98", Name: "p98", Div: 1e6},
			{ID: "client_request_write_latency_p99", Name: "p99", Div: 1e6},
			{ID: "client_request_write_latency_p999", Name: "p999", Div: 1e6},
		},
	}
	chartClientRequestsLatency = collectorapi.Chart{
		ID:       "client_requests_latency",
		Title:    "Client requests total latency",
		Units:    "seconds",
		Fam:      "latency",
		Ctx:      "cassandra.client_requests_latency",
		Priority: prioClientRequestsLatency,
		Dims: collectorapi.Dims{
			{ID: "client_request_total_latency_reads", Name: "read", Algo: collectorapi.Incremental, Div: 1e6},
			{ID: "client_request_total_latency_writes", Name: "write", Algo: collectorapi.Incremental, Div: 1e6},
		},
	}
)

var (
	chartKeyCacheHitRatio = collectorapi.Chart{
		ID:       "key_cache_hit_ratio",
		Title:    "Key cache hit ratio",
		Units:    "percentage",
		Fam:      "cache",
		Ctx:      "cassandra.key_cache_hit_ratio",
		Priority: prioKeyCacheHitRatio,
		Dims: collectorapi.Dims{
			{ID: "key_cache_hit_ratio", Name: "hit_ratio", Div: 1000},
		},
	}
	chartKeyCacheHitRate = collectorapi.Chart{
		ID:       "key_cache_hit_rate",
		Title:    "Key cache hit rate",
		Units:    "events/s",
		Fam:      "cache",
		Ctx:      "cassandra.key_cache_hit_rate",
		Priority: prioKeyCacheHitRate,
		Type:     collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "key_cache_hits", Name: "hits", Algo: collectorapi.Incremental},
			{ID: "key_cache_misses", Name: "misses", Algo: collectorapi.Incremental},
		},
	}
	chartKeyCacheUtilization = collectorapi.Chart{
		ID:       "key_cache_utilization",
		Title:    "Key cache utilization",
		Units:    "percentage",
		Fam:      "cache",
		Ctx:      "cassandra.key_cache_utilization",
		Priority: prioKeyCacheUtilization,
		Dims: collectorapi.Dims{
			{ID: "key_cache_utilization", Name: "used", Div: 1000},
		},
	}
	chartKeyCacheSize = collectorapi.Chart{
		ID:       "key_cache_size",
		Title:    "Key cache size",
		Units:    "bytes",
		Fam:      "cache",
		Ctx:      "cassandra.key_cache_size",
		Priority: prioKeyCacheSize,
		Dims: collectorapi.Dims{
			{ID: "key_cache_size", Name: "size"},
		},
	}

	chartRowCacheHitRatio = collectorapi.Chart{
		ID:       "row_cache_hit_ratio",
		Title:    "Row cache hit ratio",
		Units:    "percentage",
		Fam:      "cache",
		Ctx:      "cassandra.row_cache_hit_ratio",
		Priority: prioRowCacheHitRatio,
		Dims: collectorapi.Dims{
			{ID: "row_cache_hit_ratio", Name: "hit_ratio", Div: 1000},
		},
	}
	chartRowCacheHitRate = collectorapi.Chart{
		ID:       "row_cache_hit_rate",
		Title:    "Row cache hit rate",
		Units:    "events/s",
		Fam:      "cache",
		Ctx:      "cassandra.row_cache_hit_rate",
		Priority: prioRowCacheHitRate,
		Type:     collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "row_cache_hits", Name: "hits", Algo: collectorapi.Incremental},
			{ID: "row_cache_misses", Name: "misses", Algo: collectorapi.Incremental},
		},
	}
	chartRowCacheUtilization = collectorapi.Chart{
		ID:       "row_cache_utilization",
		Title:    "Row cache utilization",
		Units:    "percentage",
		Fam:      "cache",
		Ctx:      "cassandra.row_cache_utilization",
		Priority: prioRowCacheUtilization,
		Dims: collectorapi.Dims{
			{ID: "row_cache_utilization", Name: "used", Div: 1000},
		},
	}
	chartRowCacheSize = collectorapi.Chart{
		ID:       "row_cache_size",
		Title:    "Row cache size",
		Units:    "bytes",
		Fam:      "cache",
		Ctx:      "cassandra.row_cache_size",
		Priority: prioRowCacheSize,
		Dims: collectorapi.Dims{
			{ID: "row_cache_size", Name: "size"},
		},
	}
)

var (
	chartStorageLiveDiskSpaceUsed = collectorapi.Chart{
		ID:       "storage_live_disk_space_used",
		Title:    "Disk space used by live data",
		Units:    "bytes",
		Fam:      "disk usage",
		Ctx:      "cassandra.storage_live_disk_space_used",
		Priority: prioStorageLiveDiskSpaceUsed,
		Dims: collectorapi.Dims{
			{ID: "storage_load", Name: "used"},
		},
	}
)

var (
	chartCompactionCompletedTasksRate = collectorapi.Chart{
		ID:       "compaction_completed_tasks_rate",
		Title:    "Completed compactions rate",
		Units:    "tasks/s",
		Fam:      "compaction",
		Ctx:      "cassandra.compaction_completed_tasks_rate",
		Priority: prioCompactionCompletedTasksRate,
		Dims: collectorapi.Dims{
			{ID: "compaction_completed_tasks", Name: "completed", Algo: collectorapi.Incremental},
		},
	}
	chartCompactionPendingTasksCount = collectorapi.Chart{
		ID:       "compaction_pending_tasks_count",
		Title:    "Pending compactions",
		Units:    "tasks",
		Fam:      "compaction",
		Ctx:      "cassandra.compaction_pending_tasks_count",
		Priority: prioCompactionPendingTasksCount,
		Dims: collectorapi.Dims{
			{ID: "compaction_pending_tasks", Name: "pending"},
		},
	}
	chartCompactionBytesCompactedRate = collectorapi.Chart{
		ID:       "compaction_compacted_rate",
		Title:    "Compaction data rate",
		Units:    "bytes/s",
		Fam:      "compaction",
		Ctx:      "cassandra.compaction_compacted_rate",
		Priority: prioCompactionBytesCompactedRate,
		Dims: collectorapi.Dims{
			{ID: "compaction_bytes_compacted", Name: "compacted", Algo: collectorapi.Incremental},
		},
	}
)

var (
	chartsTmplThreadPool = collectorapi.Charts{
		chartTmplThreadPoolActiveTasksCount.Copy(),
		chartTmplThreadPoolPendingTasksCount.Copy(),
		chartTmplThreadPoolBlockedTasksCount.Copy(),
		chartTmplThreadPoolBlockedTasksRate.Copy(),
	}

	chartTmplThreadPoolActiveTasksCount = collectorapi.Chart{
		ID:       "thread_pool_%s_active_tasks_count",
		Title:    "Active tasks",
		Units:    "tasks",
		Fam:      "thread pools",
		Ctx:      "cassandra.thread_pool_active_tasks_count",
		Priority: prioThreadPoolActiveTasksCount,
		Dims: collectorapi.Dims{
			{ID: "thread_pool_%s_active_tasks", Name: "active"},
		},
	}
	chartTmplThreadPoolPendingTasksCount = collectorapi.Chart{
		ID:       "thread_pool_%s_pending_tasks_count",
		Title:    "Pending tasks",
		Units:    "tasks",
		Fam:      "thread pools",
		Ctx:      "cassandra.thread_pool_pending_tasks_count",
		Priority: prioThreadPoolPendingTasksCount,
		Dims: collectorapi.Dims{
			{ID: "thread_pool_%s_pending_tasks", Name: "pending"},
		},
	}
	chartTmplThreadPoolBlockedTasksCount = collectorapi.Chart{
		ID:       "thread_pool_%s_blocked_tasks_count",
		Title:    "Blocked tasks",
		Units:    "tasks",
		Fam:      "thread pools",
		Ctx:      "cassandra.thread_pool_blocked_tasks_count",
		Priority: prioThreadPoolBlockedTasksCount,
		Dims: collectorapi.Dims{
			{ID: "thread_pool_%s_blocked_tasks", Name: "blocked"},
		},
	}
	chartTmplThreadPoolBlockedTasksRate = collectorapi.Chart{
		ID:       "thread_pool_%s_blocked_tasks_rate",
		Title:    "Blocked tasks rate",
		Units:    "tasks/s",
		Fam:      "thread pools",
		Ctx:      "cassandra.thread_pool_blocked_tasks_rate",
		Priority: prioThreadPoolBlockedTasksRate,
		Dims: collectorapi.Dims{
			{ID: "thread_pool_%s_total_blocked_tasks", Name: "blocked", Algo: collectorapi.Incremental},
		},
	}
)

var (
	chartJVMMemoryUsed = collectorapi.Chart{
		ID:       "jvm_memory_used",
		Title:    "Memory used",
		Units:    "bytes",
		Fam:      "jvm runtime",
		Ctx:      "cassandra.jvm_memory_used",
		Priority: prioJVMMemoryUsed,
		Type:     collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "jvm_memory_heap_used", Name: "heap"},
			{ID: "jvm_memory_nonheap_used", Name: "nonheap"},
		},
	}
	chartJVMGCRate = collectorapi.Chart{
		ID:       "jvm_gc_rate",
		Title:    "Garbage collections rate",
		Units:    "gc/s",
		Fam:      "jvm runtime",
		Ctx:      "cassandra.jvm_gc_rate",
		Priority: prioJVMGCCount,
		Dims: collectorapi.Dims{
			{ID: "jvm_gc_parnew_count", Name: "parnew", Algo: collectorapi.Incremental},
			{ID: "jvm_gc_cms_count", Name: "cms", Algo: collectorapi.Incremental},
		},
	}
	chartJVMGCTime = collectorapi.Chart{
		ID:       "jvm_gc_time",
		Title:    "Garbage collection time",
		Units:    "seconds",
		Fam:      "jvm runtime",
		Ctx:      "cassandra.jvm_gc_time",
		Priority: prioJVMGCTime,
		Dims: collectorapi.Dims{
			{ID: "jvm_gc_parnew_time", Name: "parnew", Algo: collectorapi.Incremental, Div: 1e9},
			{ID: "jvm_gc_cms_time", Name: "cms", Algo: collectorapi.Incremental, Div: 1e9},
		},
	}
)

var (
	chartDroppedMessagesRate = collectorapi.Chart{
		ID:       "dropped_messages_rate",
		Title:    "Dropped messages rate",
		Units:    "messages/s",
		Fam:      "errors",
		Ctx:      "cassandra.dropped_messages_rate",
		Priority: prioDroppedMessagesRate,
		Dims: collectorapi.Dims{
			{ID: "dropped_messages", Name: "dropped"},
		},
	}
	chartClientRequestTimeoutsRate = collectorapi.Chart{
		ID:       "client_requests_timeouts_rate",
		Title:    "Client requests timeouts rate",
		Units:    "timeouts/s",
		Fam:      "errors",
		Ctx:      "cassandra.client_requests_timeouts_rate",
		Priority: prioRequestsTimeoutsRate,
		Dims: collectorapi.Dims{
			{ID: "client_request_timeouts_reads", Name: "read", Algo: collectorapi.Incremental},
			{ID: "client_request_timeouts_writes", Name: "write", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	chartClientRequestUnavailablesRate = collectorapi.Chart{
		ID:       "client_requests_unavailables_rate",
		Title:    "Client requests unavailable exceptions rate",
		Units:    "exceptions/s",
		Fam:      "errors",
		Ctx:      "cassandra.client_requests_unavailables_rate",
		Priority: prioRequestsUnavailablesRate,
		Dims: collectorapi.Dims{
			{ID: "client_request_unavailables_reads", Name: "read", Algo: collectorapi.Incremental},
			{ID: "client_request_unavailables_writes", Name: "write", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	chartClientRequestFailuresRate = collectorapi.Chart{
		ID:       "client_requests_failures_rate",
		Title:    "Client requests failures rate",
		Units:    "failures/s",
		Fam:      "errors",
		Ctx:      "cassandra.client_requests_failures_rate",
		Priority: prioRequestsFailuresRate,
		Dims: collectorapi.Dims{
			{ID: "client_request_failures_reads", Name: "read", Algo: collectorapi.Incremental},
			{ID: "client_request_failures_writes", Name: "write", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	chartStorageExceptionsRate = collectorapi.Chart{
		ID:       "storage_exceptions_rate",
		Title:    "Storage exceptions rate",
		Units:    "exceptions/s",
		Fam:      "errors",
		Ctx:      "cassandra.storage_exceptions_rate",
		Priority: prioStorageExceptionsRate,
		Dims: collectorapi.Dims{
			{ID: "storage_exceptions", Name: "storage", Algo: collectorapi.Incremental},
		},
	}
)

func (c *Collector) addThreadPoolCharts(pool *threadPoolMetrics) {
	charts := chartsTmplThreadPool.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, pool.name)
		chart.Labels = []collectorapi.Label{
			{Key: "thread_pool", Value: pool.name},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, pool.name)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}
