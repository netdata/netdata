// SPDX-License-Identifier: GPL-3.0-or-later

package clickhouse

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioConnections = collectorapi.Priority + iota

	prioSlowReads
	prioReadBackoff

	prioMemoryUsage

	prioDiskSpaceUsage

	prioRunningQueries
	prioQueriesPreempted
	prioQueries
	prioSelectQueries
	prioInsertQueries
	prioQueriesMemoryLimitExceeded

	prioLongestRunningQueryTime
	prioQueriesLatency
	prioSelectQueriesLatency
	prioInsertQueriesLatency

	prioIO
	prioIOPS
	prioIOErrors
	prioIOSeeks
	prioIOFileOpens

	prioDatabaseTableSize
	prioDatabaseTableParts
	prioDatabaseTableRows

	prioReplicatedPartsCurrentActivity
	prioReplicasMaxAbsoluteDelay
	prioReadOnlyReplica
	prioReplicatedDataLoss
	prioReplicatedPartFetches
	prioReplicatedPartFetchesOfMerged
	prioReplicatedPartMerges

	prioInsertedBytes
	prioInsertedRows
	prioRejectedInserts
	prioDelayedInserts
	prioDelayedInsertsThrottleTime

	prioSelectedBytes
	prioSelectedRows
	prioSelectedParts
	prioSelectedRanges
	prioSelectedMarks

	prioMerges
	prioMergesLatency
	prioMergedUncompressedBytes
	prioMergedRows

	prioMergeTreeDataWriterRows
	prioMergeTreeDataWriterUncompressedBytes
	prioMergeTreeDataWriterCompressedBytes

	prioUncompressedCacheRequests
	prioMarkCacheRequests

	prioMaxPartCountForPartition
	prioParts

	prioDistributedSend
	prioDistributedConnectionTries
	prioDistributedConnectionFailTry
	prioDistributedConnectionFailAtAll

	prioDistributedFilesToInsert
	prioDistributedRejectedInserts
	prioDistributedDelayedInserts
	prioDistributedDelayedInsertsMilliseconds
	prioDistributedSyncInsertionTimeoutExceeded
	prioDistributedAsyncInsertionFailures

	prioUptime
)

var chCharts = collectorapi.Charts{
	chartConnections.Copy(),

	chartMemoryUsage.Copy(),

	chartSlowReads.Copy(),
	chartReadBackoff.Copy(),

	chartRunningQueries.Copy(),
	chartQueries.Copy(),
	chartSelectQueries.Copy(),
	chartInsertQueries.Copy(),
	chartQueriesPreempted.Copy(),
	chartQueriesMemoryLimitExceeded.Copy(),

	chartLongestRunningQueryTime.Copy(),
	chartQueriesLatency.Copy(),
	chartSelectQueriesLatency.Copy(),
	chartInsertQueriesLatency.Copy(),

	chartFileDescriptorIO.Copy(),
	chartFileDescriptorIOPS.Copy(),
	chartFileDescriptorIOErrors.Copy(),
	chartIOSeeks.Copy(),
	chartIOFileOpens.Copy(),

	chartReplicatedPartsActivity.Copy(),
	chartReplicasMaxAbsoluteDelay.Copy(),
	chartReadonlyReplica.Copy(),
	chartReplicatedDataLoss.Copy(),
	chartReplicatedPartFetches.Copy(),
	chartReplicatedPartMerges.Copy(),
	chartReplicatedPartFetchesOfMerged.Copy(),

	chartInsertedRows.Copy(),
	chartInsertedBytes.Copy(),
	chartRejectedInserts.Copy(),
	chartDelayedInserts.Copy(),
	chartDelayedInsertsThrottleTime.Copy(),

	chartSelectedRows.Copy(),
	chartSelectedBytes.Copy(),
	chartSelectedParts.Copy(),
	chartSelectedRanges.Copy(),
	chartSelectedMarks.Copy(),

	chartMerges.Copy(),
	chartMergesLatency.Copy(),
	chartMergedUncompressedBytes.Copy(),
	chartMergedRows.Copy(),

	chartMergeTreeDataWriterInsertedRows.Copy(),
	chartMergeTreeDataWriterUncompressedBytes.Copy(),
	chartMergeTreeDataWriterCompressedBytes.Copy(),

	chartUncompressedCacheRequests.Copy(),
	chartMarkCacheRequests.Copy(),

	chartMaxPartCountForPartition.Copy(),
	chartPartsCount.Copy(),

	chartDistributedConnections.Copy(),
	chartDistributedConnectionAttempts.Copy(),
	chartDistributedConnectionFailRetries.Copy(),
	chartDistributedConnectionFailExhaustedRetries.Copy(),

	chartDistributedFilesToInsert.Copy(),
	chartDistributedRejectedInserts.Copy(),
	chartDistributedDelayedInserts.Copy(),
	chartDistributedDelayedInsertsLatency.Copy(),
	chartDistributedSyncInsertionTimeoutExceeded.Copy(),
	chartDistributedAsyncInsertionFailures.Copy(),

	chartUptime.Copy(),
}

