// SPDX-License-Identifier: GPL-3.0-or-later

package mongo

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioOperationsRate = collectorapi.Priority + iota
	prioOperationsLatencyTime
	prioOperationsByTypeRate
	prioDocumentOperationsRate
	prioScannedIndexesRate
	prioScannedDocumentsRate

	prioActiveClientsCount
	prioQueuedOperationsCount

	prioGlobalLockAcquisitionsRate
	prioDatabaseLockAcquisitionsRate
	prioCollectionLockAcquisitionsRate
	prioMutexLockAcquisitionsRate
	prioMetadataLockAcquisitionsRate
	prioOpLogLockAcquisitionsRate

	prioCursorsOpenCount
	prioCursorsOpenNoTimeoutCount
	prioCursorsOpenedRate
	prioTimedOutCursorsRate
	prioCursorsByLifespanCount

	prioTransactionsCount
	prioTransactionsRate
	prioTransactionsNoShardsCommitsRate
	prioTransactionsNoShardsCommitsDurationTime
	prioTransactionsSingleShardCommitsRate
	prioTransactionsSingleShardCommitsDurationTime
	prioTransactionsSingleWriteShardCommitsRate
	prioTransactionsSingleWriteShardCommitsDurationTime
	prioTransactionsReadOnlyCommitsRate
	prioTransactionsReadOnlyCommitsDurationTime
	prioTransactionsTwoPhaseCommitCommitsRate
	prioTransactionsTwoPhaseCommitCommitsDurationTime
	prioTransactionsRecoverWithTokenCommitsRate
	prioTransactionsRecoverWithTokenCommitsDurationTime

	prioConnectionsUsage
	prioConnectionsByStateCount
	prioConnectionsRate

	prioAssertsRate

	prioNetworkTrafficRate
	prioNetworkRequestsRate
	prioNetworkSlowDNSResolutionsRate
	prioNetworkSlowSSLHandshakesRate

	prioMemoryResidentSize
	prioMemoryVirtualSize
	prioMemoryPageFaultsRate
	prioMemoryTCMallocStats

	prioWiredTigerConcurrentReadTransactionsUsage
	prioWiredTigerConcurrentWriteTransactionsUsage
	prioWiredTigerCacheUsage
	prioWiredTigerCacheDirtySpaceSize
	prioWiredTigerCacheIORate
	prioWiredTigerCacheEvictionsRate

	prioDatabaseCollectionsCount
	prioDatabaseIndexesCount
	prioDatabaseViewsCount
	prioDatabaseDocumentsCount
	prioDatabaseDataSize
	prioDatabaseStorageSize
	prioDatabaseIndexSize

	prioReplSetMemberState
	prioReplSetMemberHealthStatus
	prioReplSetMemberReplicationLagTime
	prioReplSetMemberHeartbeatLatencyTime
	prioReplSetMemberPingRTTTime
	prioReplSetMemberUptime

	prioShardingNodesCount
	prioShardingShardedDatabasesCount
	prioShardingShardedCollectionsCount
	prioShardChunks
)

const (
	chartPxDatabase      = "database_"
	chartPxReplSetMember = "replica_set_member_"
	chartPxShard         = "sharding_shard_"
)

// these charts are expected to be available in many versions
var chartsServerStatus = collectorapi.Charts{
	chartOperationsByTypeRate.Copy(),
	chartDocumentOperationsRate.Copy(),
	chartScannedIndexesRate.Copy(),
	chartScannedDocumentsRate.Copy(),

	chartConnectionsUsage.Copy(),
	chartConnectionsByStateCount.Copy(),
	chartConnectionsRate.Copy(),

	chartNetworkTrafficRate.Copy(),
	chartNetworkRequestsRate.Copy(),

	chartMemoryResidentSize.Copy(),
	chartMemoryVirtualSize.Copy(),
	chartMemoryPageFaultsRate.Copy(),

	chartAssertsRate.Copy(),
}

var chartsTmplDatabase = collectorapi.Charts{
	chartTmplDatabaseCollectionsCount.Copy(),
	chartTmplDatabaseIndexesCount.Copy(),
	chartTmplDatabaseViewsCount.Copy(),
	chartTmplDatabaseDocumentsCount.Copy(),
	chartTmplDatabaseDataSize.Copy(),
	chartTmplDatabaseStorageSize.Copy(),
	chartTmplDatabaseIndexSize.Copy(),
}

var chartsTmplReplSetMember = collectorapi.Charts{
	chartTmplReplSetMemberState.Copy(),
	chartTmplReplSetMemberHealthStatus.Copy(),
	chartTmplReplSetMemberReplicationLagTime.Copy(),
	chartTmplReplSetMemberHeartbeatLatencyTime.Copy(),
	chartTmplReplSetMemberPingRTTTime.Copy(),
	chartTmplReplSetMemberUptime.Copy(),
}

var chartsSharding = collectorapi.Charts{
	chartShardingNodesCount.Copy(),
	chartShardingShardedDatabases.Copy(),
	chartShardingShardedCollectionsCount.Copy(),
}

var chartsTmplShardingShard = collectorapi.Charts{
	chartTmplShardChunks.Copy(),
}

