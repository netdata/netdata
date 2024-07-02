// SPDX-License-Identifier: GPL-3.0-or-later

package mongo

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioOperationsRate = module.Priority + iota
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
var chartsServerStatus = module.Charts{
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

var chartsTmplDatabase = module.Charts{
	chartTmplDatabaseCollectionsCount.Copy(),
	chartTmplDatabaseIndexesCount.Copy(),
	chartTmplDatabaseViewsCount.Copy(),
	chartTmplDatabaseDocumentsCount.Copy(),
	chartTmplDatabaseDataSize.Copy(),
	chartTmplDatabaseStorageSize.Copy(),
	chartTmplDatabaseIndexSize.Copy(),
}

var chartsTmplReplSetMember = module.Charts{
	chartTmplReplSetMemberState.Copy(),
	chartTmplReplSetMemberHealthStatus.Copy(),
	chartTmplReplSetMemberReplicationLagTime.Copy(),
	chartTmplReplSetMemberHeartbeatLatencyTime.Copy(),
	chartTmplReplSetMemberPingRTTTime.Copy(),
	chartTmplReplSetMemberUptime.Copy(),
}

var chartsSharding = module.Charts{
	chartShardingNodesCount.Copy(),
	chartShardingShardedDatabases.Copy(),
	chartShardingShardedCollectionsCount.Copy(),
}

var chartsTmplShardingShard = module.Charts{
	chartTmplShardChunks.Copy(),
}

var (
	chartOperationsRate = module.Chart{
		ID:       "operations_rate",
		Title:    "Operations rate",
		Units:    "operations/s",
		Fam:      "operations",
		Ctx:      "mongodb.operations_rate",
		Priority: prioOperationsRate,
		Dims: module.Dims{
			{ID: "operations_latencies_reads_ops", Name: "reads", Algo: module.Incremental},
			{ID: "operations_latencies_writes_ops", Name: "writes", Algo: module.Incremental},
			{ID: "operations_latencies_commands_ops", Name: "commands", Algo: module.Incremental},
		},
	}
	chartOperationsLatencyTime = module.Chart{
		ID:       "operations_latency_time",
		Title:    "Operations Latency",
		Units:    "milliseconds",
		Fam:      "operations",
		Ctx:      "mongodb.operations_latency_time",
		Priority: prioOperationsLatencyTime,
		Dims: module.Dims{
			{ID: "operations_latencies_reads_latency", Name: "reads", Algo: module.Incremental, Div: 1000},
			{ID: "operations_latencies_writes_latency", Name: "writes", Algo: module.Incremental, Div: 1000},
			{ID: "operations_latencies_commands_latency", Name: "commands", Algo: module.Incremental, Div: 1000},
		},
	}
	chartOperationsByTypeRate = module.Chart{
		ID:       "operations_by_type_rate",
		Title:    "Operations by type",
		Units:    "operations/s",
		Fam:      "operations",
		Ctx:      "mongodb.operations_by_type_rate",
		Priority: prioOperationsByTypeRate,
		Dims: module.Dims{
			{ID: "operations_insert", Name: "insert", Algo: module.Incremental},
			{ID: "operations_query", Name: "query", Algo: module.Incremental},
			{ID: "operations_update", Name: "update", Algo: module.Incremental},
			{ID: "operations_delete", Name: "delete", Algo: module.Incremental},
			{ID: "operations_getmore", Name: "getmore", Algo: module.Incremental},
			{ID: "operations_command", Name: "command", Algo: module.Incremental},
		},
	}
	chartDocumentOperationsRate = module.Chart{
		ID:       "document_operations_rate",
		Title:    "Document operations",
		Units:    "operations/s",
		Fam:      "operations",
		Ctx:      "mongodb.document_operations_rate",
		Type:     module.Stacked,
		Priority: prioDocumentOperationsRate,
		Dims: module.Dims{
			{ID: "metrics_document_inserted", Name: "inserted", Algo: module.Incremental},
			{ID: "metrics_document_deleted", Name: "deleted", Algo: module.Incremental},
			{ID: "metrics_document_returned", Name: "returned", Algo: module.Incremental},
			{ID: "metrics_document_updated", Name: "updated", Algo: module.Incremental},
		},
	}
	chartScannedIndexesRate = module.Chart{
		ID:       "scanned_indexes_rate",
		Title:    "Scanned indexes",
		Units:    "indexes/s",
		Fam:      "operations",
		Ctx:      "mongodb.scanned_indexes_rate",
		Priority: prioScannedIndexesRate,
		Dims: module.Dims{
			{ID: "metrics_query_executor_scanned", Name: "scanned", Algo: module.Incremental},
		},
	}
	chartScannedDocumentsRate = module.Chart{
		ID:       "scanned_documents_rate",
		Title:    "Scanned documents",
		Units:    "documents/s",
		Fam:      "operations",
		Ctx:      "mongodb.scanned_documents_rate",
		Priority: prioScannedDocumentsRate,
		Dims: module.Dims{
			{ID: "metrics_query_executor_scanned_objects", Name: "scanned", Algo: module.Incremental},
		},
	}

	chartGlobalLockActiveClientsCount = module.Chart{
		ID:       "active_clients_count",
		Title:    "Connected clients",
		Units:    "clients",
		Fam:      "clients",
		Ctx:      "mongodb.active_clients_count",
		Priority: prioActiveClientsCount,
		Dims: module.Dims{
			{ID: "global_lock_active_clients_readers", Name: "readers"},
			{ID: "global_lock_active_clients_writers", Name: "writers"},
		},
	}
	chartGlobalLockCurrentQueueCount = module.Chart{
		ID:       "queued_operations",
		Title:    "Queued operations because of a lock",
		Units:    "operations",
		Fam:      "clients",
		Ctx:      "mongodb.queued_operations_count",
		Priority: prioQueuedOperationsCount,
		Dims: module.Dims{
			{ID: "global_lock_current_queue_readers", Name: "readers"},
			{ID: "global_lock_current_queue_writers", Name: "writers"},
		},
	}

	chartConnectionsUsage = module.Chart{
		ID:       "connections_usage",
		Title:    "Connections usage",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "mongodb.connections_usage",
		Type:     module.Stacked,
		Priority: prioConnectionsUsage,
		Dims: module.Dims{
			{ID: "connections_available", Name: "available"},
			{ID: "connections_current", Name: "used"},
		},
	}
	chartConnectionsByStateCount = module.Chart{
		ID:       "connections_by_state_count",
		Title:    "Connections By State",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "mongodb.connections_by_state_count",
		Priority: prioConnectionsByStateCount,
		Dims: module.Dims{
			{ID: "connections_active", Name: "active"},
			{ID: "connections_threaded", Name: "threaded"},
			{ID: "connections_exhaust_is_master", Name: "exhaust_is_master"},
			{ID: "connections_exhaust_hello", Name: "exhaust_hello"},
			{ID: "connections_awaiting_topology_changes", Name: "awaiting_topology_changes"},
		},
	}
	chartConnectionsRate = module.Chart{
		ID:       "connections_rate",
		Title:    "Connections Rate",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "mongodb.connections_rate",
		Priority: prioConnectionsRate,
		Dims: module.Dims{
			{ID: "connections_total_created", Name: "created", Algo: module.Incremental},
		},
	}

	chartNetworkTrafficRate = module.Chart{
		ID:       "network_traffic",
		Title:    "Network traffic",
		Units:    "bytes/s",
		Fam:      "network",
		Ctx:      "mongodb.network_traffic_rate",
		Priority: prioNetworkTrafficRate,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "network_bytes_in", Name: "in", Algo: module.Incremental},
			{ID: "network_bytes_out", Name: "out", Algo: module.Incremental},
		},
	}
	chartNetworkRequestsRate = module.Chart{
		ID:       "network_requests_rate",
		Title:    "Network Requests",
		Units:    "requests/s",
		Fam:      "network",
		Ctx:      "mongodb.network_requests_rate",
		Priority: prioNetworkRequestsRate,
		Dims: module.Dims{
			{ID: "network_requests", Name: "requests", Algo: module.Incremental},
		},
	}
	chartNetworkSlowDNSResolutionsRate = module.Chart{
		ID:       "network_slow_dns_resolutions_rate",
		Title:    "Slow DNS resolution operations",
		Units:    "resolutions/s",
		Fam:      "network",
		Ctx:      "mongodb.network_slow_dns_resolutions_rate",
		Priority: prioNetworkSlowDNSResolutionsRate,
		Dims: module.Dims{
			{ID: "network_slow_dns_operations", Name: "slow_dns", Algo: module.Incremental},
		},
	}
	chartNetworkSlowSSLHandshakesRate = module.Chart{
		ID:       "network_slow_ssl_handshakes_rate",
		Title:    "Slow SSL handshake operations",
		Units:    "handshakes/s",
		Fam:      "network",
		Ctx:      "mongodb.network_slow_ssl_handshakes_rate",
		Priority: prioNetworkSlowSSLHandshakesRate,
		Dims: module.Dims{
			{ID: "network_slow_ssl_operations", Name: "slow_ssl", Algo: module.Incremental},
		},
	}

	chartMemoryResidentSize = module.Chart{
		ID:       "memory_resident_size",
		Title:    "Used resident memory",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "mongodb.memory_resident_size",
		Priority: prioMemoryResidentSize,
		Dims: module.Dims{
			{ID: "memory_resident", Name: "used"},
		},
	}
	chartMemoryVirtualSize = module.Chart{
		ID:       "memory_virtual_size",
		Title:    "Used virtual memory",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "mongodb.memory_virtual_size",
		Priority: prioMemoryVirtualSize,
		Dims: module.Dims{
			{ID: "memory_virtual", Name: "used"},
		},
	}
	chartMemoryPageFaultsRate = module.Chart{
		ID:       "memory_page_faults",
		Title:    "Memory page faults",
		Units:    "pgfaults/s",
		Fam:      "memory",
		Ctx:      "mongodb.memory_page_faults_rate",
		Priority: prioMemoryPageFaultsRate,
		Dims: module.Dims{
			{ID: "extra_info_page_faults", Name: "pgfaults", Algo: module.Incremental},
		},
	}
	chartMemoryTCMallocStatsChart = module.Chart{
		ID:       "memory_tcmalloc_stats",
		Title:    "TCMalloc statistics",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "mongodb.memory_tcmalloc_stats",
		Priority: prioMemoryTCMallocStats,
		Dims: module.Dims{
			{ID: "tcmalloc_generic_current_allocated_bytes", Name: "allocated"},
			{ID: "tcmalloc_central_cache_free_bytes", Name: "central_cache_freelist"},
			{ID: "tcmalloc_transfer_cache_free_bytes", Name: "transfer_cache_freelist"},
			{ID: "tcmalloc_thread_cache_free_bytes", Name: "thread_cache_freelists"},
			{ID: "tcmalloc_pageheap_free_bytes", Name: "pageheap_freelist"},
			{ID: "tcmalloc_pageheap_unmapped_bytes", Name: "pageheap_unmapped"},
		},
	}

	chartAssertsRate = module.Chart{
		ID:       "asserts_rate",
		Title:    "Raised assertions",
		Units:    "asserts/s",
		Fam:      "asserts",
		Ctx:      "mongodb.asserts_rate",
		Type:     module.Stacked,
		Priority: prioAssertsRate,
		Dims: module.Dims{
			{ID: "asserts_regular", Name: "regular", Algo: module.Incremental},
			{ID: "asserts_warning", Name: "warning", Algo: module.Incremental},
			{ID: "asserts_msg", Name: "msg", Algo: module.Incremental},
			{ID: "asserts_user", Name: "user", Algo: module.Incremental},
			{ID: "asserts_tripwire", Name: "tripwire", Algo: module.Incremental},
			{ID: "asserts_rollovers", Name: "rollovers", Algo: module.Incremental},
		},
	}

	chartTransactionsCount = module.Chart{
		ID:       "transactions_count",
		Title:    "Current transactions",
		Units:    "transactions",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_count",
		Priority: prioTransactionsCount,
		Dims: module.Dims{
			{ID: "txn_active", Name: "active"},
			{ID: "txn_inactive", Name: "inactive"},
			{ID: "txn_open", Name: "open"},
			{ID: "txn_prepared", Name: "prepared"},
		},
	}
	chartTransactionsRate = module.Chart{
		ID:       "transactions_rate",
		Title:    "Transactions rate",
		Units:    "transactions/s",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_rate",
		Priority: prioTransactionsRate,
		Dims: module.Dims{
			{ID: "txn_total_started", Name: "started", Algo: module.Incremental},
			{ID: "txn_total_aborted", Name: "aborted", Algo: module.Incremental},
			{ID: "txn_total_committed", Name: "committed", Algo: module.Incremental},
			{ID: "txn_total_prepared", Name: "prepared", Algo: module.Incremental},
		},
	}
	chartTransactionsNoShardsCommitsRate = module.Chart{
		ID:       "transactions_no_shards_commits_rate",
		Title:    "Transactions commits",
		Units:    "commits/s",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_rate",
		Priority: prioTransactionsNoShardsCommitsRate,
		Type:     module.Stacked,
		Labels:   []module.Label{{Key: "commit_type", Value: "noShards"}},
		Dims: module.Dims{
			{ID: "txn_commit_types_no_shards_successful", Name: "success", Algo: module.Incremental},
			{ID: "txn_commit_types_no_shards_unsuccessful", Name: "fail", Algo: module.Incremental},
		},
	}
	chartTransactionsNoShardsCommitsDurationTime = module.Chart{
		ID:       "transactions_no_shards_commits_duration_time",
		Title:    "Transactions successful commits duration",
		Units:    "milliseconds",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_duration_time",
		Priority: prioTransactionsNoShardsCommitsDurationTime,
		Labels:   []module.Label{{Key: "commit_type", Value: "noShards"}},
		Dims: module.Dims{
			{ID: "txn_commit_types_no_shards_successful_duration_micros", Name: "commits", Algo: module.Incremental, Div: 1000},
		},
	}
	chartTransactionsSingleShardCommitsRate = module.Chart{
		ID:       "transactions_single_shard_commits_rate",
		Title:    "Transactions commits",
		Units:    "commits/s",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_rate",
		Priority: prioTransactionsSingleShardCommitsRate,
		Type:     module.Stacked,
		Labels:   []module.Label{{Key: "commit_type", Value: "singleShard"}},
		Dims: module.Dims{
			{ID: "txn_commit_types_single_shard_successful", Name: "success", Algo: module.Incremental},
			{ID: "txn_commit_types_single_shard_unsuccessful", Name: "fail", Algo: module.Incremental},
		},
	}
	chartTransactionsSingleShardCommitsDurationTime = module.Chart{
		ID:       "transactions_single_shard_commits_duration_time",
		Title:    "Transactions successful commits duration",
		Units:    "milliseconds",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_duration_time",
		Priority: prioTransactionsSingleShardCommitsDurationTime,
		Labels:   []module.Label{{Key: "commit_type", Value: "singleShard"}},
		Dims: module.Dims{
			{ID: "txn_commit_types_single_shard_successful_duration_micros", Name: "commits", Algo: module.Incremental, Div: 1000},
		},
	}
	chartTransactionsSingleWriteShardCommitsRate = module.Chart{
		ID:       "transactions_single_write_shard_commits_rate",
		Title:    "Transactions commits",
		Units:    "commits/s",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_rate",
		Priority: prioTransactionsSingleWriteShardCommitsRate,
		Type:     module.Stacked,
		Labels:   []module.Label{{Key: "commit_type", Value: "singleWriteShard"}},
		Dims: module.Dims{
			{ID: "txn_commit_types_single_write_shard_successful", Name: "success", Algo: module.Incremental},
			{ID: "txn_commit_types_single_write_shard_unsuccessful", Name: "fail", Algo: module.Incremental},
		},
	}
	chartTransactionsSingleWriteShardCommitsDurationTime = module.Chart{
		ID:       "transactions_single_write_shard_commits_duration_time",
		Title:    "Transactions successful commits duration",
		Units:    "milliseconds",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_duration_time",
		Priority: prioTransactionsSingleWriteShardCommitsDurationTime,
		Labels:   []module.Label{{Key: "commit_type", Value: "singleWriteShard"}},
		Dims: module.Dims{
			{ID: "txn_commit_types_single_write_shard_successful_duration_micros", Name: "commits", Algo: module.Incremental, Div: 1000},
		},
	}
	chartTransactionsReadOnlyCommitsRate = module.Chart{
		ID:       "transactions_read_only_commits_rate",
		Title:    "Transactions commits",
		Units:    "commits/s",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_rate",
		Priority: prioTransactionsReadOnlyCommitsRate,
		Type:     module.Stacked,
		Labels:   []module.Label{{Key: "commit_type", Value: "readOnly"}},
		Dims: module.Dims{
			{ID: "txn_commit_types_read_only_successful", Name: "success", Algo: module.Incremental},
			{ID: "txn_commit_types_read_only_unsuccessful", Name: "fail", Algo: module.Incremental},
		},
	}
	chartTransactionsReadOnlyCommitsDurationTime = module.Chart{
		ID:       "transactions_read_only_commits_duration_time",
		Title:    "Transactions successful commits duration",
		Units:    "milliseconds",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_duration_time",
		Priority: prioTransactionsReadOnlyCommitsDurationTime,
		Labels:   []module.Label{{Key: "commit_type", Value: "readOnly"}},
		Dims: module.Dims{
			{ID: "txn_commit_types_read_only_successful_duration_micros", Name: "commits", Algo: module.Incremental, Div: 1000},
		},
	}
	chartTransactionsTwoPhaseCommitCommitsRate = module.Chart{
		ID:       "transactions_two_phase_commit_commits_rate",
		Title:    "Transactions commits",
		Units:    "commits/s",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_rate",
		Priority: prioTransactionsTwoPhaseCommitCommitsRate,
		Type:     module.Stacked,
		Labels:   []module.Label{{Key: "commit_type", Value: "twoPhaseCommit"}},
		Dims: module.Dims{
			{ID: "txn_commit_types_two_phase_commit_successful", Name: "success", Algo: module.Incremental},
			{ID: "txn_commit_types_two_phase_commit_unsuccessful", Name: "fail", Algo: module.Incremental},
		},
	}
	chartTransactionsTwoPhaseCommitCommitsDurationTime = module.Chart{
		ID:       "transactions_two_phase_commit_commits_duration_time",
		Title:    "Transactions successful commits duration",
		Units:    "milliseconds",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_duration_time",
		Priority: prioTransactionsTwoPhaseCommitCommitsDurationTime,
		Labels:   []module.Label{{Key: "commit_type", Value: "twoPhaseCommit"}},
		Dims: module.Dims{
			{ID: "txn_commit_types_two_phase_commit_successful_duration_micros", Name: "commits", Algo: module.Incremental, Div: 1000},
		},
	}
	chartTransactionsRecoverWithTokenCommitsRate = module.Chart{
		ID:       "transactions_recover_with_token_commits_rate",
		Title:    "Transactions commits",
		Units:    "commits/s",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_rate",
		Priority: prioTransactionsRecoverWithTokenCommitsRate,
		Type:     module.Stacked,
		Labels:   []module.Label{{Key: "commit_type", Value: "recoverWithToken"}},
		Dims: module.Dims{
			{ID: "txn_commit_types_recover_with_token_successful", Name: "success", Algo: module.Incremental},
			{ID: "txn_commit_types_recover_with_token_unsuccessful", Name: "fail", Algo: module.Incremental},
		},
	}
	chartTransactionsRecoverWithTokenCommitsDurationTime = module.Chart{
		ID:       "transactions_recover_with_token_commits_duration_time",
		Title:    "Transactions successful commits duration",
		Units:    "milliseconds",
		Fam:      "transactions",
		Ctx:      "mongodb.transactions_commits_duration_time",
		Priority: prioTransactionsRecoverWithTokenCommitsDurationTime,
		Labels:   []module.Label{{Key: "commit_type", Value: "recoverWithToken"}},
		Dims: module.Dims{
			{ID: "txn_commit_types_recover_with_token_successful_duration_micros", Name: "commits", Algo: module.Incremental, Div: 1000},
		},
	}

	chartGlobalLockAcquisitionsRate = module.Chart{
		ID:       "global_lock_acquisitions_rate",
		Title:    "Global lock acquisitions",
		Units:    "acquisitions/s",
		Fam:      "locks",
		Ctx:      "mongodb.lock_acquisitions_rate",
		Priority: prioGlobalLockAcquisitionsRate,
		Labels:   []module.Label{{Key: "lock_type", Value: "global"}},
		Dims: module.Dims{
			{ID: "locks_global_acquire_shared", Name: "shared", Algo: module.Incremental},
			{ID: "locks_global_acquire_exclusive", Name: "exclusive", Algo: module.Incremental},
			{ID: "locks_global_acquire_intent_shared", Name: "intent_shared", Algo: module.Incremental},
			{ID: "locks_global_acquire_intent_exclusive", Name: "intent_exclusive", Algo: module.Incremental},
		},
	}
	chartDatabaseLockAcquisitionsRate = module.Chart{
		ID:       "database_lock_acquisitions_rate",
		Title:    "Database lock acquisitions",
		Units:    "acquisitions/s",
		Fam:      "locks",
		Ctx:      "mongodb.lock_acquisitions_rate",
		Priority: prioDatabaseLockAcquisitionsRate,
		Labels:   []module.Label{{Key: "lock_type", Value: "database"}},
		Dims: module.Dims{
			{ID: "locks_database_acquire_shared", Name: "shared", Algo: module.Incremental},
			{ID: "locks_database_acquire_exclusive", Name: "exclusive", Algo: module.Incremental},
			{ID: "locks_database_acquire_intent_shared", Name: "intent_shared", Algo: module.Incremental},
			{ID: "locks_database_acquire_intent_exclusive", Name: "intent_exclusive", Algo: module.Incremental},
		},
	}
	chartCollectionLockAcquisitionsRate = module.Chart{
		ID:       "collection_lock_acquisitions_rate",
		Title:    "Collection lock acquisitions",
		Units:    "acquisitions/s",
		Fam:      "locks",
		Ctx:      "mongodb.lock_acquisitions_rate",
		Priority: prioCollectionLockAcquisitionsRate,
		Labels:   []module.Label{{Key: "lock_type", Value: "collection"}},
		Dims: module.Dims{
			{ID: "locks_collection_acquire_shared", Name: "shared", Algo: module.Incremental},
			{ID: "locks_collection_acquire_exclusive", Name: "exclusive", Algo: module.Incremental},
			{ID: "locks_collection_acquire_intent_shared", Name: "intent_shared", Algo: module.Incremental},
			{ID: "locks_collection_acquire_intent_exclusive", Name: "intent_exclusive", Algo: module.Incremental},
		},
	}
	chartMutexLockAcquisitionsRate = module.Chart{
		ID:       "mutex_lock_acquisitions_rate",
		Title:    "Mutex lock acquisitions",
		Units:    "acquisitions/s",
		Fam:      "locks",
		Ctx:      "mongodb.lock_acquisitions_rate",
		Priority: prioMutexLockAcquisitionsRate,
		Labels:   []module.Label{{Key: "lock_type", Value: "mutex"}},
		Dims: module.Dims{
			{ID: "locks_mutex_acquire_shared", Name: "shared", Algo: module.Incremental},
			{ID: "locks_mutex_acquire_exclusive", Name: "exclusive", Algo: module.Incremental},
			{ID: "locks_mutex_acquire_intent_shared", Name: "intent_shared", Algo: module.Incremental},
			{ID: "locks_mutex_acquire_intent_exclusive", Name: "intent_exclusive", Algo: module.Incremental},
		},
	}
	chartMetadataLockAcquisitionsRate = module.Chart{
		ID:       "metadata_lock_acquisitions_rate",
		Title:    "Metadata lock acquisitions",
		Units:    "acquisitions/s",
		Fam:      "locks",
		Ctx:      "mongodb.lock_acquisitions_rate",
		Priority: prioMetadataLockAcquisitionsRate,
		Labels:   []module.Label{{Key: "lock_type", Value: "metadata"}},
		Dims: module.Dims{
			{ID: "locks_metadata_acquire_shared", Name: "shared", Algo: module.Incremental},
			{ID: "locks_metadata_acquire_exclusive", Name: "exclusive", Algo: module.Incremental},
			{ID: "locks_metadata_acquire_intent_shared", Name: "intent_shared", Algo: module.Incremental},
			{ID: "locks_metadata_acquire_intent_exclusive", Name: "intent_exclusive", Algo: module.Incremental},
		},
	}
	chartOpLogLockAcquisitionsRate = module.Chart{
		ID:       "oplog_lock_acquisitions_rate",
		Title:    "Operations log lock acquisitions",
		Units:    "acquisitions/s",
		Fam:      "locks",
		Ctx:      "mongodb.lock_acquisitions_rate",
		Priority: prioOpLogLockAcquisitionsRate,
		Labels:   []module.Label{{Key: "lock_type", Value: "oplog"}},
		Dims: module.Dims{
			{ID: "locks_oplog_acquire_shared", Name: "shared", Algo: module.Incremental},
			{ID: "locks_oplog_acquire_exclusive", Name: "exclusive", Algo: module.Incremental},
			{ID: "locks_oplog_acquire_intent_shared", Name: "intent_shared", Algo: module.Incremental},
			{ID: "locks_oplog_acquire_intent_exclusive", Name: "intent_exclusive", Algo: module.Incremental},
		},
	}

	chartCursorsOpenCount = module.Chart{
		ID:       "cursors_open_count",
		Title:    "Open cursors",
		Units:    "cursors",
		Fam:      "cursors",
		Ctx:      "mongodb.cursors_open_count",
		Priority: prioCursorsOpenCount,
		Dims: module.Dims{
			{ID: "metrics_cursor_open_total", Name: "open"},
		},
	}
	chartCursorsOpenNoTimeoutCount = module.Chart{
		ID:       "cursors_open_no_timeout_count",
		Title:    "Open cursors with disabled timeout",
		Units:    "cursors",
		Fam:      "cursors",
		Ctx:      "mongodb.cursors_open_no_timeout_count",
		Priority: prioCursorsOpenNoTimeoutCount,
		Dims: module.Dims{
			{ID: "metrics_cursor_open_no_timeout", Name: "open_no_timeout"},
		},
	}
	chartCursorsOpenedRate = module.Chart{
		ID:       "cursors_opened_rate",
		Title:    "Opened cursors rate",
		Units:    "cursors/s",
		Fam:      "cursors",
		Ctx:      "mongodb.cursors_opened_rate",
		Priority: prioCursorsOpenedRate,
		Dims: module.Dims{
			{ID: "metrics_cursor_total_opened", Name: "opened"},
		},
	}
	chartCursorsTimedOutRate = module.Chart{
		ID:       "cursors_timed_out_rate",
		Title:    "Timed-out cursors",
		Units:    "cursors/s",
		Fam:      "cursors",
		Ctx:      "mongodb.cursors_timed_out_rate",
		Priority: prioTimedOutCursorsRate,
		Dims: module.Dims{
			{ID: "metrics_cursor_timed_out", Name: "timed_out"},
		},
	}
	chartCursorsByLifespanCount = module.Chart{
		ID:       "cursors_by_lifespan_count",
		Title:    "Cursors lifespan",
		Units:    "cursors",
		Fam:      "cursors",
		Ctx:      "mongodb.cursors_by_lifespan_count",
		Priority: prioCursorsByLifespanCount,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "metrics_cursor_lifespan_less_than_1_second", Name: "le_1s"},
			{ID: "metrics_cursor_lifespan_less_than_5_seconds", Name: "1s_5s"},
			{ID: "metrics_cursor_lifespan_less_than_15_seconds", Name: "5s_15s"},
			{ID: "metrics_cursor_lifespan_less_than_30_seconds", Name: "15s_30s"},
			{ID: "metrics_cursor_lifespan_less_than_1_minute", Name: "30s_1m"},
			{ID: "metrics_cursor_lifespan_less_than_10_minutes", Name: "1m_10m"},
			{ID: "metrics_cursor_lifespan_greater_than_or_equal_10_minutes", Name: "ge_10m"},
		},
	}

	chartWiredTigerConcurrentReadTransactionsUsage = module.Chart{
		ID:       "wiredtiger_concurrent_read_transactions_usage",
		Title:    "Wired Tiger concurrent read transactions usage",
		Units:    "transactions",
		Fam:      "wiredtiger",
		Ctx:      "mongodb.wiredtiger_concurrent_read_transactions_usage",
		Priority: prioWiredTigerConcurrentReadTransactionsUsage,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "wiredtiger_concurrent_txn_read_available", Name: "available"},
			{ID: "wiredtiger_concurrent_txn_read_out", Name: "used"},
		},
	}
	chartWiredTigerConcurrentWriteTransactionsUsage = module.Chart{
		ID:       "wiredtiger_concurrent_write_transactions_usage",
		Title:    "Wired Tiger concurrent write transactions usage",
		Units:    "transactions",
		Fam:      "wiredtiger",
		Ctx:      "mongodb.wiredtiger_concurrent_write_transactions_usage",
		Priority: prioWiredTigerConcurrentWriteTransactionsUsage,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "wiredtiger_concurrent_txn_write_available", Name: "available"},
			{ID: "wiredtiger_concurrent_txn_write_out", Name: "used"},
		},
	}
	chartWiredTigerCacheUsage = module.Chart{
		ID:       "wiredtiger_cache_usage",
		Title:    "Wired Tiger cache usage",
		Units:    "bytes",
		Fam:      "wiredtiger",
		Ctx:      "mongodb.wiredtiger_cache_usage",
		Priority: prioWiredTigerCacheUsage,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "wiredtiger_cache_currently_in_cache_bytes", Name: "used"},
		},
	}
	chartWiredTigerCacheDirtySpaceSize = module.Chart{
		ID:       "wiredtiger_cache_dirty_space_size",
		Title:    "Wired Tiger cache dirty space size",
		Units:    "bytes",
		Fam:      "wiredtiger",
		Ctx:      "mongodb.wiredtiger_cache_dirty_space_size",
		Priority: prioWiredTigerCacheDirtySpaceSize,
		Dims: module.Dims{
			{ID: "wiredtiger_cache_tracked_dirty_in_the_cache_bytes", Name: "dirty"},
		},
	}
	chartWiredTigerCacheIORate = module.Chart{
		ID:       "wiredtiger_cache_io_rate",
		Title:    "Wired Tiger IO activity",
		Units:    "pages/s",
		Fam:      "wiredtiger",
		Ctx:      "mongodb.wiredtiger_cache_io_rate",
		Priority: prioWiredTigerCacheIORate,
		Dims: module.Dims{
			{ID: "wiredtiger_cache_read_into_cache_pages", Name: "read", Algo: module.Incremental},
			{ID: "wiredtiger_cache_written_from_cache_pages", Name: "written", Algo: module.Incremental},
		},
	}
	chartWiredTigerCacheEvictionsRate = module.Chart{
		ID:       "wiredtiger_cache_eviction_rate",
		Title:    "Wired Tiger cache evictions",
		Units:    "pages/s",
		Fam:      "wiredtiger",
		Ctx:      "mongodb.wiredtiger_cache_evictions_rate",
		Type:     module.Stacked,
		Priority: prioWiredTigerCacheEvictionsRate,
		Dims: module.Dims{
			{ID: "wiredtiger_cache_unmodified_evicted_pages", Name: "unmodified", Algo: module.Incremental},
			{ID: "wiredtiger_cache_modified_evicted_pages", Name: "modified", Algo: module.Incremental},
		},
	}
)

