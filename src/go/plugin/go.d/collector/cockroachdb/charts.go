// SPDX-License-Identifier: GPL-3.0-or-later

package cockroachdb

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

type (
	Charts = collectorapi.Charts
	Chart  = collectorapi.Chart
	Dims   = collectorapi.Dims
	Vars   = collectorapi.Vars
)

var charts = Charts{
	chartProcessCPUCombinedPercent.Copy(),
	chartProcessCPUPercent.Copy(),
	chartProcessCPUUsage.Copy(),
	chartProcessMemory.Copy(),
	chartProcessFDUsage.Copy(),
	chartProcessUptime.Copy(),

	chartHostDiskBandwidth.Copy(),
	chartHostDiskOperations.Copy(),
	chartHostDiskIOPS.Copy(),
	chartHostNetworkBandwidth.Copy(),
	chartHostNetworkPackets.Copy(),

	chartLiveNodes.Copy(),
	chartHeartBeats.Copy(),

	chartCapacity.Copy(),
	chartCapacityUsability.Copy(),
	chartCapacityUsable.Copy(),
	chartCapacityUsedPercentage.Copy(),

	chartSQLConnections.Copy(),
	chartSQLTraffic.Copy(),
	chartSQLStatementsTotal.Copy(),
	chartSQLErrors.Copy(),
	chartSQLStartedDDLStatements.Copy(),
	chartSQLExecutedDDLStatements.Copy(),
	chartSQLStartedDMLStatements.Copy(),
	chartSQLExecutedDMLStatements.Copy(),
	chartSQLStartedTCLStatements.Copy(),
	chartSQLExecutedTCLStatements.Copy(),
	chartSQLActiveDistQueries.Copy(),
	chartSQLActiveFlowsForDistQueries.Copy(),

	chartUsedLiveData.Copy(),
	chartLogicalData.Copy(),
	chartLogicalDataCount.Copy(),

	chartKVTransactions.Copy(),
	chartKVTransactionsRestarts.Copy(),

	chartRanges.Copy(),
	chartRangesWithProblems.Copy(),
	chartRangesEvents.Copy(),
	chartRangesSnapshotEvents.Copy(),

	chartRocksDBReadAmplification.Copy(),
	chartRocksDBTableOperations.Copy(),
	chartRocksDBCacheUsage.Copy(),
	chartRocksDBCacheOperations.Copy(),
	chartRocksDBCacheHitRage.Copy(),
	chartRocksDBSSTables.Copy(),

	chartReplicas.Copy(),
	chartReplicasQuiescence.Copy(),
	chartReplicasLeaders.Copy(),
	chartReplicasLeaseHolder.Copy(),

	chartQueuesProcessingFailures.Copy(),

	chartRebalancingQueries.Copy(),
	chartRebalancingWrites.Copy(),

	chartTimeSeriesWrittenSamples.Copy(),
	chartTimeSeriesWriteErrors.Copy(),
	chartTimeSeriesWrittenBytes.Copy(),

	chartSlowRequests.Copy(),

	chartGoroutines.Copy(),
	chartGoCgoHeapMemory.Copy(),
	chartCGoCalls.Copy(),
	chartGCRuns.Copy(),
	chartGCPauseTime.Copy(),
}

