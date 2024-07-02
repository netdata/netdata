// SPDX-License-Identifier: GPL-3.0-or-later

package cockroachdb

// Architecture Overview
// https://www.cockroachlabs.com/docs/stable/architecture/overview.html

// Web Dashboards
// https://github.com/cockroachdb/cockroach/tree/master/pkg/ui/src/views/cluster/containers/nodeGraphs/dashboards

// Process
const (
	// https://github.com/cockroachdb/cockroach/blob/master/pkg/server/status/runtime.go
	metricSysCPUUserNs                    = "sys_cpu_user_ns"
	metricSysCPUSysNs                     = "sys_cpu_sys_ns"
	metricSysCPUUserPercent               = "sys_cpu_user_percent"
	metricSysCPUSysPercent                = "sys_cpu_sys_percent"
	metricSysCPUCombinedPercentNormalized = "sys_cpu_combined_percent_normalized"
	metricSysRSS                          = "sys_rss"
	metricSysFDOpen                       = "sys_fd_open"
	metricSysFDSoftLimit                  = "sys_fd_softlimit"
	metricSysUptime                       = "sys_uptime"
)

// Host Disk/Network Cumulative
const (
	// https://github.com/cockroachdb/cockroach/blob/master/pkg/server/status/runtime.go
	metricSysHostDiskReadBytes      = "sys_host_disk_read_bytes"
	metricSysHostDiskWriteBytes     = "sys_host_disk_write_bytes"
	metricSysHostDiskReadCount      = "sys_host_disk_read_count"
	metricSysHostDiskWriteCount     = "sys_host_disk_write_count"
	metricSysHostDiskIOPSInProgress = "sys_host_disk_iopsinprogress"
	metricSysHostNetSendBytes       = "sys_host_net_send_bytes"
	metricSysHostNetRecvBytes       = "sys_host_net_recv_bytes"
	metricSysHostNetSendPackets     = "sys_host_net_send_packets"
	metricSysHostNetRecvPackets     = "sys_host_net_recv_packets"
)

// Liveness
const (
	//https://github.com/cockroachdb/cockroach/blob/master/pkg/storage/node_liveness.go
	metricLiveNodes          = "liveness_livenodes"
	metricHeartBeatSuccesses = "liveness_heartbeatsuccesses"
	metricHeartBeatFailures  = "liveness_heartbeatfailures"
)

// Capacity
const (
	// https://github.com/cockroachdb/cockroach/blob/master/pkg/storage/metrics.go
	metricCapacity          = "capacity"
	metricCapacityAvailable = "capacity_available"
	metricCapacityUsed      = "capacity_used"
	//metricCapacityReserved  = "capacity_reserved"
)