var (
	chartOperationsRate = collectorapi.Chart{
		ID:       "operations_rate",
		Title:    "Operations rate",
		Units:    "operations/s",
		Fam:      "operations",
		Ctx:      "mongodb.operations_rate",
		Priority: prioOperationsRate,
		Dims: collectorapi.Dims{
			{ID: "operations_latencies_reads_ops", Name: "reads", Algo: collectorapi.Incremental},
			{ID: "operations_latencies_writes_ops", Name: "writes", Algo: collectorapi.Incremental},
			{ID: "operations_latencies_commands_ops", Name: "commands", Algo: collectorapi.Incremental},
		},
	}
	chartOperationsLatencyTime = collectorapi.Chart{
		ID:       "operations_latency_time",
		Title:    "Operations Latency",
		Units:    "milliseconds",
		Fam:      "operations",
		Ctx:      "mongodb.operations_latency_time",
		Priority: prioOperationsLatencyTime,
		Dims: collectorapi.Dims{
			{ID: "operations_latencies_reads_latency", Name: "reads", Algo: collectorapi.Incremental, Div: 1000},
			{ID: "operations_latencies_writes_latency", Name: "writes", Algo: collectorapi.Incremental, Div: 1000},
			{ID: "operations_latencies_commands_latency", Name: "commands", Algo: collectorapi.Incremental, Div: 1000},
		},
	}
	chartOperationsByTypeRate = collectorapi.Chart{
		ID:       "operations_by_type_rate",
		Title:    "Operations by type",
		Units:    "operations/s",
		Fam:      "operations",
		Ctx:      "mongodb.operations_by_type_rate",
		Priority: prioOperationsByTypeRate,
		Dims: collectorapi.Dims{
			{ID: "operations_insert", Name: "insert", Algo: collectorapi.Incremental},
			{ID: "operations_query", Name: "query", Algo: collectorapi.Incremental},
			{ID: "operations_update", Name: "update", Algo: collectorapi.Incremental},
			{ID: "operations_delete", Name: "delete", Algo: collectorapi.Incremental},
			{ID: "operations_getmore", Name: "getmore", Algo: collectorapi.Incremental},
			{ID: "operations_command", Name: "command", Algo: collectorapi.Incremental},
		},
	}
	chartDocumentOperationsRate = collectorapi.Chart{
		ID:       "document_operations_rate",
		Title:    "Document operations",
		Units:    "operations/s",
		Fam:      "operations",
		Ctx:      "mongodb.document_operations_rate",
		Type:     collectorapi.Stacked,
		Priority: prioDocumentOperationsRate,
		Dims: collectorapi.Dims{
			{ID: "metrics_document_inserted", Name: "inserted", Algo: collectorapi.Incremental},
			{ID: "metrics_document_deleted", Name: "deleted", Algo: collectorapi.Incremental},
			{ID: "metrics_document_returned", Name: "returned", Algo: collectorapi.Incremental},
			{ID: "metrics_document_updated", Name: "updated", Algo: collectorapi.Incremental},
		},
	}
	chartScannedIndexesRate = collectorapi.Chart{
		ID:       "scanned_indexes_rate",
		Title:    "Scanned indexes",
		Units:    "indexes/s",
		Fam:      "operations",
		Ctx:      "mongodb.scanned_indexes_rate",
		Priority: prioScannedIndexesRate,
		Dims: collectorapi.Dims{
			{ID: "metrics_query_executor_scanned", Name: "scanned", Algo: collectorapi.Incremental},
		},
	}
	chartScannedDocumentsRate = collectorapi.Chart{
		ID:       "scanned_documents_rate",
		Title:    "Scanned documents",
		Units:    "documents/s",
		Fam:      "operations",
		Ctx:      "mongodb.scanned_documents_rate",
		Priority: prioScannedDocumentsRate,
		Dims: collectorapi.Dims{
			{ID: "metrics_query_executor_scanned_objects", Name: "scanned", Algo: collectorapi.Incremental},
		},
	}

	chartGlobalLockActiveClientsCount = collectorapi.Chart{
		ID:       "active_clients_count",
		Title:    "Connected clients",
		Units:    "clients",
		Fam:      "clients",
		Ctx:      "mongodb.active_clients_count",
		Priority: prioActiveClientsCount,
		Dims: collectorapi.Dims{
			{ID: "global_lock_active_clients_readers", Name: "readers"},
			{ID: "global_lock_active_clients_writers", Name: "writers"},
		},
	}
	chartGlobalLockCurrentQueueCount = collectorapi.Chart{
		ID:       "queued_operations",
		Title:    "Queued operations because of a lock",
		Units:    "operations",
		Fam:      "clients",
		Ctx:      "mongodb.queued_operations_count",
		Priority: prioQueuedOperationsCount,
		Dims: collectorapi.Dims{
			{ID: "global_lock_current_queue_readers", Name: "readers"},
			{ID: "global_lock_current_queue_writers", Name: "writers"},
		},
	}

	chartConnectionsUsage = collectorapi.Chart{
		ID:       "connections_usage",
		Title:    "Connections usage",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "mongodb.connections_usage",
		Type:     collectorapi.Stacked,
		Priority: prioConnectionsUsage,
		Dims: collectorapi.Dims{
			{ID: "connections_available", Name: "available"},
			{ID: "connections_current", Name: "used"},
		},
	}
	chartConnectionsByStateCount = collectorapi.Chart{
		ID:       "connections_by_state_count",
		Title:    "Connections By State",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "mongodb.connections_by_state_count",
		Priority: prioConnectionsByStateCount,
		Dims: collectorapi.Dims{
			{ID: "connections_active", Name: "active"},
			{ID: "connections_threaded", Name: "threaded"},
			{ID: "connections_exhaust_is_master", Name: "exhaust_is_master"},
			{ID: "connections_exhaust_hello", Name: "exhaust_hello"},
			{ID: "connections_awaiting_topology_changes", Name: "awaiting_topology_changes"},
		},
	}
	chartConnectionsRate = collectorapi.Chart{
		ID:       "connections_rate",
		Title:    "Connections Rate",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "mongodb.connections_rate",
		Priority: prioConnectionsRate,
		Dims: collectorapi.Dims{
			{ID: "connections_total_created", Name: "created", Algo: collectorapi.Incremental},
		},
	}

	chartNetworkTrafficRate = collectorapi.Chart{
		ID:       "network_traffic",
		Title:    "Network traffic",
		Units:    "bytes/s",
		Fam:      "network",
		Ctx:      "mongodb.network_traffic_rate",
		Priority: prioNetworkTrafficRate,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "network_bytes_in", Name: "in", Algo: collectorapi.Incremental},
			{ID: "network_bytes_out", Name: "out", Algo: collectorapi.Incremental},
		},
	}
	chartNetworkRequestsRate = collectorapi.Chart{
		ID:       "network_requests_rate",
		Title:    "Network Requests",
		Units:    "requests/s",
		Fam:      "network",
		Ctx:      "mongodb.network_requests_rate",
		Priority: prioNetworkRequestsRate,
		Dims: collectorapi.Dims{
			{ID: "network_requests", Name: "requests", Algo: collectorapi.Incremental},
		},
	}
	chartNetworkSlowDNSResolutionsRate = collectorapi.Chart{
		ID:       "network_slow_dns_resolutions_rate",
		Title:    "Slow DNS resolution operations",
		Units:    "resolutions/s",
		Fam:      "network",
		Ctx:      "mongodb.network_slow_dns_resolutions_rate",
		Priority: prioNetworkSlowDNSResolutionsRate,
		Dims: collectorapi.Dims{
			{ID: "network_slow_dns_operations", Name: "slow_dns", Algo: collectorapi.Incremental},
		},
	}
	chartNetworkSlowSSLHandshakesRate = collectorapi.Chart{
		ID:       "network_slow_ssl_handshakes_rate",
		Title:    "Slow SSL handshake operations",
		Units:    "handshakes/s",
		Fam:      "network",
		Ctx:      "mongodb.network_slow_ssl_handshakes_rate",
		Priority: prioNetworkSlowSSLHandshakesRate,
		Dims: collectorapi.Dims{
			{ID: "network_slow_ssl_operations", Name: "slow_ssl", Algo: collectorapi.Incremental},
		},
	}

	chartMemoryResidentSize = collectorapi.Chart{
		ID:       "memory_resident_size",
		Title:    "Used resident memory",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "mongodb.memory_resident_size",
		Priority: prioMemoryResidentSize,
		Dims: collectorapi.Dims{
			{ID: "memory_resident", Name: "used"},
		},
	}
	chartMemoryVirtualSize = collectorapi.Chart{
		ID:       "memory_virtual_size",
		Title:    "Used virtual memory",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "mongodb.memory_virtual_size",
		Priority: prioMemoryVirtualSize,
		Dims: collectorapi.Dims{
			{ID: "memory_virtual", Name: "used"},
		},
	}
	chartMemoryPageFaultsRate = collectorapi.Chart{
		ID:       "memory_page_faults",
		Title:    "Memory page faults",
		Units:    "pgfaults/s",
		Fam:      "memory",
		Ctx:      "mongodb.memory_page_faults_rate",
		Priority: prioMemoryPageFaultsRate,
		Dims: collectorapi.Dims{
			{ID: "extra_info_page_faults", Name: "pgfaults", Algo: collectorapi.Incremental},
		},
	}
	chartMemoryTCMallocStatsChart = collectorapi.Chart{
		ID:       "memory_tcmalloc_stats",
		Title:    "TCMalloc statistics",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "mongodb.memory_tcmalloc_stats",
		Priority: prioMemoryTCMallocStats,
		Dims: collectorapi.Dims{
			{ID: "tcmalloc_generic_current_allocated_bytes", Name: "allocated"},
			{ID: "tcmalloc_central_cache_free_bytes", Name: "central_cache_freelist"},
			{ID: "tcmalloc_transfer_cache_free_bytes", Name: "transfer_cache_freelist"},
			{ID: "tcmalloc_thread_cache_free_bytes", Name: "thread_cache_freelists"},
			{ID: "tcmalloc_pageheap_free_bytes", Name: "pageheap_freelist"},
			{ID: "tcmalloc_pageheap_unmapped_bytes", Name: "pageheap_unmapped"},
		},
	}

	chartAssertsRate = collectorapi.Chart{
		ID:       "asserts_rate",
		Title:    "Raised assertions",
		Units:    "asserts/s",
		Fam:      "asserts",
		Ctx:      "mongodb.asserts_rate",
		Type:     collectorapi.Stacked,
		Priority: prioAssertsRate,
		Dims: collectorapi.Dims{
			{ID: "asserts_regular", Name: "regular", Algo: collectorapi.Incremental},
			{ID: "asserts_warning", Name: "warning", Algo: collectorapi.Incremental},
			{ID: "asserts_msg", Name: "msg", Algo: collectorapi.Incremental},
			{ID: "asserts_user", Name: "user", Algo: collectorapi.Incremental},
			{ID: "asserts_tripwire", Name: "tripwire", Algo: collectorapi.Incremental},
			{ID: "asserts_rollovers", Name: "rollovers", Algo: collectorapi.Incremental},
		},
	}

	chartTransactionsCount = collectorapi.Chart{
		ID:       "transactions_count",
		Title:    "Current transactions",
		Units:    "transactions",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_count",
		Priority: prioTransactionsCount,
		Dims: collectorapi.Dims{
			{ID: "txn_active", Name: "active"},
			{ID: "txn_inactive", Name: "inactive"},
			{ID: "txn_open", Name: "open"},
			{ID: "txn_prepared", Name: "prepared"},
		},
	}
	chartTransactionsRate = collectorapi.Chart{
		ID:       "transactions_rate",
		Title:    "Transactions rate",
		Units:    "transactions/s",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_rate",
		Priority: prioTransactionsRate,
		Dims: collectorapi.Dims{
			{ID: "txn_total_started", Name: "started", Algo: collectorapi.Incremental},
			{ID: "txn_total_aborted", Name: "aborted", Algo: collectorapi.Incremental},
			{ID: "txn_total_committed", Name: "committed", Algo: collectorapi.Incremental},
			{ID: "txn_total_prepared", Name: "prepared", Algo: collectorapi.Incremental},
		},
	}
	chartTransactionsNoShardsCommitsRate = collectorapi.Chart{
		ID:       "transactions_no_shards_commits_rate",
		Title:    "Transactions commits",
		Units:    "commits/s",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_rate",
		Priority: prioTransactionsNoShardsCommitsRate,
		Type:     collectorapi.Stacked,
		Labels:   []collectorapi.Label{{Key: "commit_type", Value: "noShards"}},
		Dims: collectorapi.Dims{
			{ID: "txn_commit_types_no_shards_successful", Name: "success", Algo: collectorapi.Incremental},
			{ID: "txn_commit_types_no_shards_unsuccessful", Name: "fail", Algo: collectorapi.Incremental},
		},
	}
	chartTransactionsNoShardsCommitsDurationTime = collectorapi.Chart{
		ID:       "transactions_no_shards_commits_duration_time",
		Title:    "Transactions successful commits duration",
		Units:    "milliseconds",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_duration_time",
		Priority: prioTransactionsNoShardsCommitsDurationTime,
		Labels:   []collectorapi.Label{{Key: "commit_type", Value: "noShards"}},
		Dims: collectorapi.Dims{
			{ID: "txn_commit_types_no_shards_successful_duration_micros", Name: "commits", Algo: collectorapi.Incremental, Div: 1000},
		},
	}
	chartTransactionsSingleShardCommitsRate = collectorapi.Chart{
		ID:       "transactions_single_shard_commits_rate",
		Title:    "Transactions commits",
		Units:    "commits/s",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_rate",
		Priority: prioTransactionsSingleShardCommitsRate,
		Type:     collectorapi.Stacked,
		Labels:   []collectorapi.Label{{Key: "commit_type", Value: "singleShard"}},
		Dims: collectorapi.Dims{
			{ID: "txn_commit_types_single_shard_successful", Name: "success", Algo: collectorapi.Incremental},
			{ID: "txn_commit_types_single_shard_unsuccessful", Name: "fail", Algo: collectorapi.Incremental},
		},
	}
	chartTransactionsSingleShardCommitsDurationTime = collectorapi.Chart{
		ID:       "transactions_single_shard_commits_duration_time",
		Title:    "Transactions successful commits duration",
		Units:    "milliseconds",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_duration_time",
		Priority: prioTransactionsSingleShardCommitsDurationTime,
		Labels:   []collectorapi.Label{{Key: "commit_type", Value: "singleShard"}},
		Dims: collectorapi.Dims{
			{ID: "txn_commit_types_single_shard_successful_duration_micros", Name: "commits", Algo: collectorapi.Incremental, Div: 1000},
		},
	}
	chartTransactionsSingleWriteShardCommitsRate = collectorapi.Chart{
		ID:       "transactions_single_write_shard_commits_rate",
		Title:    "Transactions commits",
		Units:    "commits/s",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_rate",
		Priority: prioTransactionsSingleWriteShardCommitsRate,
		Type:     collectorapi.Stacked,
		Labels:   []collectorapi.Label{{Key: "commit_type", Value: "singleWriteShard"}},
		Dims: collectorapi.Dims{
			{ID: "txn_commit_types_single_write_shard_successful", Name: "success", Algo: collectorapi.Incremental},
			{ID: "txn_commit_types_single_write_shard_unsuccessful", Name: "fail", Algo: collectorapi.Incremental},
		},
	}
	chartTransactionsSingleWriteShardCommitsDurationTime = collectorapi.Chart{
		ID:       "transactions_single_write_shard_commits_duration_time",
		Title:    "Transactions successful commits duration",
		Units:    "milliseconds",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_duration_time",
		Priority: prioTransactionsSingleWriteShardCommitsDurationTime,
		Labels:   []collectorapi.Label{{Key: "commit_type", Value: "singleWriteShard"}},
		Dims: collectorapi.Dims{
			{ID: "txn_commit_types_single_write_shard_successful_duration_micros", Name: "commits", Algo: collectorapi.Incremental, Div: 1000},
		},
	}
	chartTransactionsReadOnlyCommitsRate = collectorapi.Chart{
		ID:       "transactions_read_only_commits_rate",
		Title:    "Transactions commits",
		Units:    "commits/s",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_rate",
		Priority: prioTransactionsReadOnlyCommitsRate,
		Type:     collectorapi.Stacked,
		Labels:   []collectorapi.Label{{Key: "commit_type", Value: "readOnly"}},
		Dims: collectorapi.Dims{
			{ID: "txn_commit_types_read_only_successful", Name: "success", Algo: collectorapi.Incremental},
			{ID: "txn_commit_types_read_only_unsuccessful", Name: "fail", Algo: collectorapi.Incremental},
		},
	}
	chartTransactionsReadOnlyCommitsDurationTime = collectorapi.Chart{
		ID:       "transactions_read_only_commits_duration_time",
		Title:    "Transactions successful commits duration",
		Units:    "milliseconds",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_duration_time",
		Priority: prioTransactionsReadOnlyCommitsDurationTime,
		Labels:   []collectorapi.Label{{Key: "commit_type", Value: "readOnly"}},
		Dims: collectorapi.Dims{
			{ID: "txn_commit_types_read_only_successful_duration_micros", Name: "commits", Algo: collectorapi.Incremental, Div: 1000},
		},
	}
	chartTransactionsTwoPhaseCommitCommitsRate = collectorapi.Chart{
		ID:       "transactions_two_phase_commit_commits_rate",
		Title:    "Transactions commits",
		Units:    "commits/s",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_rate",
		Priority: prioTransactionsTwoPhaseCommitCommitsRate,
		Type:     collectorapi.Stacked,
		Labels:   []collectorapi.Label{{Key: "commit_type", Value: "twoPhaseCommit"}},
		Dims: collectorapi.Dims{
			{ID: "txn_commit_types_two_phase_commit_successful", Name: "success", Algo: collectorapi.Incremental},
			{ID: "txn_commit_types_two_phase_commit_unsuccessful", Name: "fail", Algo: collectorapi.Incremental},
		},
	}
	chartTransactionsTwoPhaseCommitCommitsDurationTime = collectorapi.Chart{
		ID:       "transactions_two_phase_commit_commits_duration_time",
		Title:    "Transactions successful commits duration",
		Units:    "milliseconds",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_duration_time",
		Priority: prioTransactionsTwoPhaseCommitCommitsDurationTime,
		Labels:   []collectorapi.Label{{Key: "commit_type", Value: "twoPhaseCommit"}},
		Dims: collectorapi.Dims{
			{ID: "txn_commit_types_two_phase_commit_successful_duration_micros", Name: "commits", Algo: collectorapi.Incremental, Div: 1000},
		},
	}
	chartTransactionsRecoverWithTokenCommitsRate = collectorapi.Chart{
		ID:       "transactions_recover_with_token_commits_rate",
		Title:    "Transactions commits",
		Units:    "commits/s",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_rate",
		Priority: prioTransactionsRecoverWithTokenCommitsRate,
		Type:     collectorapi.Stacked,
		Labels:   []collectorapi.Label{{Key: "commit_type", Value: "recoverWithToken"}},
		Dims: collectorapi.Dims{
			{ID: "txn_commit_types_recover_with_token_successful", Name: "success", Algo: collectorapi.Incremental},
			{ID: "txn_commit_types_recover_with_token_unsuccessful", Name: "fail", Algo: collectorapi.Incremental},
		},
	}
	chartTransactionsRecoverWithTokenCommitsDurationTime = collectorapi.Chart{
		ID:       "transactions_recover_with_token_commits_duration_time",
		Title:    "Transactions successful commits duration",
		Units:    "milliseconds",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_duration_time",
		Priority: prioTransactionsRecoverWithTokenCommitsDurationTime,
		Labels:   []collectorapi.Label{{Key: "commit_type", Value: "recoverWithToken"}},
		Dims: collectorapi.Dims{
			{ID: "txn_commit_types_recover_with_token_successful_duration_micros", Name: "commits", Algo: collectorapi.Incremental, Div: 1000},
		},
	}

	chartGlobalLockAcquisitionsRate = collectorapi.Chart{
		ID:       "global_lock_acquisitions_rate",
		Title:    "Global lock acquisitions",
		Units:    "acquisitions/s",
		Fam:      "locks",
		Ctx:      "mongodb.lock_acquisitions_rate",
		Priority: prioGlobalLockAcquisitionsRate,
		Labels:   []collectorapi.Label{{Key: "lock_type", Value: "global"}},
		Dims: collectorapi.Dims{
			{ID: "locks_global_acquire_shared", Name: "shared", Algo: collectorapi.Incremental},
			{ID: "locks_global_acquire_exclusive", Name: "exclusive", Algo: collectorapi.Incremental},
			{ID: "locks_global_acquire_intent_shared", Name: "intent_shared", Algo: collectorapi.Incremental},
			{ID: "locks_global_acquire_intent_exclusive", Name: "intent_exclusive", Algo: collectorapi.Incremental},
		},
	}
	chartDatabaseLockAcquisitionsRate = collectorapi.Chart{
		ID:       "database_lock_acquisitions_rate",
		Title:    "Database lock acquisitions",
		Units:    "acquisitions/s",
		Fam:      "locks",
		Ctx:      "mongodb.lock_acquisitions_rate",
		Priority: prioDatabaseLockAcquisitionsRate,
		Labels:   []collectorapi.Label{{Key: "lock_type", Value: "database"}},
		Dims: collectorapi.Dims{
			{ID: "locks_database_acquire_shared", Name: "shared", Algo: collectorapi.Incremental},
			{ID: "locks_database_acquire_exclusive", Name: "exclusive", Algo: collectorapi.Incremental},
			{ID: "locks_database_acquire_intent_shared", Name: "intent_shared", Algo: collectorapi.Incremental},
			{ID: "locks_database_acquire_intent_exclusive", Name: "intent_exclusive", Algo: collectorapi.Incremental},
		},
	}
	chartCollectionLockAcquisitionsRate = collectorapi.Chart{
		ID:       "collection_lock_acquisitions_rate",
		Title:    "Collection lock acquisitions",
		Units:    "acquisitions/s",
		Fam:      "locks",
		Ctx:      "mongodb.lock_acquisitions_rate",
		Priority: prioCollectionLockAcquisitionsRate,
		Labels:   []collectorapi.Label{{Key: "lock_type", Value: "collection"}},
		Dims: collectorapi.Dims{
			{ID: "locks_collection_acquire_shared", Name: "shared", Algo: collectorapi.Incremental},
			{ID: "locks_collection_acquire_exclusive", Name: "exclusive", Algo: collectorapi.Incremental},
			{ID: "locks_collection_acquire_intent_shared", Name: "intent_shared", Algo: collectorapi.Incremental},
			{ID: "locks_collection_acquire_intent_exclusive", Name: "intent_exclusive", Algo: collectorapi.Incremental},
		},
	}
	chartMutexLockAcquisitionsRate = collectorapi.Chart{
		ID:       "mutex_lock_acquisitions_rate",
		Title:    "Mutex lock acquisitions",
		Units:    "acquisitions/s",
		Fam:      "locks",
		Ctx:      "mongodb.lock_acquisitions_rate",
		Priority: prioMutexLockAcquisitionsRate,
		Labels:   []collectorapi.Label{{Key: "lock_type", Value: "mutex"}},
		Dims: collectorapi.Dims{
			{ID: "locks_mutex_acquire_shared", Name: "shared", Algo: collectorapi.Incremental},
			{ID: "locks_mutex_acquire_exclusive", Name: "exclusive", Algo: collectorapi.Incremental},
			{ID: "locks_mutex_acquire_intent_shared", Name: "intent_shared", Algo: collectorapi.Incremental},
			{ID: "locks_mutex_acquire_intent_exclusive", Name: "intent_exclusive", Algo: collectorapi.Incremental},
		},
	}
	chartMetadataLockAcquisitionsRate = collectorapi.Chart{
		ID:       "metadata_lock_acquisitions_rate",
		Title:    "Metadata lock acquisitions",
		Units:    "acquisitions/s",
		Fam:      "locks",
		Ctx:      "mongodb.lock_acquisitions_rate",
		Priority: prioMetadataLockAcquisitionsRate,
		Labels:   []collectorapi.Label{{Key: "lock_type", Value: "metadata"}},
		Dims: collectorapi.Dims{
			{ID: "locks_metadata_acquire_shared", Name: "shared", Algo: collectorapi.Incremental},
			{ID: "locks_metadata_acquire_exclusive", Name: "exclusive", Algo: collectorapi.Incremental},
			{ID: "locks_metadata_acquire_intent_shared", Name: "intent_shared", Algo: collectorapi.Incremental},
			{ID: "locks_metadata_acquire_intent_exclusive", Name: "intent_exclusive", Algo: collectorapi.Incremental},
		},
	}
	chartOpLogLockAcquisitionsRate = collectorapi.Chart{
		ID:       "oplog_lock_acquisitions_rate",
		Title:    "Operations log lock acquisitions",
		Units:    "acquisitions/s",
		Fam:      "locks",
		Ctx:      "mongodb.lock_acquisitions_rate",
		Priority: prioOpLogLockAcquisitionsRate,
		Labels:   []collectorapi.Label{{Key: "lock_type", Value: "oplog"}},
		Dims: collectorapi.Dims{
			{ID: "locks_oplog_acquire_shared", Name: "shared", Algo: collectorapi.Incremental},
			{ID: "locks_oplog_acquire_exclusive", Name: "exclusive", Algo: collectorapi.Incremental},
			{ID: "locks_oplog_acquire_intent_shared", Name: "intent_shared", Algo: collectorapi.Incremental},
			{ID: "locks_oplog_acquire_intent_exclusive", Name: "intent_exclusive", Algo: collectorapi.Incremental},
		},
	}

	chartCursorsOpenCount = collectorapi.Chart{
		ID:       "cursors_open_count",
		Title:    "Open cursors",
		Units:    "cursors",
		Fam:      "cursors",
		Ctx:      "mongodb.cursors_open_count",
		Priority: prioCursorsOpenCount,
		Dims: collectorapi.Dims{
			{ID: "metrics_cursor_open_total", Name: "open"},
		},
	}
	chartCursorsOpenNoTimeoutCount = collectorapi.Chart{
		ID:       "cursors_open_no_timeout_count",
		Title:    "Open cursors with disabled timeout",
		Units:    "cursors",
		Fam:      "cursors",
		Ctx:      "mongodb.cursors_open_no_timeout_count",
		Priority: prioCursorsOpenNoTimeoutCount,
		Dims: collectorapi.Dims{
			{ID: "metrics_cursor_open_no_timeout", Name: "open_no_timeout"},
		},
	}
	chartCursorsOpenedRate = collectorapi.Chart{
		ID:       "cursors_opened_rate",
		Title:    "Opened cursors rate",
		Units:    "cursors/s",
		Fam:      "cursors",
		Ctx:      "mongodb.cursors_opened_rate",
		Priority: prioCursorsOpenedRate,
		Dims: collectorapi.Dims{
			{ID: "metrics_cursor_total_opened", Name: "opened"},
		},
	}
	chartCursorsTimedOutRate = collectorapi.Chart{
		ID:       "cursors_timed_out_rate",
		Title:    "Timed-out cursors",
		Units:    "cursors/s",
		Fam:      "cursors",
		Ctx:      "mongodb.cursors_timed_out_rate",
		Priority: prioTimedOutCursorsRate,
		Dims: collectorapi.Dims{
			{ID: "metrics_cursor_timed_out", Name: "timed_out"},
		},
	}
	chartCursorsByLifespanCount = collectorapi.Chart{
		ID:       "cursors_by_lifespan_count",
		Title:    "Cursors lifespan",
		Units:    "cursors",
		Fam:      "cursors",
		Ctx:      "mongodb.cursors_by_lifespan_count",
		Priority: prioCursorsByLifespanCount,
		Type:     collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "metrics_cursor_lifespan_less_than_1_second", Name: "le_1s"},
			{ID: "metrics_cursor_lifespan_less_than_5_seconds", Name: "1s_5s"},
			{ID: "metrics_cursor_lifespan_less_than_15_seconds", Name: "5s_15s"},
			{ID: "metrics_cursor_lifespan_less_than_30_seconds", Name: "15s_30s"},
			{ID: "metrics_cursor_lifespan_less_than_1_minute", Name: "30s_1m"},
			{ID: "metrics_cursor_lifespan_less_than_10_minutes", Name: "1m_10m"},
			{ID: "metrics_cursor_lifespan_greater_than_or_equal_10_minutes", Name: "ge_10m"},
		},
	}

	chartWiredTigerConcurrentReadTransactionsUsage = collectorapi.Chart{
		ID:       "wiredtiger_concurrent_read_transactions_usage",
		Title:    "Wired Tiger concurrent read transactions usage",
		Units:    "transactions",
		Fam:      "wiredtiger",
		Ctx:      "mongodb.wiredtiger_concurrent_read_transactions_usage",
		Priority: prioWiredTigerConcurrentReadTransactionsUsage,
		Type:     collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "wiredtiger_concurrent_txn_read_available", Name: "available"},
			{ID: "wiredtiger_concurrent_txn_read_out", Name: "used"},
		},
	}
	chartWiredTigerConcurrentWriteTransactionsUsage = collectorapi.Chart{
		ID:       "wiredtiger_concurrent_write_transactions_usage",
		Title:    "Wired Tiger concurrent write transactions usage",
		Units:    "transactions",
		Fam:      "wiredtiger",
		Ctx:      "mongodb.wiredtiger_concurrent_write_transactions_usage",
		Priority: prioWiredTigerConcurrentWriteTransactionsUsage,
		Type:     collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "wiredtiger_concurrent_txn_write_available", Name: "available"},
			{ID: "wiredtiger_concurrent_txn_write_out", Name: "used"},
		},
	}
	chartWiredTigerCacheUsage = collectorapi.Chart{
		ID:       "wiredtiger_cache_usage",
		Title:    "Wired Tiger cache usage",
		Units:    "bytes",
		Fam:      "wiredtiger",
		Ctx:      "mongodb.wiredtiger_cache_usage",
		Priority: prioWiredTigerCacheUsage,
		Type:     collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "wiredtiger_cache_currently_in_cache_bytes", Name: "used"},
		},
	}
	chartWiredTigerCacheDirtySpaceSize = collectorapi.Chart{
		ID:       "wiredtiger_cache_dirty_space_size",
		Title:    "Wired Tiger cache dirty space size",
		Units:    "bytes",
		Fam:      "wiredtiger",
		Ctx:      "mongodb.wiredtiger_cache_dirty_space_size",
		Priority: prioWiredTigerCacheDirtySpaceSize,
		Dims: collectorapi.Dims{
			{ID: "wiredtiger_cache_tracked_dirty_in_the_cache_bytes", Name: "dirty"},
		},
	}
	chartWiredTigerCacheIORate = collectorapi.Chart{
		ID:       "wiredtiger_cache_io_rate",
		Title:    "Wired Tiger IO activity",
		Units:    "pages/s",
		Fam:      "wiredtiger",
		Ctx:      "mongodb.wiredtiger_cache_io_rate",
		Priority: prioWiredTigerCacheIORate,
		Dims: collectorapi.Dims{
			{ID: "wiredtiger_cache_read_into_cache_pages", Name: "read", Algo: collectorapi.Incremental},
			{ID: "wiredtiger_cache_written_from_cache_pages", Name: "written", Algo: collectorapi.Incremental},
		},
	}
	chartWiredTigerCacheEvictionsRate = collectorapi.Chart{
		ID:       "wiredtiger_cache_eviction_rate",
		Title:    "Wired Tiger cache evictions",
		Units:    "pages/s",
		Fam:      "wiredtiger",
		Ctx:      "mongodb.wiredtiger_cache_evictions_rate",
		Type:     collectorapi.Stacked,
		Priority: prioWiredTigerCacheEvictionsRate,
		Dims: collectorapi.Dims{
			{ID: "wiredtiger_cache_unmodified_evicted_pages", Name: "unmodified", Algo: collectorapi.Incremental},
			{ID: "wiredtiger_cache_modified_evicted_pages", Name: "modified", Algo: collectorapi.Incremental},
		},
	}
)