// Process
var (
	chartProcessCPUCombinedPercent = Chart{
		ID:    "process_cpu_time_combined_percentage",
		Title: "Combined CPU Time Percentage, Normalized 0-1 by Number of Cores",
		Units: "percentage",
		Fam:   "process",
		Ctx:   "cockroachdb.process_cpu_time_combined_percentage",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: metricSysCPUCombinedPercentNormalized, Name: "used", Div: precision},
		},
	}
	chartProcessCPUPercent = Chart{
		ID:    "process_cpu_time_percentage",
		Title: "CPU Time Percentage",
		Units: "percentage",
		Fam:   "process",
		Ctx:   "cockroachdb.process_cpu_time_percentage",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: metricSysCPUUserPercent, Name: "user", Div: precision},
			{ID: metricSysCPUSysPercent, Name: "sys", Div: precision},
		},
	}
	chartProcessCPUUsage = Chart{
		ID:    "process_cpu_time",
		Title: "CPU Time",
		Units: "ms",
		Fam:   "process",
		Ctx:   "cockroachdb.process_cpu_time",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: metricSysCPUUserNs, Name: "user", Algo: collectorapi.Incremental, Div: 1e6},
			{ID: metricSysCPUSysNs, Name: "sys", Algo: collectorapi.Incremental, Div: 1e6},
		},
	}
	chartProcessMemory = Chart{
		ID:    "process_memory",
		Title: "Memory Usage",
		Units: "KiB",
		Fam:   "process",
		Ctx:   "cockroachdb.process_memory",
		Dims: Dims{
			{ID: metricSysRSS, Name: "rss", Div: 1024},
		},
	}
	chartProcessFDUsage = Chart{
		ID:    "process_file_descriptors",
		Title: "File Descriptors",
		Units: "fd",
		Fam:   "process",
		Ctx:   "cockroachdb.process_file_descriptors",
		Dims: Dims{
			{ID: metricSysFDOpen, Name: "open"},
		},
		Vars: Vars{
			{ID: metricSysFDSoftLimit},
		},
	}
	chartProcessUptime = Chart{
		ID:    "process_uptime",
		Title: "Uptime",
		Units: "seconds",
		Fam:   "process",
		Ctx:   "cockroachdb.process_uptime",
		Dims: Dims{
			{ID: metricSysUptime, Name: "uptime"},
		},
	}
)

// Host
// Host
var (
	chartHostDiskBandwidth = Chart{
		ID:    "host_disk_bandwidth",
		Title: "Host Disk Cumulative Bandwidth",
		Units: "KiB",
		Fam:   "host",
		Ctx:   "cockroachdb.host_disk_bandwidth",
		Type:  collectorapi.Area,
		Dims: Dims{
			{ID: metricSysHostDiskReadBytes, Name: "read", Div: 1024, Algo: collectorapi.Incremental},
			{ID: metricSysHostDiskWriteBytes, Name: "write", Div: -1024, Algo: collectorapi.Incremental},
		},
	}
	chartHostDiskOperations = Chart{
		ID:    "host_disk_operations",
		Title: "Host Disk Cumulative Operations",
		Units: "operations",
		Fam:   "host",
		Ctx:   "cockroachdb.host_disk_operations",
		Dims: Dims{
			{ID: metricSysHostDiskReadCount, Name: "reads", Algo: collectorapi.Incremental},
			{ID: metricSysHostDiskWriteCount, Name: "writes", Mul: -1, Algo: collectorapi.Incremental},
		},
	}
	chartHostDiskIOPS = Chart{
		ID:    "host_disk_iops_in_progress",
		Title: "Host Disk Cumulative IOPS In Progress",
		Units: "iops",
		Fam:   "host",
		Ctx:   "cockroachdb.host_disk_iops_in_progress",
		Dims: Dims{
			{ID: metricSysHostDiskIOPSInProgress, Name: "in progress"},
		},
	}
	chartHostNetworkBandwidth = Chart{
		ID:    "host_network_bandwidth",
		Title: "Host Network Cumulative Bandwidth",
		Units: "kilobits",
		Fam:   "host",
		Ctx:   "cockroachdb.host_network_bandwidth",
		Type:  collectorapi.Area,
		Dims: Dims{
			{ID: metricSysHostNetRecvBytes, Name: "received", Div: 1000, Algo: collectorapi.Incremental},
			{ID: metricSysHostNetSendBytes, Name: "sent", Div: -1000, Algo: collectorapi.Incremental},
		},
	}
	chartHostNetworkPackets = Chart{
		ID:    "host_network_packets",
		Title: "Host Network Cumulative Packets",
		Units: "packets",
		Fam:   "host",
		Ctx:   "cockroachdb.host_network_packets",
		Dims: Dims{
			{ID: metricSysHostNetRecvPackets, Name: "received", Algo: collectorapi.Incremental},
			{ID: metricSysHostNetSendPackets, Name: "sent", Mul: -1, Algo: collectorapi.Incremental},
		},
	}
)