// SQL
const (
	// https://github.com/cockroachdb/cockroach/blob/master/pkg/sql/pgwire/server.go
	metricSQLConnections = "sql_conns"
	metricSQLBytesIn     = "sql_bytesin"
	metricSQLBytesOut    = "sql_bytesout"

	// https://github.com/cockroachdb/cockroach/blob/master/pkg/sql/exec_util.go
	// Started Statements
	metricSQLQueryStartedCount                    = "sql_query_started_count" // Cumulative (Statements + Transaction Statements)
	metricSQLSelectStartedCount                   = "sql_select_started_count"
	metricSQLUpdateStartedCount                   = "sql_update_started_count"
	metricSQLInsertStartedCount                   = "sql_insert_started_count"
	metricSQLDeleteStartedCount                   = "sql_delete_started_count"
	metricSQLSavepointStartedCount                = "sql_savepoint_started_count"
	metricSQLRestartSavepointStartedCount         = "sql_restart_savepoint_started_count"
	metricSQLRestartSavepointReleaseStartedCount  = "sql_restart_savepoint_release_started_count"
	metricSQLRestartSavepointRollbackStartedCount = "sql_restart_savepoint_rollback_started_count"
	metricSQLDDLStartedCount                      = "sql_ddl_started_count"
	metricSQLMiscStartedCount                     = "sql_misc_started_count"
	// Started Transaction Statements
	metricSQLTXNBeginStartedCount    = "sql_txn_begin_started_count"
	metricSQLTXNCommitStartedCount   = "sql_txn_commit_started_count"
	metricSQLTXNRollbackStartedCount = "sql_txn_rollback_started_count"

	// Executed Statements
	metricSQLQueryCount                    = "sql_query_count" // Cumulative (Statements + Transaction Statements)
	metricSQLSelectCount                   = "sql_select_count"
	metricSQLUpdateCount                   = "sql_update_count"
	metricSQLInsertCount                   = "sql_insert_count"
	metricSQLDeleteCount                   = "sql_delete_count"
	metricSQLSavepointCount                = "sql_savepoint_count"
	metricSQLRestartSavepointCount         = "sql_restart_savepoint_count"
	metricSQLRestartSavepointReleaseCount  = "sql_restart_savepoint_release_count"
	metricSQLRestartSavepointRollbackCount = "sql_restart_savepoint_rollback_count"
	metricSQLDDLCount                      = "sql_ddl_count"
	metricSQLMiscCount                     = "sql_misc_count"
	// Executed Transaction statements
	metricSQLTXNBeginCount    = "sql_txn_begin_count"
	metricSQLTXNCommitCount   = "sql_txn_commit_count"
	metricSQLTXNRollbackCount = "sql_txn_rollback_count"

	// Statements Resulted In An Error
	metricSQLFailureCount = "sql_failure_count"
	// Transaction Resulted In Abort Errors
	metricSQLTXNAbortCount = "sql_txn_abort_count"

	// Distributed SQL
	// https://github.com/cockroachdb/cockroach/blob/master/pkg/sql/execinfra/metrics.go
	metricSQLDistSQLQueriesActive = "sql_distsql_queries_active"
	metricSQLDistSQLFlowsActive   = "sql_distsql_flows_active"
	metricSQLDistSQLFlowsQueued   = "sql_distsql_flows_queued"
)

// Storage
const (
	// https://github.com/cockroachdb/cockroach/blob/master/pkg/storage/metrics.go
	metricLiveBytes = "livebytes"
	metricSysBytes  = "sysbytes"
	metricKeyBytes  = "keybytes"
	metricValBytes  = "valbytes"
	metricKeyCount  = "keycount"
	metricValCount  = "valcount"
)

// KV Transactions
const (
	// https://github.com/cockroachdb/cockroach/blob/master/pkg/kv/txn_metrics.go
	metricTxnCommits                       = "txn_commits"
	metricTxnCommits1PC                    = "txn_commits1PC"
	metricTxnAborts                        = "txn_aborts"
	metricTxnRestartsWriteTooOld           = "txn_restarts_writetooold"
	metricTxnRestartsWriteTooOldMulti      = "txn_restarts_writetoooldmulti"
	metricTxnRestartsSerializable          = "txn_restarts_serializable"
	metricTxnRestartsPossibleReplay        = "txn_restarts_possiblereplay"
	metricTxnRestartsAsyncWriteFailure     = "txn_restarts_asyncwritefailure"
	metricTxnRestartsReadWithInUncertainty = "txn_restarts_readwithinuncertainty"
	metricTxnRestartsTxnAborted            = "txn_restarts_txnaborted"
	metricTxnRestartsTxnPush               = "txn_restarts_txnpush"
	metricTxnRestartsUnknown               = "txn_restarts_unknown"
)

// Ranges
const (
	// https://github.com/cockroachdb/cockroach/blob/master/pkg/storage/metrics.go
	metricRanges                = "ranges"
	metricRangesUnavailable     = "ranges_unavailable"
	metricRangesUnderReplicated = "ranges_underreplicated"
	metricRangesOverReplicated  = "ranges_overreplicated"
	// Range Events Metrics
	// https://github.com/cockroachdb/cockroach/blob/master/pkg/storage/metrics.go
	metricRangeSplits                     = "range_splits"
	metricRangeAdds                       = "range_adds"
	metricRangeRemoves                    = "range_removes"
	metricRangeMerges                     = "range_merges"
	metricRangeSnapshotsGenerated         = "range_snapshots_generated"
	metricRangeSnapshotsPreemptiveApplied = "range_snapshots_preemptive_applied"
	metricRangeSnapshotsLearnerApplied    = "range_snapshots_learner_applied"
	metricRangeSnapshotsNormalApplied     = "range_snapshots_normal_applied"
)

