// SPDX-License-Identifier: GPL-3.0-or-later

package mongo

import "time"

// https://www.mongodb.com/docs/manual/reference/command/serverStatus
type documentServerStatus struct {
	Process      string                  `bson:"process"` // mongod|mongos
	OpCounters   documentOpCounters      `bson:"opcounters" stm:"operations"`
	OpLatencies  *documentOpLatencies    `bson:"opLatencies" stm:"operations_latencies"` // mongod only
	Connections  documentConnections     `bson:"connections" stm:"connections"`
	Network      documentNetwork         `bson:"network" stm:"network"`
	Memory       documentMemory          `bson:"mem" stm:"memory"`
	Metrics      documentMetrics         `bson:"metrics" stm:"metrics"`
	ExtraInfo    documentExtraInfo       `bson:"extra_info" stm:"extra_info"`
	Asserts      documentAsserts         `bson:"asserts" stm:"asserts"`
	Transactions *documentTransactions   `bson:"transactions" stm:"txn"` // mongod in 3.6.3+ and on mongos in 4.2+
	GlobalLock   *documentGlobalLock     `bson:"globalLock" stm:"global_lock"`
	Tcmalloc     *documentTCMallocStatus `bson:"tcmalloc" stm:"tcmalloc"`
	Locks        *documentLocks          `bson:"locks" stm:"locks"`
	WiredTiger   *documentWiredTiger     `bson:"wiredTiger" stm:"wiredtiger"`
	Repl         any                     `bson:"repl"`
}