// Liveness
var (
	chartLiveNodes = Chart{
		ID:    "live_nodes",
		Title: "Live Nodes in the Cluster",
		Units: "nodes",
		Fam:   "liveness",
		Ctx:   "cockroachdb.live_nodes",
		Dims: Dims{
			{ID: metricLiveNodes, Name: "live nodes"},
		},
	}
	chartHeartBeats = Chart{
		ID:    "node_liveness_heartbeats",
		Title: "Node Liveness Heartbeats",
		Units: "heartbeats",
		Fam:   "liveness",
		Ctx:   "cockroachdb.node_liveness_heartbeats",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: metricHeartBeatSuccesses, Name: "successful", Algo: collectorapi.Incremental},
			{ID: metricHeartBeatFailures, Name: "failed", Algo: collectorapi.Incremental},
		},
	}
)

// Capacity
var (
	chartCapacity = Chart{
		ID:    "total_storage_capacity",
		Title: "Total Storage Capacity",
		Units: "KiB",
		Fam:   "capacity",
		Ctx:   "cockroachdb.total_storage_capacity",
		Dims: Dims{
			{ID: metricCapacity, Name: "total", Div: 1024},
		},
	}
	chartCapacityUsability = Chart{
		ID:    "storage_capacity_usability",
		Title: "Storage Capacity Usability",
		Units: "KiB",
		Fam:   "capacity",
		Ctx:   "cockroachdb.storage_capacity_usability",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: metricCapacityUsable, Name: "usable", Div: 1024},
			{ID: metricCapacityUnusable, Name: "unusable", Div: 1024},
		},
	}
	chartCapacityUsable = Chart{
		ID:    "storage_usable_capacity",
		Title: "Storage Usable Capacity",
		Units: "KiB",
		Fam:   "capacity",
		Ctx:   "cockroachdb.storage_usable_capacity",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: metricCapacityAvailable, Name: "available", Div: 1024},
			{ID: metricCapacityUsed, Name: "used", Div: 1024},
		},
	}
	chartCapacityUsedPercentage = Chart{
		ID:    "storage_used_capacity_percentage",
		Title: "Storage Used Capacity Utilization",
		Units: "percentage",
		Fam:   "capacity",
		Ctx:   "cockroachdb.storage_used_capacity_percentage",
		Dims: Dims{
			{ID: metricCapacityUsedPercentage, Name: "total", Div: precision},
			{ID: metricCapacityUsableUsedPercentage, Name: "usable", Div: precision},
		},
	}
)