var (
	chartTmplDatabaseCollectionsCount = &collectorapi.Chart{
		ID:       chartPxDatabase + "%s_collections_count",
		Title:    "Database collections",
		Units:    "collections",
		Fam:      "databases",
		Ctx:      "mongodb.database_collections_count",
		Priority: prioDatabaseCollectionsCount,
		Dims: collectorapi.Dims{
			{ID: "database_%s_collections", Name: "collections"},
		},
	}
	chartTmplDatabaseIndexesCount = &collectorapi.Chart{
		ID:       chartPxDatabase + "%s_indexes_count",
		Title:    "Database indexes",
		Units:    "indexes",
		Fam:      "databases",
		Ctx:      "mongodb.database_indexes_count",
		Priority: prioDatabaseIndexesCount,
		Dims: collectorapi.Dims{
			{ID: "database_%s_indexes", Name: "indexes"},
		},
	}
	chartTmplDatabaseViewsCount = &collectorapi.Chart{
		ID:       chartPxDatabase + "%s_views_count",
		Title:    "Database views",
		Units:    "views",
		Fam:      "databases",
		Ctx:      "mongodb.database_views_count",
		Priority: prioDatabaseViewsCount,
		Dims: collectorapi.Dims{
			{ID: "database_%s_views", Name: "views"},
		},
	}
	chartTmplDatabaseDocumentsCount = &collectorapi.Chart{
		ID:       chartPxDatabase + "%s_documents_count",
		Title:    "Database documents",
		Units:    "documents",
		Fam:      "databases",
		Ctx:      "mongodb.database_documents_count",
		Priority: prioDatabaseDocumentsCount,
		Dims: collectorapi.Dims{
			{ID: "database_%s_documents", Name: "documents"},
		},
	}
	chartTmplDatabaseDataSize = &collectorapi.Chart{
		ID:       chartPxDatabase + "%s_data_size",
		Title:    "Database data size",
		Units:    "bytes",
		Fam:      "databases",
		Ctx:      "mongodb.database_data_size",
		Priority: prioDatabaseDataSize,
		Dims: collectorapi.Dims{
			{ID: "database_%s_data_size", Name: "data_size"},
		},
	}
	chartTmplDatabaseStorageSize = &collectorapi.Chart{
		ID:       chartPxDatabase + "%s_storage_size",
		Title:    "Database storage size",
		Units:    "bytes",
		Fam:      "databases",
		Ctx:      "mongodb.database_storage_size",
		Priority: prioDatabaseStorageSize,
		Dims: collectorapi.Dims{
			{ID: "database_%s_storage_size", Name: "storage_size"},
		},
	}
	chartTmplDatabaseIndexSize = &collectorapi.Chart{
		ID:       chartPxDatabase + "%s_index_size",
		Title:    "Database index size",
		Units:    "bytes",
		Fam:      "databases",
		Ctx:      "mongodb.database_index_size",
		Priority: prioDatabaseIndexSize,
		Dims: collectorapi.Dims{
			{ID: "database_%s_index_size", Name: "index_size"},
		},
	}
)