var (
	chartConnections = collectorapi.Chart{
		ID:       "connections",
		Title:    "Connections",
		Units:    "connections",
		Fam:      "conns",
		Ctx:      "clickhouse.connections",
		Priority: prioConnections,
		Dims: collectorapi.Dims{
			{ID: "metrics_TCPConnection", Name: "tcp"},
			{ID: "metrics_HTTPConnection", Name: "http"},
			{ID: "metrics_MySQLConnection", Name: "mysql"},
			{ID: "metrics_PostgreSQLConnection", Name: "postgresql"},
			{ID: "metrics_InterserverConnection", Name: "interserver"},
		},
	}
)

var (
	chartSlowReads = collectorapi.Chart{
		ID:       "slow_reads",
		Title:    "Slow reads from a file",
		Units:    "reads/s",
		Fam:      "slow reads",
		Ctx:      "clickhouse.slow_reads",
		Priority: prioSlowReads,
		Dims: collectorapi.Dims{
			{ID: "events_SlowRead", Name: "slow"},
		},
	}
	chartReadBackoff = collectorapi.Chart{
		ID:       "read_backoff",
		Title:    "Read backoff events",
		Units:    "events/s",
		Fam:      "slow reads",
		Ctx:      "clickhouse.read_backoff",
		Priority: prioReadBackoff,
		Dims: collectorapi.Dims{
			{ID: "events_ReadBackoff", Name: "read_backoff"},
		},
	}
)

var (
	chartMemoryUsage = collectorapi.Chart{
		ID:       "memory_usage",
		Title:    "Memory usage",
		Units:    "bytes",
		Fam:      "mem",
		Ctx:      "clickhouse.memory_usage",
		Priority: prioMemoryUsage,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "metrics_MemoryTracking", Name: "used"},
		},
	}
)

var diskChartsTmpl = collectorapi.Charts{
	diskSpaceUsageChartTmpl.Copy(),
}

var (
	diskSpaceUsageChartTmpl = collectorapi.Chart{
		ID:       "disk_%s_space_usage",
		Title:    "Disk space usage",
		Units:    "bytes",
		Fam:      "disk space",
		Ctx:      "clickhouse.disk_space_usage",
		Type:     collectorapi.Stacked,
		Priority: prioDiskSpaceUsage,
		Dims: collectorapi.Dims{
			{ID: "disk_%s_free_space_bytes", Name: "free"},
			{ID: "disk_%s_used_space_bytes", Name: "used"},
		},
	}
)

var (
	chartRunningQueries = collectorapi.Chart{
		ID:       "running_queries",
		Title:    "Running queries",
		Units:    "queries",
		Fam:      "queries",
		Ctx:      "clickhouse.running_queries",
		Priority: prioRunningQueries,
		Dims: collectorapi.Dims{
			{ID: "metrics_Query", Name: "running"},
		},
	}
	chartQueriesPreempted = collectorapi.Chart{
		ID:       "queries_preempted",
		Title:    "Queries waiting due to priority",
		Units:    "queries",
		Fam:      "queries",
		Ctx:      "clickhouse.queries_preempted",
		Priority: prioQueriesPreempted,
		Dims: collectorapi.Dims{
			{ID: "metrics_QueryPreempted", Name: "preempted"},
		},
	}
	chartQueries = collectorapi.Chart{
		ID:       "queries",
		Title:    "Queries",
		Units:    "queries/s",
		Fam:      "queries",
		Ctx:      "clickhouse.queries",
		Priority: prioQueries,
		Type:     collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "events_SuccessfulQuery", Name: "successful", Algo: collectorapi.Incremental},
			{ID: "events_FailedQuery", Name: "failed", Algo: collectorapi.Incremental},
		},
	}
	chartSelectQueries = collectorapi.Chart{
		ID:       "select_queries",
		Title:    "Select queries",
		Units:    "selects/s",
		Fam:      "queries",
		Ctx:      "clickhouse.select_queries",
		Priority: prioSelectQueries,
		Type:     collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "events_SuccessfulSelectQuery", Name: "successful", Algo: collectorapi.Incremental},
			{ID: "events_FailedSelectQuery", Name: "failed", Algo: collectorapi.Incremental},
		},
	}
	chartInsertQueries = collectorapi.Chart{
		ID:       "insert_queries",
		Title:    "Insert queries",
		Units:    "inserts/s",
		Fam:      "queries",
		Ctx:      "clickhouse.insert_queries",
		Priority: prioInsertQueries,
		Type:     collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "events_SuccessfulInsertQuery", Name: "successful", Algo: collectorapi.Incremental},
			{ID: "events_FailedInsertQuery", Name: "failed", Algo: collectorapi.Incremental},
		},
	}
	chartQueriesMemoryLimitExceeded = collectorapi.Chart{
		ID:       "queries_memory_limit_exceeded",
		Title:    "Memory limit exceeded for query",
		Units:    "queries/s",
		Fam:      "queries",
		Ctx:      "clickhouse.queries_memory_limit_exceeded",
		Priority: prioQueriesMemoryLimitExceeded,
		Dims: collectorapi.Dims{
			{ID: "events_QueryMemoryLimitExceeded", Name: "mem_limit_exceeded"},
		},
	}
)