type (
	// https://www.mongodb.com/docs/manual/reference/command/serverStatus/#opcounters
	documentOpCounters struct {
		Insert  int64 `bson:"insert" stm:"insert"`
		Query   int64 `bson:"query" stm:"query"`
		Update  int64 `bson:"update" stm:"update"`
		Delete  int64 `bson:"delete" stm:"delete"`
		GetMore int64 `bson:"getmore" stm:"getmore"`
		Command int64 `bson:"command" stm:"command"`
	}
	// https://www.mongodb.com/docs/manual/reference/command/serverStatus/#oplatencies
	documentOpLatencies struct {
		Reads    documentLatencyStats `bson:"reads" stm:"reads"`
		Writes   documentLatencyStats `bson:"writes" stm:"writes"`
		Commands documentLatencyStats `bson:"commands" stm:"commands"`
	}
	// https://www.mongodb.com/docs/manual/reference/operator/aggregation/collStats/#latencystats-document
	documentLatencyStats struct {
		Latency int64 `bson:"latency" stm:"latency"`
		Ops     int64 `bson:"ops" stm:"ops"`
	}
	// https://www.mongodb.com/docs/manual/reference/command/serverStatus/#connections
	documentConnections struct {
		Current                 int64  `bson:"current" stm:"current"`
		Available               int64  `bson:"available" stm:"available"`
		TotalCreated            int64  `bson:"totalCreated" stm:"total_created"`
		Active                  *int64 `bson:"active" stm:"active"`
		Threaded                *int64 `bson:"threaded" stm:"threaded"`
		ExhaustIsMaster         *int64 `bson:"exhaustIsMaster" stm:"exhaust_is_master"`
		ExhaustHello            *int64 `bson:"exhaustHello" stm:"exhaust_hello"`
		AwaitingTopologyChanges *int64 `bson:"awaitingTopologyChanges" stm:"awaiting_topology_changes"`
	}
	// https://www.mongodb.com/docs/manual/reference/command/serverStatus/#network
	documentNetwork struct {
		BytesIn              int64  `bson:"bytesIn" stm:"bytes_in"`
		BytesOut             int64  `bson:"bytesOut" stm:"bytes_out"`
		NumRequests          int64  `bson:"numRequests" stm:"requests"`
		NumSlowDNSOperations *int64 `bson:"numSlowDNSOperations" stm:"slow_dns_operations"` // 4.4+
		NumSlowSSLOperations *int64 `bson:"numSlowSSLOperations" stm:"slow_ssl_operations"` // 4.4+
	}
	// https://www.mongodb.com/docs/manual/reference/command/serverStatus/#mem
	documentMemory struct {
		Resident int64 `bson:"resident" stm:"resident,1048576,1"`
		Virtual  int64 `bson:"virtual" stm:"virtual,1048576,1"`
	}
	// https://www.mongodb.com/docs/manual/reference/command/serverStatus/#extra_info
	documentExtraInfo struct {
		PageFaults int64 `bson:"page_faults" stm:"page_faults"`
	}
	// Values:
	//  - mongodb: https://github.com/mongodb/mongo/blob/54e1be7d98aa154e1676d6d652b4d2d1a1073b07/src/mongo/util/tcmalloc_server_status_section.cpp#L88
	//  - tcmalloc: https://github.com/google/tcmalloc/blob/927c1433141daa1f0bcf920e6d71bf64795cc2c2/tcmalloc/global_stats.cc#L582
	// formattedString:
	//  - https://github.com/google/tcmalloc/blob/master/docs/stats.md
	//  - https://github.com/google/tcmalloc/blob/927c1433141daa1f0bcf920e6d71bf64795cc2c2/tcmalloc/global_stats.cc#L208
	documentTCMallocStatus struct {
		Generic *struct {
			CurrentAllocatedBytes int64 `bson:"current_allocated_bytes" stm:"current_allocated_bytes"`
			HeapSize              int64 `bson:"heap_size" stm:"heap_size"`
		} `bson:"generic" stm:"generic"`
		Tcmalloc *struct {
			PageheapFreeBytes            int64 `bson:"pageheap_free_bytes" stm:"pageheap_free_bytes"`
			PageheapUnmappedBytes        int64 `bson:"pageheap_unmapped_bytes" stm:"pageheap_unmapped_bytes"`
			MaxTotalThreadCacheBytes     int64 `bson:"max_total_thread_cache_bytes" stm:"max_total_thread_cache_bytes"`
			CurrentTotalThreadCacheBytes int64 `bson:"current_total_thread_cache_bytes" stm:"current_total_thread_cache_bytes"`
			TotalFreeBytes               int64 `bson:"total_free_bytes" stm:"total_free_bytes"`
			CentralCacheFreeBytes        int64 `bson:"central_cache_free_bytes" stm:"central_cache_free_bytes"`
			TransferCacheFreeBytes       int64 `bson:"transfer_cache_free_bytes" stm:"transfer_cache_free_bytes"`
			ThreadCacheFreeBytes         int64 `bson:"thread_cache_free_bytes" stm:"thread_cache_free_bytes"`
			AggressiveMemoryDecommit     int64 `bson:"aggressive_memory_decommit" stm:"aggressive_memory_decommit"`
			PageheapCommittedBytes       int64 `bson:"pageheap_committed_bytes" stm:"pageheap_committed_bytes"`
			PageheapScavengeBytes        int64 `bson:"pageheap_scavenge_bytes" stm:"pageheap_scavenge_bytes"`
			PageheapCommitCount          int64 `bson:"pageheap_commit_count" stm:"pageheap_commit_count"`
			PageheapTotalCommitBytes     int64 `bson:"pageheap_total_commit_bytes" stm:"pageheap_total_commit_bytes"`
			PageheapDecommitCount        int64 `bson:"pageheap_decommit_count" stm:"pageheap_decommit_count"`
			PageheapTotalDecommitBytes   int64 `bson:"pageheap_total_decommit_bytes" stm:"pageheap_total_decommit_bytes"`
			PageheapReserveCount         int64 `bson:"pageheap_reserve_count" stm:"pageheap_reserve_count"`
			PageheapTotalReserveBytes    int64 `bson:"pageheap_total_reserve_bytes" stm:"pageheap_total_reserve_bytes"`
			SpinlockTotalDelayNs         int64 `bson:"spinlock_total_delay_ns" stm:"spinlock_total_delay_ns"`
		} `bson:"tcmalloc" stm:""`
	}
	// https://www.mongodb.com/docs/manual/reference/command/serverStatus/#metrics
	documentMetrics struct {
		Cursor struct {
			TotalOpened *int64 `bson:"totalOpened" stm:"total_opened"`
			TimedOut    *int64 `bson:"timedOut" stm:"timed_out"`
			Open        struct {
				NoTimeout *int64 `bson:"noTimeout" stm:"no_timeout"`
				Total     *int64 `bson:"total" stm:"total"`
			} `bson:"open" stm:"open"`
			Lifespan *struct {
				GreaterThanOrEqual10Minutes int64 `bson:"greaterThanOrEqual10Minutes" stm:"greater_than_or_equal_10_minutes"`
				LessThan10Minutes           int64 `bson:"lessThan10Minutes" stm:"less_than_10_minutes"`
				LessThan15Seconds           int64 `bson:"lessThan15Seconds" stm:"less_than_15_seconds"`
				LessThan1Minute             int64 `bson:"lessThan1Minute" stm:"less_than_1_minute"`
				LessThan1Second             int64 `bson:"lessThan1Second" stm:"less_than_1_second"`
				LessThan30Seconds           int64 `bson:"lessThan30Seconds" stm:"less_than_30_seconds"`
				LessThan5Seconds            int64 `bson:"lessThan5Seconds" stm:"less_than_5_seconds"`
			} `bson:"lifespan" stm:"lifespan"`
		} `bson:"cursor" stm:"cursor"`
		Document struct {
			Deleted  int64 `bson:"deleted" stm:"deleted"`
			Inserted int64 `bson:"inserted" stm:"inserted"`
			Returned int64 `bson:"returned" stm:"returned"`
			Updated  int64 `bson:"updated" stm:"updated"`
		} `bson:"document" stm:"document"`
		QueryExecutor struct {
			Scanned        int64 `bson:"scanned" stm:"scanned"`
			ScannedObjects int64 `bson:"scannedObjects" stm:"scanned_objects"`
		} `bson:"queryExecutor" stm:"query_executor"`
	}
	// https://www.mongodb.com/docs/manual/reference/command/serverStatus/#asserts
	documentAsserts struct {
		Regular   int64 `bson:"regular" stm:"regular"`
		Warning   int64 `bson:"warning" stm:"warning"`
		Msg       int64 `bson:"msg" stm:"msg"`
		User      int64 `bson:"user" stm:"user"`
		Tripwire  int64 `bson:"tripwire" stm:"tripwire"`
		Rollovers int64 `bson:"rollovers" stm:"rollovers"`
	}
	// https://www.mongodb.com/docs/manual/reference/command/serverStatus/#transactions
	documentTransactions struct {
		CurrentActive   *int64                           `bson:"currentActive" stm:"active"`           // mongod in 4.0.2+ and mongos in 4.2.1+
		CurrentInactive *int64                           `bson:"currentInactive" stm:"inactive"`       // mongod in 4.0.2+ and mongos in 4.2.1+
		CurrentOpen     *int64                           `bson:"currentOpen" stm:"open"`               // mongod in 4.0.2+ and mongos in 4.2.1+
		CurrentPrepared *int64                           `bson:"currentPrepared" stm:"prepared"`       // 4.2+ mongod only
		TotalAborted    *int64                           `bson:"totalAborted" stm:"total_aborted"`     // mongod in 4.0.2+ and mongos in 4.2+
		TotalCommitted  *int64                           `bson:"totalCommitted" stm:"total_committed"` // mongod in 4.0.2+ and mongos in 4.2+
		TotalStarted    *int64                           `bson:"totalStarted" stm:"total_started"`     // mongod in 4.0.2+ and mongos in 4.2+
		TotalPrepared   *int64                           `bson:"totalPrepared" stm:"total_prepared"`   // mongod in 4.0.2+ and mongos in 4.2+
		CommitTypes     *documentTransactionsCommitTypes `bson:"commitTypes" stm:"commit_types"`       // mongos only
	}
	// https://www.mongodb.com/docs/manual/reference/command/serverStatus/#mongodb-serverstatus-serverstatus.transactions.commitTypes
	documentTransactionsCommitTypes struct {
		NoShards         documentTransactionsCommitType `bson:"noShards" stm:"no_shards"`
		SingleShard      documentTransactionsCommitType `bson:"singleShard" stm:"single_shard"`
		SingleWriteShard documentTransactionsCommitType `bson:"singleWriteShard" stm:"single_write_shard"`
		ReadOnly         documentTransactionsCommitType `bson:"readOnly" stm:"read_only"`
		TwoPhaseCommit   documentTransactionsCommitType `bson:"twoPhaseCommit" stm:"two_phase_commit"`
		RecoverWithToken documentTransactionsCommitType `bson:"recoverWithToken" stm:"recover_with_token"`
	}
	documentTransactionsCommitType struct {
		Initiated                int64 `json:"initiated" stm:"initiated"`
		Successful               int64 `json:"successful" stm:"successful"`
		SuccessfulDurationMicros int64 `json:"successfulDurationMicros" stm:"successful_duration_micros"`
	}
	// https://www.mongodb.com/docs/manual/reference/command/serverStatus/#globallock
	documentGlobalLock struct {
		CurrentQueue *struct {
			Readers int64 `bson:"readers" stm:"readers"`
			Writers int64 `bson:"writers" stm:"writers"`
		} `bson:"currentQueue" stm:"current_queue"`
		ActiveClients *struct {
			Readers int64 `bson:"readers" stm:"readers"`
			Writers int64 `bson:"writers" stm:"writers"`
		} `bson:"activeClients" stm:"active_clients"`
	}
	// https://www.mongodb.com/docs/manual/reference/command/serverStatus/#mongodb-serverstatus-serverstatus.locks
	documentLocks struct {
		Global     *documentLockType `bson:"Global" stm:"global"`
		Database   *documentLockType `bson:"Database" stm:"database"`
		Collection *documentLockType `bson:"Collection" stm:"collection"`
		Mutex      *documentLockType `bson:"Mutex" stm:"mutex"`
		Metadata   *documentLockType `bson:"Metadata" stm:"metadata"`
		Oplog      *documentLockType `bson:"oplog" stm:"oplog"`
	}
	documentLockType struct {
		AcquireCount documentLockModes `bson:"acquireCount" stm:"acquire"`
	}
	documentLockModes struct {
		Shared          int64 `bson:"R" stm:"shared"`
		Exclusive       int64 `bson:"W" stm:"exclusive"`
		IntentShared    int64 `bson:"r" stm:"intent_shared"`
		IntentExclusive int64 `bson:"w" stm:"intent_exclusive"`
	}
	// https://www.mongodb.com/docs/manual/reference/command/serverStatus/#wiredtiger
	documentWiredTiger struct {
		ConcurrentTransaction struct {
			Write struct {
				Out       int `bson:"out" stm:"out"`
				Available int `bson:"available" stm:"available"`
			} `bson:"write" stm:"write"`
			Read struct {
				Out       int `bson:"out" stm:"out"`
				Available int `bson:"available" stm:"available"`
			} `bson:"read" stm:"read"`
		} `bson:"concurrentTransactions" stm:"concurrent_txn"`
		Cache struct {
			BytesCurrentlyInCache    int `bson:"bytes currently in the cache" stm:"currently_in_cache_bytes"`
			MaximumBytesConfigured   int `bson:"maximum bytes configured" stm:"maximum_configured_bytes"`
			TrackedDirtyBytesInCache int `bson:"tracked dirty bytes in the cache" stm:"tracked_dirty_in_the_cache_bytes"`
			UnmodifiedPagesEvicted   int `bson:"unmodified pages evicted" stm:"unmodified_evicted_pages"`
			ModifiedPagesEvicted     int `bson:"modified pages evicted" stm:"modified_evicted_pages"`
			PagesReadIntoCache       int `bson:"pages read into cache" stm:"read_into_cache_pages"`
			PagesWrittenFromCache    int `bson:"pages written from cache" stm:"written_from_cache_pages"`
		} `bson:"cache" stm:"cache"`
	}
)