var (
	chartTmplReplSetMemberState = &collectorapi.Chart{
		ID:       chartPxReplSetMember + "%s_state",
		Title:    "Replica Set member state",
		Units:    "state",
		Fam:      "replica sets",
		Ctx:      "mongodb.repl_set_member_state",
		Priority: prioReplSetMemberState,
		Dims: collectorapi.Dims{
			{ID: "repl_set_member_%s_state_primary", Name: "primary"},
			{ID: "repl_set_member_%s_state_startup", Name: "startup"},
			{ID: "repl_set_member_%s_state_secondary", Name: "secondary"},
			{ID: "repl_set_member_%s_state_recovering", Name: "recovering"},
			{ID: "repl_set_member_%s_state_startup2", Name: "startup2"},
			{ID: "repl_set_member_%s_state_unknown", Name: "unknown"},
			{ID: "repl_set_member_%s_state_arbiter", Name: "arbiter"},
			{ID: "repl_set_member_%s_state_down", Name: "down"},
			{ID: "repl_set_member_%s_state_rollback", Name: "rollback"},
			{ID: "repl_set_member_%s_state_removed", Name: "removed"},
		},
	}
	chartTmplReplSetMemberHealthStatus = &collectorapi.Chart{
		ID:       chartPxReplSetMember + "%s_health_status",
		Title:    "Replica Set member health status",
		Units:    "status",
		Fam:      "replica sets",
		Ctx:      "mongodb.repl_set_member_health_status",
		Priority: prioReplSetMemberHealthStatus,
		Dims: collectorapi.Dims{
			{ID: "repl_set_member_%s_health_status_up", Name: "up"},
			{ID: "repl_set_member_%s_health_status_down", Name: "down"},
		},
	}
	chartTmplReplSetMemberReplicationLagTime = &collectorapi.Chart{
		ID:       chartPxReplSetMember + "%s_replication_lag_time",
		Title:    "Replica Set member replication lag",
		Units:    "milliseconds",
		Fam:      "replica sets",
		Ctx:      "mongodb.repl_set_member_replication_lag_time",
		Priority: prioReplSetMemberReplicationLagTime,
		Dims: collectorapi.Dims{
			{ID: "repl_set_member_%s_replication_lag", Name: "replication_lag"},
		},
	}
	chartTmplReplSetMemberHeartbeatLatencyTime = &collectorapi.Chart{
		ID:       chartPxReplSetMember + "%s_heartbeat_latency_time",
		Title:    "Replica Set member heartbeat latency",
		Units:    "milliseconds",
		Fam:      "replica sets",
		Ctx:      "mongodb.repl_set_member_heartbeat_latency_time",
		Priority: prioReplSetMemberHeartbeatLatencyTime,
		Dims: collectorapi.Dims{
			{ID: "repl_set_member_%s_heartbeat_latency", Name: "heartbeat_latency"},
		},
	}
	chartTmplReplSetMemberPingRTTTime = &collectorapi.Chart{
		ID:       chartPxReplSetMember + "%s_ping_rtt_time",
		Title:    "Replica Set member ping RTT",
		Units:    "milliseconds",
		Fam:      "replica sets",
		Ctx:      "mongodb.repl_set_member_ping_rtt_time",
		Priority: prioReplSetMemberPingRTTTime,
		Dims: collectorapi.Dims{
			{ID: "repl_set_member_%s_ping_rtt", Name: "ping_rtt"},
		},
	}
	chartTmplReplSetMemberUptime = &collectorapi.Chart{
		ID:       chartPxReplSetMember + "%s_uptime",
		Title:    "Replica Set member uptime",
		Units:    "seconds",
		Fam:      "replica sets",
		Ctx:      "mongodb.repl_set_member_uptime",
		Priority: prioReplSetMemberUptime,
		Dims: collectorapi.Dims{
			{ID: "repl_set_member_%s_uptime", Name: "uptime"},
		},
	}
)