var (
	chartLongestRunningQueryTime = collectorapi.Chart{
		ID:       "longest_running_query_time",
		Title:    "Longest running query time",
		Units:    "seconds",
		Fam:      "query latency",
		Ctx:      "clickhouse.longest_running_query_time",
		Priority: prioLongestRunningQueryTime,
		Dims: collectorapi.Dims{
			{ID: "LongestRunningQueryTime", Name: "longest_query_time", Div: precision},
		},
	}
	chartQueriesLatency = collectorapi.Chart{
		ID:       "queries_latency",
		Title:    "Queries latency",
		Units:    "microseconds",
		Fam:      "query latency",
		Ctx:      "clickhouse.queries_latency",
		Priority: prioQueriesLatency,
		Dims: collectorapi.Dims{
			{ID: "events_QueryTimeMicroseconds", Name: "queries_time", Algo: collectorapi.Incremental},
		},
	}
	chartSelectQueriesLatency = collectorapi.Chart{
		ID:       "select_queries_latency",
		Title:    "Select queries latency",
		Units:    "microseconds",
		Fam:      "query latency",
		Ctx:      "clickhouse.select_queries_latency",
		Priority: prioSelectQueriesLatency,
		Dims: collectorapi.Dims{
			{ID: "events_SelectQueryTimeMicroseconds", Name: "selects_time", Algo: collectorapi.Incremental},
		},
	}
	chartInsertQueriesLatency = collectorapi.Chart{
		ID:       "insert_queries_latency",
		Title:    "Insert queries latency",
		Units:    "microseconds",
		Fam:      "query latency",
		Ctx:      "clickhouse.insert_queries_latency",
		Priority: prioInsertQueriesLatency,
		Dims: collectorapi.Dims{
			{ID: "events_InsertQueryTimeMicroseconds", Name: "inserts_time", Algo: collectorapi.Incremental},
		},
	}
)

var (
	chartFileDescriptorIO = collectorapi.Chart{
		ID:       "file_descriptor_io",
		Title:    "Read and written data",
		Units:    "bytes/s",
		Fam:      "io",
		Ctx:      "clickhouse.io",
		Priority: prioIO,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "events_ReadBufferFromFileDescriptorReadBytes", Name: "reads", Algo: collectorapi.Incremental},
			{ID: "events_WriteBufferFromFileDescriptorWriteBytes", Name: "writes", Mul: -1, Algo: collectorapi.Incremental},
		},
	}
	chartFileDescriptorIOPS = collectorapi.Chart{
		ID:       "file_descriptor_iops",
		Title:    "Read and write operations",
		Units:    "ops/s",
		Fam:      "io",
		Ctx:      "clickhouse.iops",
		Priority: prioIOPS,
		Dims: collectorapi.Dims{
			{ID: "events_ReadBufferFromFileDescriptorRead", Name: "reads", Algo: collectorapi.Incremental},
			{ID: "events_WriteBufferFromFileDescriptorWrite", Name: "writes", Mul: -1, Algo: collectorapi.Incremental},
		},
	}
	chartFileDescriptorIOErrors = collectorapi.Chart{
		ID:       "file_descriptor_io_errors",
		Title:    "Read and write errors",
		Units:    "errors/s",
		Fam:      "io",
		Ctx:      "clickhouse.io_errors",
		Priority: prioIOErrors,
		Dims: collectorapi.Dims{
			{ID: "events_ReadBufferFromFileDescriptorReadFailed", Name: "read", Algo: collectorapi.Incremental},
			{ID: "events_WriteBufferFromFileDescriptorWriteFailed", Name: "write", Algo: collectorapi.Incremental},
		},
	}
	chartIOSeeks = collectorapi.Chart{
		ID:       "io_seeks",
		Title:    "lseek function calls",
		Units:    "ops/s",
		Fam:      "io",
		Ctx:      "clickhouse.io_seeks",
		Priority: prioIOSeeks,
		Dims: collectorapi.Dims{
			{ID: "events_Seek", Name: "lseek", Algo: collectorapi.Incremental},
		},
	}
	chartIOFileOpens = collectorapi.Chart{
		ID:       "io_file_opens",
		Title:    "File opens",
		Units:    "ops/s",
		Fam:      "io",
		Ctx:      "clickhouse.io_file_opens",
		Priority: prioIOFileOpens,
		Dims: collectorapi.Dims{
			{ID: "events_FileOpen", Name: "file_open", Algo: collectorapi.Incremental},
		},
	}
)