// SQL
var (
	chartSQLConnections = Chart{
		ID:    "sql_connections",
		Title: "Active SQL Connections",
		Units: "connections",
		Fam:   "sql",
		Ctx:   "cockroachdb.sql_connections",
		Dims: Dims{
			{ID: metricSQLConnections, Name: "active"},
		},
	}
	chartSQLTraffic = Chart{
		ID:    "sql_bandwidth",
		Title: "SQL Bandwidth",
		Units: "KiB",
		Fam:   "sql",
		Ctx:   "cockroachdb.sql_bandwidth",
		Type:  collectorapi.Area,
		Dims: Dims{
			{ID: metricSQLBytesIn, Name: "received", Div: 1024, Algo: collectorapi.Incremental},
			{ID: metricSQLBytesOut, Name: "sent", Div: -1024, Algo: collectorapi.Incremental},
		},
	}
	chartSQLStatementsTotal = Chart{
		ID:    "sql_statements_total",
		Title: "SQL Statements Total",
		Units: "statements",
		Fam:   "sql",
		Ctx:   "cockroachdb.sql_statements_total",
		Type:  collectorapi.Area,
		Dims: Dims{
			{ID: metricSQLQueryStartedCount, Name: "started", Algo: collectorapi.Incremental},
			{ID: metricSQLQueryCount, Name: "executed", Algo: collectorapi.Incremental},
		},
	}
	chartSQLErrors = Chart{
		ID:    "sql_errors",
		Title: "SQL Statements and Transaction Errors",
		Units: "errors",
		Fam:   "sql",
		Ctx:   "cockroachdb.sql_errors",
		Dims: Dims{
			{ID: metricSQLFailureCount, Name: "statement", Algo: collectorapi.Incremental},
			{ID: metricSQLTXNAbortCount, Name: "transaction", Algo: collectorapi.Incremental},
		},
	}
	chartSQLStartedDDLStatements = Chart{
		ID:    "sql_started_ddl_statements",
		Title: "SQL Started DDL Statements",
		Units: "statements",
		Fam:   "sql",
		Ctx:   "cockroachdb.sql_started_ddl_statements",
		Dims: Dims{
			{ID: metricSQLDDLStartedCount, Name: "DDL"},
		},
	}
	chartSQLExecutedDDLStatements = Chart{
		ID:    "sql_executed_ddl_statements",
		Title: "SQL Executed DDL Statements",
		Units: "statements",
		Fam:   "sql",
		Ctx:   "cockroachdb.sql_executed_ddl_statements",
		Dims: Dims{
			{ID: metricSQLDDLCount, Name: "DDL"},
		},
	}
	chartSQLStartedDMLStatements = Chart{
		ID:    "sql_started_dml_statements",
		Title: "SQL Started DML Statements",
		Units: "statements",
		Fam:   "sql",
		Ctx:   "cockroachdb.sql_started_dml_statements",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: metricSQLSelectStartedCount, Name: "SELECT", Algo: collectorapi.Incremental},
			{ID: metricSQLUpdateStartedCount, Name: "UPDATE", Algo: collectorapi.Incremental},
			{ID: metricSQLInsertStartedCount, Name: "INSERT", Algo: collectorapi.Incremental},
			{ID: metricSQLDeleteStartedCount, Name: "DELETE", Algo: collectorapi.Incremental},
		},
	}
	chartSQLExecutedDMLStatements = Chart{
		ID:    "sql_executed_dml_statements",
		Title: "SQL Executed DML Statements",
		Units: "statements",
		Fam:   "sql",
		Ctx:   "cockroachdb.sql_executed_dml_statements",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: metricSQLSelectCount, Name: "SELECT", Algo: collectorapi.Incremental},
			{ID: metricSQLUpdateCount, Name: "UPDATE", Algo: collectorapi.Incremental},
			{ID: metricSQLInsertCount, Name: "INSERT", Algo: collectorapi.Incremental},
			{ID: metricSQLDeleteCount, Name: "DELETE", Algo: collectorapi.Incremental},
		},
	}
	chartSQLStartedTCLStatements = Chart{
		ID:    "sql_started_tcl_statements",
		Title: "SQL Started TCL Statements",
		Units: "statements",
		Fam:   "sql",
		Ctx:   "cockroachdb.sql_started_tcl_statements",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: metricSQLTXNBeginStartedCount, Name: "BEGIN", Algo: collectorapi.Incremental},
			{ID: metricSQLTXNCommitStartedCount, Name: "COMMIT", Algo: collectorapi.Incremental},
			{ID: metricSQLTXNRollbackStartedCount, Name: "ROLLBACK", Algo: collectorapi.Incremental},
			{ID: metricSQLSavepointStartedCount, Name: "SAVEPOINT", Algo: collectorapi.Incremental},
			{ID: metricSQLRestartSavepointStartedCount, Name: "SAVEPOINT cockroach_restart", Algo: collectorapi.Incremental},
			{ID: metricSQLRestartSavepointReleaseStartedCount, Name: "RELEASE SAVEPOINT cockroach_restart", Algo: collectorapi.Incremental},
			{ID: metricSQLRestartSavepointRollbackStartedCount, Name: "ROLLBACK TO SAVEPOINT cockroach_restart", Algo: collectorapi.Incremental},
		},
	}
	chartSQLExecutedTCLStatements = Chart{
		ID:    "sql_executed_tcl_statements",
		Title: "SQL Executed TCL Statements",
		Units: "statements",
		Fam:   "sql",
		Ctx:   "cockroachdb.sql_executed_tcl_statements",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: metricSQLTXNBeginCount, Name: "BEGIN", Algo: collectorapi.Incremental},
			{ID: metricSQLTXNCommitCount, Name: "COMMIT", Algo: collectorapi.Incremental},
			{ID: metricSQLTXNRollbackCount, Name: "ROLLBACK", Algo: collectorapi.Incremental},
			{ID: metricSQLSavepointCount, Name: "SAVEPOINT", Algo: collectorapi.Incremental},
			{ID: metricSQLRestartSavepointCount, Name: "SAVEPOINT cockroach_restart", Algo: collectorapi.Incremental},
			{ID: metricSQLRestartSavepointReleaseCount, Name: "RELEASE SAVEPOINT cockroach_restart", Algo: collectorapi.Incremental},
			{ID: metricSQLRestartSavepointRollbackCount, Name: "ROLLBACK TO SAVEPOINT cockroach_restart", Algo: collectorapi.Incremental},
		},
	}
	chartSQLActiveDistQueries = Chart{
		ID:    "sql_active_distributed_queries",
		Title: "Active Distributed SQL Queries",
		Units: "queries",
		Fam:   "sql",
		Ctx:   "cockroachdb.sql_active_distributed_queries",
		Dims: Dims{
			{ID: metricSQLDistSQLQueriesActive, Name: "active"},
		},
	}
	chartSQLActiveFlowsForDistQueries = Chart{
		ID:    "sql_distributed_flows",
		Title: "Distributed SQL Flows",
		Units: "flows",
		Fam:   "sql",
		Ctx:   "cockroachdb.sql_distributed_flows",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: metricSQLDistSQLFlowsActive, Name: "active"},
			{ID: metricSQLDistSQLFlowsQueued, Name: "queued"},
		},
	}
)