// RocksDB
const (
	// https://github.com/cockroachdb/cockroach/blob/master/pkg/storage/metrics.go
	metricRocksDBReadAmplification = "rocksdb_read_amplification"
	metricRocksDBNumSSTables       = "rocksdb_num_sstables"
	metricRocksDBBlockCacheUsage   = "rocksdb_block_cache_usage"
	metricRocksDBBlockCacheHits    = "rocksdb_block_cache_hits"
	metricRocksDBBlockCacheMisses  = "rocksdb_block_cache_misses"
	metricRocksDBCompactions       = "rocksdb_compactions"
	metricRocksDBFlushes           = "rocksdb_flushes"
)

// Replication
const (
	// https://github.com/cockroachdb/cockroach/blob/master/pkg/storage/metrics.go
	metricReplicas = "replicas"
	// metricReplicasReserved               = "replicas_reserved"
	metricReplicasLeaders                = "replicas_leaders"
	metricReplicasLeadersNotLeaseholders = "replicas_leaders_not_leaseholders"
	metricReplicasLeaseholders           = "replicas_leaseholders"
	metricReplicasQuiescent              = "replicas_quiescent"
)

// Queues
const (
	// https://github.com/cockroachdb/cockroach/blob/master/pkg/storage/metrics.go
	metricQueueGCProcessFailure            = "queue_gc_process_failure"
	metricQueueReplicaGCProcessFailure     = "queue_replicagc_process_failure"
	metricQueueReplicateProcessFailure     = "queue_replicate_process_failure"
	metricQueueSplitProcessFailure         = "queue_split_process_failure"
	metricQueueConsistencyProcessFailure   = "queue_consistency_process_failure"
	metricQueueRaftLogProcessFailure       = "queue_raftlog_process_failure"
	metricQueueRaftSnapshotProcessFailure  = "queue_raftsnapshot_process_failure"
	metricQueueTSMaintenanceProcessFailure = "queue_tsmaintenance_process_failure"
)

// Rebalancing
const (
	// https://github.com/cockroachdb/cockroach/blob/master/pkg/storage/metrics.go
	metricRebalancingQueriesPerSecond = "rebalancing_queriespersecond"
	metricRebalancingWritesPerSecond  = "rebalancing_writespersecond"
)

// Slow Requests
const (
	// https://github.com/cockroachdb/cockroach/blob/master/pkg/storage/metrics.go
	metricRequestsSlowLease = "requests_slow_lease"
	metricRequestsSlowLatch = "requests_slow_latch"
	metricRequestsSlowRaft  = "requests_slow_raft"
)

// Time Series
const (
	// https://github.com/cockroachdb/cockroach/blob/master/pkg/ts/metrics.go
	metricTimeSeriesWriteSamples = "timeseries_write_samples"
	metricTimeSeriesWriteErrors  = "timeseries_write_errors"
	metricTimeSeriesWriteBytes   = "timeseries_write_bytes"
)

// Go/Cgo
const (
	// https://github.com/cockroachdb/cockroach/blob/master/pkg/server/status/runtime.go
	metricSysGoAllocBytes  = "sys_go_allocbytes"
	metricSysCGoAllocBytes = "sys_cgo_allocbytes"
	metricSysCGoCalls      = "sys_cgocalls"
	metricSysGoroutines    = "sys_goroutines"
	metricSysGCCount       = "sys_gc_count"
	metricSysGCPauseNs     = "sys_gc_pause_ns"
)

const (
	// Calculated Metrics
	metricCapacityUsable               = "capacity_usable"
	metricCapacityUnusable             = "capacity_unusable"
	metricCapacityUsedPercentage       = "capacity_used_percent"
	metricCapacityUsableUsedPercentage = "capacity_usable_used_percent"
	metricRocksDBBlockCacheHitRate     = "rocksdb_block_cache_hit_rate"
	metricReplicasActive               = "replicas_active"
)