var tableChartsTmpl = collectorapi.Charts{
	tableSizeChartTmpl.Copy(),
	tablePartsChartTmpl.Copy(),
	tableRowsChartTmpl.Copy(),
}

var (
	tableSizeChartTmpl = collectorapi.Chart{
		ID:       "table_%s_database_%s_size",
		Title:    "Table size",
		Units:    "bytes",
		Fam:      "tables",
		Ctx:      "clickhouse.database_table_size",
		Type:     collectorapi.Area,
		Priority: prioDatabaseTableSize,
		Dims: collectorapi.Dims{
			{ID: "table_%s_database_%s_size_bytes", Name: "size"},
		},
	}
	tablePartsChartTmpl = collectorapi.Chart{
		ID:       "table_%s_database_%s_parts",
		Title:    "Table parts",
		Units:    "parts",
		Fam:      "tables",
		Ctx:      "clickhouse.database_table_parts",
		Priority: prioDatabaseTableParts,
		Dims: collectorapi.Dims{
			{ID: "table_%s_database_%s_parts", Name: "parts"},
		},
	}
	tableRowsChartTmpl = collectorapi.Chart{
		ID:       "table_%s_database_%s_rows",
		Title:    "Table rows",
		Units:    "rows",
		Fam:      "tables",
		Ctx:      "clickhouse.database_table_rows",
		Priority: prioDatabaseTableRows,
		Dims: collectorapi.Dims{
			{ID: "table_%s_database_%s_rows", Name: "rows"},
		},
	}
)

var (
	chartReplicatedPartsActivity = collectorapi.Chart{
		ID:       "replicated_parts_activity",
		Title:    "Replicated parts current activity",
		Units:    "parts",
		Fam:      "replicas",
		Ctx:      "clickhouse.replicated_parts_current_activity",
		Priority: prioReplicatedPartsCurrentActivity,
		Dims: collectorapi.Dims{
			{ID: "metrics_ReplicatedFetch", Name: "fetch"},
			{ID: "metrics_ReplicatedSend", Name: "send"},
			{ID: "metrics_ReplicatedChecks", Name: "check"},
		},
	}
	chartReplicasMaxAbsoluteDelay = collectorapi.Chart{
		ID:       "replicas_max_absolute_delay",
		Title:    "Replicas max absolute delay",
		Units:    "seconds",
		Fam:      "replicas",
		Ctx:      "clickhouse.replicas_max_absolute_delay",
		Priority: prioReplicasMaxAbsoluteDelay,
		Dims: collectorapi.Dims{
			{ID: "async_metrics_ReplicasMaxAbsoluteDelay", Name: "replication_delay", Div: precision},
		},
	}
	chartReadonlyReplica = collectorapi.Chart{
		ID:       "readonly_replica",
		Title:    "Replicated tables in readonly state",
		Units:    "tables",
		Fam:      "replicas",
		Ctx:      "clickhouse.replicated_readonly_tables",
		Priority: prioReadOnlyReplica,
		Dims: collectorapi.Dims{
			{ID: "metrics_ReadonlyReplica", Name: "read_only"},
		},
	}
	chartReplicatedDataLoss = collectorapi.Chart{
		ID:       "replicated_data_loss",
		Title:    "Replicated data loss",
		Units:    "events/s",
		Fam:      "replicas",
		Ctx:      "clickhouse.replicated_data_loss",
		Priority: prioReplicatedDataLoss,
		Dims: collectorapi.Dims{
			{ID: "events_ReplicatedDataLoss", Name: "data_loss", Algo: collectorapi.Incremental},
		},
	}
	chartReplicatedPartFetches = collectorapi.Chart{
		ID:       "replicated_part_fetches",
		Title:    "Replicated part fetches",
		Units:    "fetches/s",
		Fam:      "replicas",
		Ctx:      "clickhouse.replicated_part_fetches",
		Priority: prioReplicatedPartFetches,
		Dims: collectorapi.Dims{
			{ID: "events_ReplicatedPartFetches", Name: "successful", Algo: collectorapi.Incremental},
			{ID: "events_ReplicatedPartFailedFetches", Name: "failed", Algo: collectorapi.Incremental},
		},
	}
	chartReplicatedPartFetchesOfMerged = collectorapi.Chart{
		ID:       "replicated_part_fetches_of_merged",
		Title:    "Replicated part fetches of merged",
		Units:    "fetches/s",
		Fam:      "replicas",
		Ctx:      "clickhouse.replicated_part_fetches_of_merged",
		Priority: prioReplicatedPartFetchesOfMerged,
		Dims: collectorapi.Dims{
			{ID: "events_ReplicatedPartFetchesOfMerged", Name: "merged", Algo: collectorapi.Incremental},
		},
	}
	chartReplicatedPartMerges = collectorapi.Chart{
		ID:       "replicated_part_merges",
		Title:    "Replicated part merges",
		Units:    "merges/s",
		Fam:      "replicas",
		Ctx:      "clickhouse.replicated_part_merges",
		Priority: prioReplicatedPartMerges,
		Dims: collectorapi.Dims{
			{ID: "events_ReplicatedPartMerges", Name: "merges", Algo: collectorapi.Incremental},
		},
	}
)