// Storage
var (
	chartUsedLiveData = Chart{
		ID:    "live_bytes",
		Title: "Used Live Data",
		Units: "KiB",
		Fam:   "storage",
		Ctx:   "cockroachdb.live_bytes",
		Dims: Dims{
			{ID: metricLiveBytes, Name: "applications", Div: 1024},
			{ID: metricSysBytes, Name: "system", Div: 1024},
		},
	}
	chartLogicalData = Chart{
		ID:    "logical_data",
		Title: "Logical Data",
		Units: "KiB",
		Fam:   "storage",
		Ctx:   "cockroachdb.logical_data",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: metricKeyBytes, Name: "keys", Div: 1024},
			{ID: metricValBytes, Name: "values", Div: 1024},
		},
	}
	chartLogicalDataCount = Chart{
		ID:    "logical_data_count",
		Title: "Logical Data Count",
		Units: "num",
		Fam:   "storage",
		Ctx:   "cockroachdb.logical_data_count",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: metricKeyCount, Name: "keys"},
			{ID: metricValCount, Name: "values"},
		},
	}
)

// KV Transactions
var (
	chartKVTransactions = Chart{
		ID:    "kv_transactions",
		Title: "KV Transactions",
		Units: "transactions",
		Fam:   "kv transactions",
		Ctx:   "cockroachdb.kv_transactions",
		Type:  collectorapi.Area,
		Dims: Dims{
			{ID: metricTxnCommits, Name: "committed", Algo: collectorapi.Incremental},
			{ID: metricTxnCommits1PC, Name: "fast-path_committed", Algo: collectorapi.Incremental},
			{ID: metricTxnAborts, Name: "aborted", Algo: collectorapi.Incremental},
		},
	}
	chartKVTransactionsRestarts = Chart{
		ID:    "kv_transaction_restarts",
		Title: "KV Transaction Restarts",
		Units: "restarts",
		Fam:   "kv transactions",
		Ctx:   "cockroachdb.kv_transaction_restarts",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: metricTxnRestartsWriteTooOld, Name: "write too old", Algo: collectorapi.Incremental},
			{ID: metricTxnRestartsWriteTooOldMulti, Name: "write too old (multiple)", Algo: collectorapi.Incremental},
			{ID: metricTxnRestartsSerializable, Name: "forwarded timestamp (iso=serializable)", Algo: collectorapi.Incremental},
			{ID: metricTxnRestartsPossibleReplay, Name: "possible reply", Algo: collectorapi.Incremental},
			{ID: metricTxnRestartsAsyncWriteFailure, Name: "async consensus failure", Algo: collectorapi.Incremental},
			{ID: metricTxnRestartsReadWithInUncertainty, Name: "read within uncertainty interval", Algo: collectorapi.Incremental},
			{ID: metricTxnRestartsTxnAborted, Name: "aborted", Algo: collectorapi.Incremental},
			{ID: metricTxnRestartsTxnPush, Name: "push failure", Algo: collectorapi.Incremental},
			{ID: metricTxnRestartsUnknown, Name: "unknown", Algo: collectorapi.Incremental},
		},
	}
)