var (
	chartShardingNodesCount = &collectorapi.Chart{
		ID:       "sharding_nodes_count",
		Title:    "Sharding Nodes",
		Units:    "nodes",
		Fam:      "sharding",
		Ctx:      "mongodb.sharding_nodes_count",
		Type:     collectorapi.Stacked,
		Priority: prioShardingNodesCount,
		Dims: collectorapi.Dims{
			{ID: "shard_nodes_aware", Name: "shard_aware"},
			{ID: "shard_nodes_unaware", Name: "shard_unaware"},
		},
	}
	chartShardingShardedDatabases = &collectorapi.Chart{
		ID:       "sharding_sharded_databases_count",
		Title:    "Sharded databases",
		Units:    "databases",
		Fam:      "sharding",
		Ctx:      "mongodb.sharding_sharded_databases_count",
		Type:     collectorapi.Stacked,
		Priority: prioShardingShardedDatabasesCount,
		Dims: collectorapi.Dims{
			{ID: "shard_databases_partitioned", Name: "partitioned"},
			{ID: "shard_databases_unpartitioned", Name: "unpartitioned"},
		},
	}

	chartShardingShardedCollectionsCount = &collectorapi.Chart{
		ID:       "sharding_sharded_collections_count",
		Title:    "Sharded collections",
		Units:    "collections",
		Fam:      "sharding",
		Ctx:      "mongodb.sharding_sharded_collections_count",
		Type:     collectorapi.Stacked,
		Priority: prioShardingShardedCollectionsCount,
		Dims: collectorapi.Dims{
			{ID: "shard_collections_partitioned", Name: "partitioned"},
			{ID: "shard_collections_unpartitioned", Name: "unpartitioned"},
		},
	}

	chartTmplShardChunks = &collectorapi.Chart{
		ID:       chartPxShard + "%s_chunks",
		Title:    "Shard chunks",
		Units:    "chunks",
		Fam:      "sharding",
		Ctx:      "mongodb.sharding_shard_chunks_count",
		Priority: prioShardChunks,
		Dims: collectorapi.Dims{
			{ID: "shard_id_%s_chunks", Name: "chunks"},
		},
	}
)