var (
	chartInsertedBytes = collectorapi.Chart{
		ID:       "inserted_bytes",
		Title:    "Inserted data",
		Units:    "bytes/s",
		Fam:      "inserts",
		Ctx:      "clickhouse.inserted_bytes",
		Priority: prioInsertedBytes,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "events_InsertedBytes", Name: "inserted", Algo: collectorapi.Incremental},
		},
	}
	chartInsertedRows = collectorapi.Chart{
		ID:       "inserted_rows",
		Title:    "Inserted rows",
		Units:    "rows/s",
		Fam:      "inserts",
		Ctx:      "clickhouse.inserted_rows",
		Priority: prioInsertedRows,
		Dims: collectorapi.Dims{
			{ID: "events_InsertedRows", Name: "inserted", Algo: collectorapi.Incremental},
		},
	}
	chartRejectedInserts = collectorapi.Chart{
		ID:       "rejected_inserts",
		Title:    "Rejected inserts",
		Units:    "inserts/s",
		Fam:      "inserts",
		Ctx:      "clickhouse.rejected_inserts",
		Priority: prioRejectedInserts,
		Dims: collectorapi.Dims{
			{ID: "events_RejectedInserts", Name: "rejected", Algo: collectorapi.Incremental},
		},
	}
	chartDelayedInserts = collectorapi.Chart{
		ID:       "delayed_inserts",
		Title:    "Delayed inserts",
		Units:    "inserts/s",
		Fam:      "inserts",
		Ctx:      "clickhouse.delayed_inserts",
		Priority: prioDelayedInserts,
		Dims: collectorapi.Dims{
			{ID: "events_DelayedInserts", Name: "delayed", Algo: collectorapi.Incremental},
		},
	}
	chartDelayedInsertsThrottleTime = collectorapi.Chart{
		ID:       "delayed_inserts_throttle_time",
		Title:    "Delayed inserts throttle time",
		Units:    "milliseconds",
		Fam:      "inserts",
		Ctx:      "clickhouse.delayed_inserts_throttle_time",
		Priority: prioDelayedInsertsThrottleTime,
		Dims: collectorapi.Dims{
			{ID: "events_DelayedInsertsMilliseconds", Name: "delayed_inserts_throttle_time", Algo: collectorapi.Incremental},
		},
	}
)