// Ranges
var (
	chartRanges = Chart{
		ID:    "ranges",
		Title: "Ranges",
		Units: "ranges",
		Fam:   "ranges",
		Ctx:   "cockroachdb.ranges",
		Dims: Dims{
			{ID: metricRanges, Name: "ranges"},
		},
	}
	chartRangesWithProblems = Chart{
		ID:    "ranges_replication_problem",
		Title: "Ranges Replication Problems",
		Units: "ranges",
		Fam:   "ranges",
		Ctx:   "cockroachdb.ranges_replication_problem",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: metricRangesUnavailable, Name: "unavailable"},
			{ID: metricRangesUnderReplicated, Name: "under_replicated"},
			{ID: metricRangesOverReplicated, Name: "over_replicated"},
		},
	}
	chartRangesEvents = Chart{
		ID:    "range_events",
		Title: "Range Events",
		Units: "events",
		Fam:   "ranges",
		Ctx:   "cockroachdb.range_events",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: metricRangeSplits, Name: "split", Algo: collectorapi.Incremental},
			{ID: metricRangeAdds, Name: "add", Algo: collectorapi.Incremental},
			{ID: metricRangeRemoves, Name: "remove", Algo: collectorapi.Incremental},
			{ID: metricRangeMerges, Name: "merge", Algo: collectorapi.Incremental},
		},
	}
	chartRangesSnapshotEvents = Chart{
		ID:    "range_snapshot_events",
		Title: "Range Snapshot Events",
		Units: "events",
		Fam:   "ranges",
		Ctx:   "cockroachdb.range_snapshot_events",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: metricRangeSnapshotsGenerated, Name: "generated", Algo: collectorapi.Incremental},
			{ID: metricRangeSnapshotsNormalApplied, Name: "applied (raft-initiated)", Algo: collectorapi.Incremental},
			{ID: metricRangeSnapshotsLearnerApplied, Name: "applied (learner)", Algo: collectorapi.Incremental},
			{ID: metricRangeSnapshotsPreemptiveApplied, Name: "applied (preemptive)", Algo: collectorapi.Incremental},
		},
	}
)

// RocksDB
var (
	chartRocksDBReadAmplification = Chart{
		ID:    "rocksdb_read_amplification",
		Title: "RocksDB Read Amplification",
		Units: "reads/query",
		Fam:   "rocksdb",
		Ctx:   "cockroachdb.rocksdb_read_amplification",
		Dims: Dims{
			{ID: metricRocksDBReadAmplification, Name: "reads"},
		},
	}
	chartRocksDBTableOperations = Chart{
		ID:    "rocksdb_table_operations",
		Title: "RocksDB Table Operations",
		Units: "operations",
		Fam:   "rocksdb",
		Ctx:   "cockroachdb.rocksdb_table_operations",
		Dims: Dims{
			{ID: metricRocksDBCompactions, Name: "compactions", Algo: collectorapi.Incremental},
			{ID: metricRocksDBFlushes, Name: "flushes", Algo: collectorapi.Incremental},
		},
	}
	chartRocksDBCacheUsage = Chart{
		ID:    "rocksdb_cache_usage",
		Title: "RocksDB Block Cache Usage",
		Units: "KiB",
		Fam:   "rocksdb",
		Ctx:   "cockroachdb.rocksdb_cache_usage",
		Type:  collectorapi.Area,
		Dims: Dims{
			{ID: metricRocksDBBlockCacheUsage, Name: "used", Div: 1024},
		},
	}
	chartRocksDBCacheOperations = Chart{
		ID:    "rocksdb_cache_operations",
		Title: "RocksDB Block Cache Operations",
		Units: "operations",
		Fam:   "rocksdb",
		Ctx:   "cockroachdb.rocksdb_cache_operations",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: metricRocksDBBlockCacheHits, Name: "hits", Algo: collectorapi.Incremental},
			{ID: metricRocksDBBlockCacheMisses, Name: "misses", Algo: collectorapi.Incremental},
		},
	}
	chartRocksDBCacheHitRage = Chart{
		ID:    "rocksdb_cache_hit_rate",
		Title: "RocksDB Block Cache Hit Rate",
		Units: "percentage",
		Fam:   "rocksdb",
		Ctx:   "cockroachdb.rocksdb_cache_hit_rate",
		Type:  collectorapi.Area,
		Dims: Dims{
			{ID: metricRocksDBBlockCacheHitRate, Name: "hit rate"},
		},
	}
	chartRocksDBSSTables = Chart{
		ID:    "rocksdb_sstables",
		Title: "RocksDB SSTables",
		Units: "sstables",
		Fam:   "rocksdb",
		Ctx:   "cockroachdb.rocksdb_sstables",
		Dims: Dims{
			{ID: metricRocksDBNumSSTables, Name: "sstables"},
		},
	}
)