var (
	chartTmplDatabaseCollectionsCount = &module.Chart{
		ID:       chartPxDatabase + "%s_collections_count",
		Title:    "Database collections",
		Units:    "collections",
		Fam:      "databases",
		Ctx:      "mongodb.database_collections_count",
		Priority: prioDatabaseCollectionsCount,
		Dims: module.Dims{
			{ID: "database_%s_collections", Name: "collections"},
		},
	}
	chartTmplDatabaseIndexesCount = &module.Chart{
		ID:       chartPxDatabase + "%s_indexes_count",
		Title:    "Database indexes",
		Units:    "indexes",
		Fam:      "databases",
		Ctx:      "mongodb.database_indexes_count",
		Priority: prioDatabaseIndexesCount,
		Dims: module.Dims{
			{ID: "database_%s_indexes", Name: "indexes"},
		},
	}
	chartTmplDatabaseViewsCount = &module.Chart{
		ID:       chartPxDatabase + "%s_views_count",
		Title:    "Database views",
		Units:    "views",
		Fam:      "databases",
		Ctx:      "mongodb.database_views_count",
		Priority: prioDatabaseViewsCount,
		Dims: module.Dims{
			{ID: "database_%s_views", Name: "views"},
		},
	}
	chartTmplDatabaseDocumentsCount = &module.Chart{
		ID:       chartPxDatabase + "%s_documents_count",
		Title:    "Database documents",
		Units:    "documents",
		Fam:      "databases",
		Ctx:      "mongodb.database_documents_count",
		Priority: prioDatabaseDocumentsCount,
		Dims: module.Dims{
			{ID: "database_%s_documents", Name: "documents"},
		},
	}
	chartTmplDatabaseDataSize = &module.Chart{
		ID:       chartPxDatabase + "%s_data_size",
		Title:    "Database data size",
		Units:    "bytes",
		Fam:      "databases",
		Ctx:      "mongodb.database_data_size",
		Priority: prioDatabaseDataSize,
		Dims: module.Dims{
			{ID: "database_%s_data_size", Name: "data_size"},
		},
	}
	chartTmplDatabaseStorageSize = &module.Chart{
		ID:       chartPxDatabase + "%s_storage_size",
		Title:    "Database storage size",
		Units:    "bytes",
		Fam:      "databases",
		Ctx:      "mongodb.database_storage_size",
		Priority: prioDatabaseStorageSize,
		Dims: module.Dims{
			{ID: "database_%s_storage_size", Name: "storage_size"},
		},
	}
	chartTmplDatabaseIndexSize = &module.Chart{
		ID:       chartPxDatabase + "%s_index_size",
		Title:    "Database index size",
		Units:    "bytes",
		Fam:      "databases",
		Ctx:      "mongodb.database_index_size",
		Priority: prioDatabaseIndexSize,
		Dims: module.Dims{
			{ID: "database_%s_index_size", Name: "index_size"},
		},
	}
)