var (
	chartSelectedBytes = collectorapi.Chart{
		ID:       "selected_bytes",
		Title:    "Selected data",
		Units:    "bytes/s",
		Fam:      "selects",
		Ctx:      "clickhouse.selected_bytes",
		Priority: prioSelectedBytes,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "events_SelectedBytes", Name: "selected", Algo: collectorapi.Incremental},
		},
	}
	chartSelectedRows = collectorapi.Chart{
		ID:       "selected_rows",
		Title:    "Selected rows",
		Units:    "rows/s",
		Fam:      "selects",
		Ctx:      "clickhouse.selected_rows",
		Priority: prioSelectedRows,
		Dims: collectorapi.Dims{
			{ID: "events_SelectedRows", Name: "selected", Algo: collectorapi.Incremental},
		},
	}
	chartSelectedParts = collectorapi.Chart{
		ID:       "selected_parts",
		Title:    "Selected parts",
		Units:    "parts/s",
		Fam:      "selects",
		Ctx:      "clickhouse.selected_parts",
		Priority: prioSelectedParts,
		Dims: collectorapi.Dims{
			{ID: "events_SelectedParts", Name: "selected", Algo: collectorapi.Incremental},
		},
	}
	chartSelectedRanges = collectorapi.Chart{
		ID:       "selected_ranges",
		Title:    "Selected ranges",
		Units:    "ranges/s",
		Fam:      "selects",
		Ctx:      "clickhouse.selected_ranges",
		Priority: prioSelectedRanges,
		Dims: collectorapi.Dims{
			{ID: "events_SelectedRanges", Name: "selected", Algo: collectorapi.Incremental},
		},
	}
	chartSelectedMarks = collectorapi.Chart{
		ID:       "selected_marks",
		Title:    "Selected marks",
		Units:    "marks/s",
		Fam:      "selects",
		Ctx:      "clickhouse.selected_marks",
		Priority: prioSelectedMarks,
		Dims: collectorapi.Dims{
			{ID: "events_SelectedMarks", Name: "selected", Algo: collectorapi.Incremental},
		},
	}
)

var (
	chartMerges = collectorapi.Chart{
		ID:       "merges",
		Title:    "Merge operations",
		Units:    "ops/s",
		Fam:      "merges",
		Ctx:      "clickhouse.merges",
		Priority: prioMerges,
		Dims: collectorapi.Dims{
			{ID: "events_Merge", Name: "merge", Algo: collectorapi.Incremental},
		},
	}
	chartMergesLatency = collectorapi.Chart{
		ID:       "merges_latency",
		Title:    "Time spent for background merges",
		Units:    "milliseconds",
		Fam:      "merges",
		Ctx:      "clickhouse.merges_latency",
		Priority: prioMergesLatency,
		Dims: collectorapi.Dims{
			{ID: "events_MergesTimeMilliseconds", Name: "merges_time", Algo: collectorapi.Incremental},
		},
	}
	chartMergedUncompressedBytes = collectorapi.Chart{
		ID:       "merged_uncompressed_bytes",
		Title:    "Uncompressed data read for background merges",
		Units:    "bytes/s",
		Fam:      "merges",
		Ctx:      "clickhouse.merged_uncompressed_bytes",
		Priority: prioMergedUncompressedBytes,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "events_MergedUncompressedBytes", Name: "merged_uncompressed", Algo: collectorapi.Incremental},
		},
	}
	chartMergedRows = collectorapi.Chart{
		ID:       "merged_rows",
		Title:    "Merged rows",
		Units:    "rows/s",
		Fam:      "merges",
		Ctx:      "clickhouse.merged_rows",
		Priority: prioMergedRows,
		Dims: collectorapi.Dims{
			{ID: "events_MergedRows", Name: "merged", Algo: collectorapi.Incremental},
		},
	}
)

var (
	chartMergeTreeDataWriterInsertedRows = collectorapi.Chart{
		ID:       "merge_tree_data_writer_inserted_rows",
		Title:    "Rows INSERTed to MergeTree tables",
		Units:    "rows/s",
		Fam:      "merge tree",
		Ctx:      "clickhouse.merge_tree_data_writer_inserted_rows",
		Priority: prioMergeTreeDataWriterRows,
		Dims: collectorapi.Dims{
			{ID: "events_MergeTreeDataWriterRows", Name: "inserted", Algo: collectorapi.Incremental},
		},
	}
	chartMergeTreeDataWriterUncompressedBytes = collectorapi.Chart{
		ID:       "merge_tree_data_writer_uncompressed_bytes",
		Title:    "Data INSERTed to MergeTree tables",
		Units:    "bytes/s",
		Fam:      "merge tree",
		Ctx:      "clickhouse.merge_tree_data_writer_uncompressed_bytes",
		Type:     collectorapi.Area,
		Priority: prioMergeTreeDataWriterUncompressedBytes,
		Dims: collectorapi.Dims{
			{ID: "events_MergeTreeDataWriterUncompressedBytes", Name: "inserted", Algo: collectorapi.Incremental},
		},
	}
	chartMergeTreeDataWriterCompressedBytes = collectorapi.Chart{
		ID:       "merge_tree_data_writer_compressed_bytes",
		Title:    "Data written to disk for data INSERTed to MergeTree tables",
		Units:    "bytes/s",
		Fam:      "merge tree",
		Ctx:      "clickhouse.merge_tree_data_writer_compressed_bytes",
		Type:     collectorapi.Area,
		Priority: prioMergeTreeDataWriterCompressedBytes,
		Dims: collectorapi.Dims{
			{ID: "events_MergeTreeDataWriterCompressedBytes", Name: "written", Algo: collectorapi.Incremental},
		},
	}
)