var metrics = []string{
	metricSysCPUUserNs,
	metricSysCPUSysNs,
	metricSysCPUUserPercent,
	metricSysCPUSysPercent,
	metricSysCPUCombinedPercentNormalized,
	metricSysRSS,
	metricSysFDOpen,
	metricSysFDSoftLimit,
	metricSysUptime,

	metricSysHostDiskReadBytes,
	metricSysHostDiskWriteBytes,
	metricSysHostDiskReadCount,
	metricSysHostDiskWriteCount,
	metricSysHostDiskIOPSInProgress,
	metricSysHostNetSendBytes,
	metricSysHostNetRecvBytes,
	metricSysHostNetSendPackets,
	metricSysHostNetRecvPackets,

	metricLiveNodes,
	metricHeartBeatSuccesses,
	metricHeartBeatFailures,

	metricCapacity,
	metricCapacityAvailable,
	metricCapacityUsed,

	metricSQLConnections,
	metricSQLBytesIn,
	metricSQLBytesOut,
	metricSQLQueryStartedCount,
	metricSQLSelectStartedCount,
	metricSQLUpdateStartedCount,
	metricSQLInsertStartedCount,
	metricSQLDeleteStartedCount,
	metricSQLSavepointStartedCount,
	metricSQLRestartSavepointStartedCount,
	metricSQLRestartSavepointReleaseStartedCount,
	metricSQLRestartSavepointRollbackStartedCount,
	metricSQLDDLStartedCount,
	metricSQLMiscStartedCount,
	metricSQLTXNBeginStartedCount,
	metricSQLTXNCommitStartedCount,
	metricSQLTXNRollbackStartedCount,
	metricSQLQueryCount,
	metricSQLSelectCount,
	metricSQLUpdateCount,
	metricSQLInsertCount,
	metricSQLDeleteCount,
	metricSQLSavepointCount,
	metricSQLRestartSavepointCount,
	metricSQLRestartSavepointReleaseCount,
	metricSQLRestartSavepointRollbackCount,
	metricSQLDDLCount,
	metricSQLMiscCount,
	metricSQLTXNBeginCount,
	metricSQLTXNCommitCount,
	metricSQLTXNRollbackCount,
	metricSQLFailureCount,
	metricSQLTXNAbortCount,
	metricSQLDistSQLQueriesActive,
	metricSQLDistSQLFlowsActive,
	metricSQLDistSQLFlowsQueued,

	metricLiveBytes,
	metricSysBytes,
	metricKeyBytes,
	metricValBytes,
	metricKeyCount,
	metricValCount,

	metricTxnCommits,
	metricTxnCommits1PC,
	metricTxnAborts,
	metricTxnRestartsWriteTooOld,
	metricTxnRestartsWriteTooOldMulti,
	metricTxnRestartsSerializable,
	metricTxnRestartsPossibleReplay,
	metricTxnRestartsAsyncWriteFailure,
	metricTxnRestartsReadWithInUncertainty,
	metricTxnRestartsTxnAborted,
	metricTxnRestartsTxnPush,
	metricTxnRestartsUnknown,

	metricRanges,
	metricRangesUnavailable,
	metricRangesUnderReplicated,
	metricRangesOverReplicated,
	metricRangeSplits,
	metricRangeAdds,
	metricRangeRemoves,
	metricRangeMerges,
	metricRangeSnapshotsGenerated,
	metricRangeSnapshotsPreemptiveApplied,
	metricRangeSnapshotsLearnerApplied,
	metricRangeSnapshotsNormalApplied,

	metricRocksDBReadAmplification,
	metricRocksDBNumSSTables,
	metricRocksDBBlockCacheUsage,
	metricRocksDBBlockCacheHits,
	metricRocksDBBlockCacheMisses,
	metricRocksDBCompactions,
	metricRocksDBFlushes,

	metricReplicas,
	metricReplicasLeaders,
	metricReplicasLeadersNotLeaseholders,
	metricReplicasLeaseholders,
	metricReplicasQuiescent,

	metricQueueGCProcessFailure,
	metricQueueReplicaGCProcessFailure,
	metricQueueReplicateProcessFailure,
	metricQueueSplitProcessFailure,
	metricQueueConsistencyProcessFailure,
	metricQueueRaftLogProcessFailure,
	metricQueueRaftSnapshotProcessFailure,
	metricQueueTSMaintenanceProcessFailure,

	metricRebalancingQueriesPerSecond,
	metricRebalancingWritesPerSecond,

	metricTimeSeriesWriteSamples,
	metricTimeSeriesWriteErrors,
	metricTimeSeriesWriteBytes,

	metricRequestsSlowLease,
	metricRequestsSlowLatch,
	metricRequestsSlowRaft,

	metricSysGoAllocBytes,
	metricSysCGoAllocBytes,
	metricSysCGoCalls,
	metricSysGoroutines,
	metricSysGCCount,
	metricSysGCPauseNs,
}