var (
	chartTmplReplSetMemberState = &module.Chart{
		ID:       chartPxReplSetMember + "%s_state",
		Title:    "Replica Set member state",
		Units:    "state",
		Fam:      "replica sets",
		Ctx:      "mongodb.repl_set_member_state",
		Priority: prioReplSetMemberState,
		Dims: module.Dims{
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
	chartTmplReplSetMemberHealthStatus = &module.Chart{
		ID:       chartPxReplSetMember + "%s_health_status",
		Title:    "Replica Set member health status",
		Units:    "status",
		Fam:      "replica sets",
		Ctx:      "mongodb.repl_set_member_health_status",
		Priority: prioReplSetMemberHealthStatus,
		Dims: module.Dims{
			{ID: "repl_set_member_%s_health_status_up", Name: "up"},
			{ID: "repl_set_member_%s_health_status_down", Name: "down"},
		},
	}
	chartTmplReplSetMemberReplicationLagTime = &module.Chart{
		ID:       chartPxReplSetMember + "%s_replication_lag_time",
		Title:    "Replica Set member replication lag",
		Units:    "milliseconds",
		Fam:      "replica sets",
		Ctx:      "mongodb.repl_set_member_replication_lag_time",
		Priority: prioReplSetMemberReplicationLagTime,
		Dims: module.Dims{
			{ID: "repl_set_member_%s_replication_lag", Name: "replication_lag"},
		},
	}
	chartTmplReplSetMemberHeartbeatLatencyTime = &module.Chart{
		ID:       chartPxReplSetMember + "%s_heartbeat_latency_time",
		Title:    "Replica Set member heartbeat latency",
		Units:    "milliseconds",
		Fam:      "replica sets",
		Ctx:      "mongodb.repl_set_member_heartbeat_latency_time",
		Priority: prioReplSetMemberHeartbeatLatencyTime,
		Dims: module.Dims{
			{ID: "repl_set_member_%s_heartbeat_latency", Name: "heartbeat_latency"},
		},
	}
	chartTmplReplSetMemberPingRTTTime = &module.Chart{
		ID:       chartPxReplSetMember + "%s_ping_rtt_time",
		Title:    "Replica Set member ping RTT",
		Units:    "milliseconds",
		Fam:      "replica sets",
		Ctx:      "mongodb.repl_set_member_ping_rtt_time",
		Priority: prioReplSetMemberPingRTTTime,
		Dims: module.Dims{
			{ID: "repl_set_member_%s_ping_rtt", Name: "ping_rtt"},
		},
	}
	chartTmplReplSetMemberUptime = &module.Chart{
		ID:       chartPxReplSetMember + "%s_uptime",
		Title:    "Replica Set member uptime",
		Units:    "seconds",
		Fam:      "replica sets",
		Ctx:      "mongodb.repl_set_member_uptime",
		Priority: prioReplSetMemberUptime,
		Dims: module.Dims{
			{ID: "repl_set_member_%s_uptime", Name: "uptime"},
		},
	}
)

var (
	chartShardingNodesCount = &module.Chart{
		ID:       "sharding_nodes_count",
		Title:    "Sharding Nodes",
		Units:    "nodes",
		Fam:      "sharding",
		Ctx:      "mongodb.sharding_nodes_count",
		Type:     module.Stacked,
		Priority: prioShardingNodesCount,
		Dims: module.Dims{
			{ID: "shard_nodes_aware", Name: "shard_aware"},
			{ID: "shard_nodes_unaware", Name: "shard_unaware"},
		},
	}
	chartShardingShardedDatabases = &module.Chart{
		ID:       "sharding_sharded_databases_count",
		Title:    "Sharded databases",
		Units:    "databases",
		Fam:      "sharding",
		Ctx:      "mongodb.sharding_sharded_databases_count",
		Type:     module.Stacked,
		Priority: prioShardingShardedDatabasesCount,
		Dims: module.Dims{
			{ID: "shard_databases_partitioned", Name: "partitioned"},
			{ID: "shard_databases_unpartitioned", Name: "unpartitioned"},
		},
	}

	chartShardingShardedCollectionsCount = &module.Chart{
		ID:       "sharding_sharded_collections_count",
		Title:    "Sharded collections",
		Units:    "collections",
		Fam:      "sharding",
		Ctx:      "mongodb.sharding_sharded_collections_count",
		Type:     module.Stacked,
		Priority: prioShardingShardedCollectionsCount,
		Dims: module.Dims{
			{ID: "shard_collections_partitioned", Name: "partitioned"},
			{ID: "shard_collections_unpartitioned", Name: "unpartitioned"},
		},
	}

	chartTmplShardChunks = &module.Chart{
		ID:       chartPxShard + "%s_chunks",
		Title:    "Shard chunks",
		Units:    "chunks",
		Fam:      "sharding",
		Ctx:      "mongodb.sharding_shard_chunks_count",
		Priority: prioShardChunks,
		Dims: module.Dims{
			{ID: "shard_id_%s_chunks", Name: "chunks"},
		},
	}
)