var (
	chartUncompressedCacheRequests = collectorapi.Chart{
		ID:       "uncompressed_cache_requests",
		Title:    "Uncompressed cache requests",
		Units:    "requests/s",
		Fam:      "cache",
		Ctx:      "clickhouse.uncompressed_cache_requests",
		Priority: prioUncompressedCacheRequests,
		Dims: collectorapi.Dims{
			{ID: "events_UncompressedCacheHits", Name: "hits", Algo: collectorapi.Incremental},
			{ID: "events_UncompressedCacheMisses", Name: "misses", Algo: collectorapi.Incremental},
		},
	}
	chartMarkCacheRequests = collectorapi.Chart{
		ID:       "mark_cache_requests",
		Title:    "Mark cache requests",
		Units:    "requests/s",
		Fam:      "cache",
		Ctx:      "clickhouse.mark_cache_requests",
		Priority: prioMarkCacheRequests,
		Dims: collectorapi.Dims{
			{ID: "events_MarkCacheHits", Name: "hits", Algo: collectorapi.Incremental},
			{ID: "events_MarkCacheMisses", Name: "misses", Algo: collectorapi.Incremental},
		},
	}
)

var (
	chartMaxPartCountForPartition = collectorapi.Chart{
		ID:       "max_part_count_for_partition",
		Title:    "Max part count for partition",
		Units:    "parts",
		Fam:      "parts",
		Ctx:      "clickhouse.max_part_count_for_partition",
		Priority: prioMaxPartCountForPartition,
		Dims: collectorapi.Dims{
			{ID: "async_metrics_MaxPartCountForPartition", Name: "max_parts_partition"},
		},
	}
	chartPartsCount = collectorapi.Chart{
		ID:       "parts_count",
		Title:    "Parts",
		Units:    "parts",
		Fam:      "parts",
		Ctx:      "clickhouse.parts_count",
		Priority: prioParts,
		Dims: collectorapi.Dims{
			{ID: "metrics_PartsTemporary", Name: "temporary"},
			{ID: "metrics_PartsPreActive", Name: "pre_active"},
			{ID: "metrics_PartsActive", Name: "active"},
			{ID: "metrics_PartsDeleting", Name: "deleting"},
			{ID: "metrics_PartsDeleteOnDestroy", Name: "delete_on_destroy"},
			{ID: "metrics_PartsOutdated", Name: "outdated"},
			{ID: "metrics_PartsWide", Name: "wide"},
			{ID: "metrics_PartsCompact", Name: "compact"},
		},
	}
)

var (
	chartDistributedConnections = collectorapi.Chart{
		ID:       "distributes_connections",
		Title:    "Active distributed connection",
		Units:    "connections",
		Fam:      "distributed conns",
		Ctx:      "clickhouse.distributed_connections",
		Priority: prioDistributedSend,
		Dims: collectorapi.Dims{
			{ID: "metrics_DistributedSend", Name: "active"},
		},
	}
	chartDistributedConnectionAttempts = collectorapi.Chart{
		ID:       "distributes_connections_attempts",
		Title:    "Distributed connection attempts",
		Units:    "attempts/s",
		Fam:      "distributed conns",
		Ctx:      "clickhouse.distributed_connections_attempts",
		Priority: prioDistributedConnectionTries,
		Dims: collectorapi.Dims{
			{ID: "events_DistributedConnectionTries", Name: "connection", Algo: collectorapi.Incremental},
		},
	}
	chartDistributedConnectionFailRetries = collectorapi.Chart{
		ID:       "distributes_connections_fail_retries",
		Title:    "Distributed connection fails with retry",
		Units:    "fails/s",
		Fam:      "distributed conns",
		Ctx:      "clickhouse.distributed_connections_fail_retries",
		Priority: prioDistributedConnectionFailTry,
		Dims: collectorapi.Dims{
			{ID: "events_DistributedConnectionFailTry", Name: "connection_retry", Algo: collectorapi.Incremental},
		},
	}
	chartDistributedConnectionFailExhaustedRetries = collectorapi.Chart{
		ID:       "distributes_connections_fail_exhausted_retries",
		Title:    "Distributed connection fails after all retries finished",
		Units:    "fails/s",
		Fam:      "distributed conns",
		Ctx:      "clickhouse.distributed_connections_fail_exhausted_retries",
		Priority: prioDistributedConnectionFailAtAll,
		Dims: collectorapi.Dims{
			{ID: "events_DistributedConnectionFailAtAll", Name: "connection_retry_exhausted", Algo: collectorapi.Incremental},
		},
	}
)