// Replicas
var (
	chartReplicas = Chart{
		ID:    "replicas",
		Title: "Number of Replicas",
		Units: "replicas",
		Fam:   "replication",
		Ctx:   "cockroachdb.replicas",
		Dims: Dims{
			{ID: metricReplicas, Name: "replicas"},
		},
	}
	chartReplicasQuiescence = Chart{
		ID:    "replicas_quiescence",
		Title: "Replicas Quiescence",
		Units: "replicas",
		Fam:   "replication",
		Ctx:   "cockroachdb.replicas_quiescence",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: metricReplicasQuiescent, Name: "quiescent"},
			{ID: metricReplicasActive, Name: "active"},
		},
	}
	chartReplicasLeaders = Chart{
		ID:    "replicas_leaders",
		Title: "Number of Raft Leaders",
		Units: "replicas",
		Fam:   "replication",
		Ctx:   "cockroachdb.replicas_leaders",
		Type:  collectorapi.Area,
		Dims: Dims{
			{ID: metricReplicasLeaders, Name: "leaders"},
			{ID: metricReplicasLeadersNotLeaseholders, Name: "not leaseholders"},
		},
	}
	chartReplicasLeaseHolder = Chart{
		ID:    "replicas_leaseholders",
		Title: "Number of Leaseholders",
		Units: "leaseholders",
		Fam:   "replication",
		Ctx:   "cockroachdb.replicas_leaseholders",
		Dims: Dims{
			{ID: metricReplicasLeaseholders, Name: "leaseholders"},
		},
	}
)

// Queues
var (
	chartQueuesProcessingFailures = Chart{
		ID:    "queue_processing_failures",
		Title: "Queues Processing Failures",
		Units: "failures",
		Fam:   "queues",
		Ctx:   "cockroachdb.queue_processing_failures",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: metricQueueGCProcessFailure, Name: "gc", Algo: collectorapi.Incremental},
			{ID: metricQueueReplicaGCProcessFailure, Name: "replica gc", Algo: collectorapi.Incremental},
			{ID: metricQueueReplicateProcessFailure, Name: "replication", Algo: collectorapi.Incremental},
			{ID: metricQueueSplitProcessFailure, Name: "split", Algo: collectorapi.Incremental},
			{ID: metricQueueConsistencyProcessFailure, Name: "consistency", Algo: collectorapi.Incremental},
			{ID: metricQueueRaftLogProcessFailure, Name: "raft log", Algo: collectorapi.Incremental},
			{ID: metricQueueRaftSnapshotProcessFailure, Name: "raft snapshot", Algo: collectorapi.Incremental},
			{ID: metricQueueTSMaintenanceProcessFailure, Name: "time series maintenance", Algo: collectorapi.Incremental},
		},
	}
)