// https://www.mongodb.com/docs/manual/reference/command/dbStats/
type documentDBStats struct {
	Collections int64 `bson:"collections"`
	Views       int64 `bson:"views"`
	Indexes     int64 `bson:"indexes"`
	Objects     int64 `bson:"objects"`
	DataSize    int64 `bson:"dataSize"`
	IndexSize   int64 `bson:"indexSize"`
	StorageSize int64 `bson:"storageSize"`
}

// https://www.mongodb.com/docs/manual/reference/command/replSetGetStatus/
type documentReplSetStatus struct {
	Date    time.Time               `bson:"date"`
	Members []documentReplSetMember `bson:"members"`
}

type (
	documentReplSetMember struct {
		Name              string     `bson:"name"`
		Self              *bool      `bson:"self"`
		State             int        `bson:"state"`
		Health            int        `bson:"health"`
		OptimeDate        time.Time  `bson:"optimeDate"`
		LastHeartbeat     *time.Time `bson:"lastHeartbeat"`
		LastHeartbeatRecv *time.Time `bson:"lastHeartbeatRecv"`
		PingMs            *int64     `bson:"pingMs"`
		Uptime            int64      `bson:"uptime"`
	}
)

type documentAggrResults struct {
	Bool  bool  `bson:"_id"`
	Count int64 `bson:"count"`
}

type (
	documentAggrResult struct {
		True  int64
		False int64
	}
)

type documentPartitionedResult struct {
	Partitioned   int64
	UnPartitioned int64
}

type documentShardNodesResult struct {
	ShardAware   int64
	ShardUnaware int64
}