var (
	chartDistributedFilesToInsert = collectorapi.Chart{
		ID:       "distributes_files_to_insert",
		Title:    "Pending files to process for asynchronous insertion into Distributed tables",
		Units:    "files",
		Fam:      "distributed inserts",
		Ctx:      "clickhouse.distributed_files_to_insert",
		Priority: prioDistributedFilesToInsert,
		Dims: collectorapi.Dims{
			{ID: "metrics_DistributedFilesToInsert", Name: "pending_insertions"},
		},
	}
	chartDistributedRejectedInserts = collectorapi.Chart{
		ID:       "distributes_rejected_inserts",
		Title:    "Rejected INSERTs to a Distributed table",
		Units:    "inserts/s",
		Fam:      "distributed inserts",
		Ctx:      "clickhouse.distributed_rejected_inserts",
		Priority: prioDistributedRejectedInserts,
		Dims: collectorapi.Dims{
			{ID: "events_DistributedRejectedInserts", Name: "rejected", Algo: collectorapi.Incremental},
		},
	}
	chartDistributedDelayedInserts = collectorapi.Chart{
		ID:       "distributes_delayed_inserts",
		Title:    "Delayed INSERTs to a Distributed table",
		Units:    "inserts/s",
		Fam:      "distributed inserts",
		Ctx:      "clickhouse.distributed_delayed_inserts",
		Priority: prioDistributedDelayedInserts,
		Dims: collectorapi.Dims{
			{ID: "events_DistributedDelayedInserts", Name: "delayed", Algo: collectorapi.Incremental},
		},
	}
	chartDistributedDelayedInsertsLatency = collectorapi.Chart{
		ID:       "distributes_delayed_inserts_latency",
		Title:    "Time spent while the INSERT of a block to a Distributed table was throttled",
		Units:    "milliseconds",
		Fam:      "distributed inserts",
		Ctx:      "clickhouse.distributed_delayed_inserts_latency",
		Priority: prioDistributedDelayedInsertsMilliseconds,
		Dims: collectorapi.Dims{
			{ID: "events_DistributedDelayedInsertsMilliseconds", Name: "delayed_time", Algo: collectorapi.Incremental},
		},
	}
	chartDistributedSyncInsertionTimeoutExceeded = collectorapi.Chart{
		ID:       "distributes_sync_insertion_timeout_exceeded",
		Title:    "Distributed table sync insertions timeouts",
		Units:    "timeouts/s",
		Fam:      "distributed inserts",
		Ctx:      "clickhouse.distributed_sync_insertion_timeout_exceeded",
		Priority: prioDistributedSyncInsertionTimeoutExceeded,
		Dims: collectorapi.Dims{
			{ID: "events_DistributedSyncInsertionTimeoutExceeded", Name: "sync_insertion", Algo: collectorapi.Incremental},
		},
	}
	chartDistributedAsyncInsertionFailures = collectorapi.Chart{
		ID:       "distributes_async_insertions_failures",
		Title:    "Distributed table async insertion failures",
		Units:    "failures/s",
		Fam:      "distributed inserts",
		Ctx:      "clickhouse.distributed_async_insertions_failures",
		Priority: prioDistributedAsyncInsertionFailures,
		Dims: collectorapi.Dims{
			{ID: "events_DistributedAsyncInsertionFailures", Name: "async_insertions", Algo: collectorapi.Incremental},
		},
	}
)

var (
	chartUptime = collectorapi.Chart{
		ID:       "uptime",
		Title:    "Uptime",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "clickhouse.uptime",
		Priority: prioUptime,
		Dims: collectorapi.Dims{
			{ID: "async_metrics_Uptime", Name: "uptime"},
		},
	}
)

func (c *Collector) addDiskCharts(disk *seenDisk) {
	charts := diskChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, disk.disk)
		chart.Labels = []collectorapi.Label{
			{Key: "disk_name", Value: disk.disk},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, disk.disk)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeDiskCharts(disk *seenDisk) {
	px := fmt.Sprintf("disk_%s_", disk.disk)
	c.removeCharts(px)
}

func (c *Collector) addTableCharts(table *seenTable) {
	charts := tableChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, table.table, table.db)
		chart.Labels = []collectorapi.Label{
			{Key: "database", Value: table.db},
			{Key: "table", Value: table.table},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, table.table, table.db)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeTableCharts(table *seenTable) {
	px := fmt.Sprintf("table_%s_database_%s_", table.table, table.db)
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