// Rebalancing
var (
	chartRebalancingQueries = Chart{
		ID:    "rebalancing_queries",
		Title: "Rebalancing Average Queries",
		Units: "queries/s",
		Fam:   "rebalancing",
		Ctx:   "cockroachdb.rebalancing_queries",
		Dims: Dims{
			{ID: metricRebalancingQueriesPerSecond, Name: "avg", Div: precision},
		},
	}
	chartRebalancingWrites = Chart{
		ID:    "rebalancing_writes",
		Title: "Rebalancing Average Writes",
		Units: "writes/s",
		Fam:   "rebalancing",
		Ctx:   "cockroachdb.rebalancing_writes",
		Dims: Dims{
			{ID: metricRebalancingWritesPerSecond, Name: "avg", Div: precision},
		},
	}
)

// Time Series
var (
	chartTimeSeriesWrittenSamples = Chart{
		ID:    "timeseries_samples",
		Title: "Time Series Written Samples",
		Units: "samples",
		Fam:   "time series",
		Ctx:   "cockroachdb.timeseries_samples",
		Dims: Dims{
			{ID: metricTimeSeriesWriteSamples, Name: "written", Algo: collectorapi.Incremental},
		},
	}
	chartTimeSeriesWriteErrors = Chart{
		ID:    "timeseries_write_errors",
		Title: "Time Series Write Errors",
		Units: "errors",
		Fam:   "time series",
		Ctx:   "cockroachdb.timeseries_write_errors",
		Dims: Dims{
			{ID: metricTimeSeriesWriteErrors, Name: "write", Algo: collectorapi.Incremental},
		},
	}
	chartTimeSeriesWrittenBytes = Chart{
		ID:    "timeseries_write_bytes",
		Title: "Time Series Bytes Written",
		Units: "KiB",
		Fam:   "time series",
		Ctx:   "cockroachdb.timeseries_write_bytes",
		Dims: Dims{
			{ID: metricTimeSeriesWriteBytes, Name: "written", Algo: collectorapi.Incremental},
		},
	}
)

// Slow Requests
var (
	chartSlowRequests = Chart{
		ID:    "slow_requests",
		Title: "Slow Requests",
		Units: "requests",
		Fam:   "slow requests",
		Ctx:   "cockroachdb.slow_requests",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: metricRequestsSlowLatch, Name: "acquiring latches"},
			{ID: metricRequestsSlowLease, Name: "acquiring lease"},
			{ID: metricRequestsSlowRaft, Name: "in raft"},
		},
	}
)

// Go/Cgo
var (
	chartGoCgoHeapMemory = Chart{
		ID:    "code_heap_memory_usage",
		Title: "Heap Memory Usage",
		Units: "KiB",
		Fam:   "go/cgo",
		Ctx:   "cockroachdb.code_heap_memory_usage",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: metricSysGoAllocBytes, Name: "go", Div: 1024},
			{ID: metricSysCGoAllocBytes, Name: "cgo", Div: 1024},
		},
	}
	chartGoroutines = Chart{
		ID:    "goroutines_count",
		Title: "Number of Goroutines",
		Units: "goroutines",
		Fam:   "go/cgo",
		Ctx:   "cockroachdb.goroutines",
		Dims: Dims{
			{ID: metricSysGoroutines, Name: "goroutines"},
		},
	}
	chartGCRuns = Chart{
		ID:    "gc_count",
		Title: "GC Runs",
		Units: "invokes",
		Fam:   "go/cgo",
		Ctx:   "cockroachdb.gc_count",
		Dims: Dims{
			{ID: metricSysGCCount, Name: "gc", Algo: collectorapi.Incremental},
		},
	}
	chartGCPauseTime = Chart{
		ID:    "gc_pause",
		Title: "GC Pause Time",
		Units: "us",
		Fam:   "go/cgo",
		Ctx:   "cockroachdb.gc_pause",
		Dims: Dims{
			{ID: metricSysGCPauseNs, Name: "pause", Algo: collectorapi.Incremental, Div: 1e3},
		},
	}
	chartCGoCalls = Chart{
		ID:    "cgo_calls",
		Title: "Cgo Calls",
		Units: "calls",
		Fam:   "go/cgo",
		Ctx:   "cockroachdb.cgo_calls",
		Dims: Dims{
			{ID: metricSysCGoCalls, Name: "cgo", Algo: collectorapi.Incremental},
		},
	}
)
