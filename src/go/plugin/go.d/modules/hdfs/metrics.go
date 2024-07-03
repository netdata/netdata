// SPDX-License-Identifier: GPL-3.0-or-later

package hdfs

// HDFS Architecture
// https://hadoop.apache.org/docs/r1.2.1/hdfs_design.html#NameNode+and+DataNodes

// Metrics description
// https://hadoop.apache.org/docs/current/hadoop-project-dist/hadoop-common/Metrics.html

// Good article
// https://www.datadoghq.com/blog/monitor-hadoop-metrics/#hdfs-metrics

type metrics struct {
	Jvm              *jvmMetrics              `stm:"jvm"`  // both
	Rpc              *rpcActivityMetrics      `stm:"rpc"`  // both
	FSNameSystem     *fsNameSystemMetrics     `stm:"fsns"` // namenode
	FSDatasetState   *fsDatasetStateMetrics   `stm:"fsds"` // datanode
	DataNodeActivity *dataNodeActivityMetrics `stm:"dna"`  // datanode
}

type jvmMetrics struct {
	ProcessName string `json:"tag.ProcessName"`
	HostName    string `json:"tag.Hostname"`
	//MemNonHeapUsedM            float64 `stm:"mem_non_heap_used,1000,1"`
	//MemNonHeapCommittedM       float64 `stm:"mem_non_heap_committed,1000,1"`
	//MemNonHeapMaxM             float64 `stm:"mem_non_heap_max"`
	MemHeapUsedM      float64 `stm:"mem_heap_used,1000,1"`
	MemHeapCommittedM float64 `stm:"mem_heap_committed,1000,1"`
	MemHeapMaxM       float64 `stm:"mem_heap_max"`
	//MemMaxM                    float64 `stm:"mem_max"`
	GcCount                    float64 `stm:"gc_count"`
	GcTimeMillis               float64 `stm:"gc_time_millis"`
	GcNumWarnThresholdExceeded float64 `stm:"gc_num_warn_threshold_exceeded"`
	GcNumInfoThresholdExceeded float64 `stm:"gc_num_info_threshold_exceeded"`
	GcTotalExtraSleepTime      float64 `stm:"gc_total_extra_sleep_time"`
	ThreadsNew                 float64 `stm:"threads_new"`
	ThreadsRunnable            float64 `stm:"threads_runnable"`
	ThreadsBlocked             float64 `stm:"threads_blocked"`
	ThreadsWaiting             float64 `stm:"threads_waiting"`
	ThreadsTimedWaiting        float64 `stm:"threads_timed_waiting"`
	ThreadsTerminated          float64 `stm:"threads_terminated"`
	LogFatal                   float64 `stm:"log_fatal"`
	LogError                   float64 `stm:"log_error"`
	LogWarn                    float64 `stm:"log_warn"`
	LogInfo                    float64 `stm:"log_info"`
}

type rpcActivityMetrics struct {
	ReceivedBytes       float64 `stm:"received_bytes"`
	SentBytes           float64 `stm:"sent_bytes"`
	RpcQueueTimeNumOps  float64 `stm:"queue_time_num_ops"`
	RpcQueueTimeAvgTime float64 `stm:"queue_time_avg_time,1000,1"`
	//RpcProcessingTimeNumOps  float64
	RpcProcessingTimeAvgTime float64 `stm:"processing_time_avg_time,1000,1"`
	//DeferredRpcProcessingTimeNumOps  float64
	//DeferredRpcProcessingTimeAvgTime float64
	//RpcAuthenticationFailures        float64
	//RpcAuthenticationSuccesses       float64
	//RpcAuthorizationFailures         float64
	//RpcAuthorizationSuccesses        float64
	//RpcClientBackoff                 float64
	//RpcSlowCalls                     float64
	NumOpenConnections float64 `stm:"num_open_connections"`
	CallQueueLength    float64 `stm:"call_queue_length"`
	//NumDroppedConnections            float64
}

type fsNameSystemMetrics struct {
	HostName string `json:"tag.Hostname"`
	HAState  string `json:"tag.HAState"`
	//TotalSyncTimes                               float64 `json:"tag.tag.TotalSyncTimes"`
	MissingBlocks float64 `stm:"missing_blocks"`
	//MissingReplOneBlocks                         float64 `stm:"missing_repl_one_blocks"`
	//ExpiredHeartbeats                            float64 `stm:"expired_heartbeats"`
	//TransactionsSinceLastCheckpoint              float64 `stm:"transactions_since_last_checkpoint"`
	//TransactionsSinceLastLogRoll                 float64 `stm:"transactions_since_last_log_roll"`
	//LastWrittenTransactionId                     float64 `stm:"last_written_transaction_id"`
	//LastCheckpointTime                           float64 `stm:"last_checkpoint_time"`
	CapacityTotal float64 `stm:"capacity_total"`
	//CapacityTotalGB                              float64 `stm:"capacity_total_gb"`
	CapacityDfsUsed float64 `json:"CapacityUsed" stm:"capacity_used_dfs"`
	//CapacityUsedGB                               float64 `stm:"capacity_used_gb"`
	CapacityRemaining float64 `stm:"capacity_remaining"`
	//ProvidedCapacityTotal                        float64 `stm:"provided_capacity_total"`
	//CapacityRemainingGB                          float64 `stm:"capacity_remaining_gb"`
	CapacityUsedNonDFS float64 `stm:"capacity_used_non_dfs"`
	TotalLoad          float64 `stm:"total_load"`
	//SnapshottableDirectories                     float64 `stm:"snapshottable_directories"`
	//Snapshots                                    float64 `stm:"snapshots"`
	//NumEncryptionZones                           float64 `stm:"num_encryption_zones"`
	//LockQueueLength                              float64 `stm:"lock_queue_length"`
	BlocksTotal float64 `stm:"blocks_total"`
	//NumFilesUnderConstruction                    float64 `stm:"num_files_under_construction"`
	//NumActiveClients                             float64 `stm:"num_active_clients"`
	FilesTotal float64 `stm:"files_total"`
	//PendingReplicationBlocks    float64 `stm:"pending_replication_blocks"`
	//PendingReconstructionBlocks float64 `stm:"pending_reconstruction_blocks"`
	UnderReplicatedBlocks float64 `stm:"under_replicated_blocks"`
	//LowRedundancyBlocks                          float64 `stm:"low_redundancy_blocks"`
	CorruptBlocks float64 `stm:"corrupt_blocks"`
	//ScheduledReplicationBlocks float64 `stm:"scheduled_replication_blocks"`
	//PendingDeletionBlocks      float64 `stm:"pending_deletion_blocks"`
	//LowRedundancyReplicatedBlocks                float64 `stm:"low_redundancy_replicated_blocks"`
	//CorruptReplicatedBlocks                      float64 `stm:"corrupt_replicated_blocks"`
	//MissingReplicatedBlocks                      float64 `stm:"missing_replicated_blocks"`
	//MissingReplicationOneBlocks                  float64 `stm:"missing_replication_one_blocks"`
	//HighestPriorityLowRedundancyReplicatedBlocks float64 `stm:"highest_priority_low_redundancy_replicated_blocks"`
	//HighestPriorityLowRedundancyECBlocks         float64 `stm:"highest_priority_low_redundancy_ec_blocks"`
	//BytesInFutureReplicatedBlocks                float64 `stm:"bytes_in_future_replicated_blocks"`
	//PendingDeletionReplicatedBlocks              float64 `stm:"pending_deletion_replicated_blocks"`
	//TotalReplicatedBlocks                        float64 `stm:"total_replicated_blocks"`
	//LowRedundancyECBlockGroups                   float64 `stm:"low_redundancy_ec_block_groups"`
	//CorruptECBlockGroups                         float64 `stm:"corrupt_ec_block_groups"`
	//MissingECBlockGroups                         float64 `stm:"missing_ec_block_groups"`
	//BytesInFutureECBlockGroups                   float64 `stm:"bytes_in_future_ec_block_groups"`
	//PendingDeletionECBlocks                      float64 `stm:"pending_deletion_ec_blocks"`
	//TotalECBlockGroups                           float64 `stm:"total_ec_block_groups"`
	//ExcessBlocks                                 float64 `stm:"excess_blocks"`
	//NumTimedOutPendingReconstructions            float64 `stm:"num_timed_out_pending_reconstructions"`
	//PostponedMisreplicatedBlocks                 float64 `stm:"postponed_misreplicated_blocks"`
	//PendingDataNodeMessageCount                  float64 `stm:"pending_data_node_message_count"`
	//MillisSinceLastLoadedEdits                   float64 `stm:"millis_since_last_loaded_edits"`
	//BlockCapacity                                float64 `stm:"block_capacity"`
	NumLiveDataNodes float64 `stm:"num_live_data_nodes"`
	NumDeadDataNodes float64 `stm:"num_dead_data_nodes"`
	//NumDecomLiveDataNodes                        float64 `stm:"num_decom_live_data_nodes"`
	//NumDecomDeadDataNodes                        float64 `stm:"num_decom_dead_data_nodes"`
	VolumeFailuresTotal float64 `stm:"volume_failures_total"`
	//EstimatedCapacityLostTotal                   float64 `stm:"estimated_capacity_lost_total"`
	//NumDecommissioningDataNodes                  float64 `stm:"num_decommissioning_data_nodes"`
	StaleDataNodes float64 `stm:"stale_data_nodes"`
	//NumStaleStorages                             float64 `stm:"num_stale_storages"`
	//TotalSyncCount                               float64 `stm:"total_sync_count"`
	//NumInMaintenanceLiveDataNodes                float64 `stm:"num_in_maintenance_live_data_nodes"`
	//NumInMaintenanceDeadDataNodes                float64 `stm:"num_in_maintenance_dead_data_nodes"`
	//NumEnteringMaintenanceDataNodes              float64 `stm:"num_entering_maintenance_data_nodes"`

	// custom attributes
	CapacityUsed float64 `json:"-" stm:"capacity_used"`
}

type fsDatasetStateMetrics struct {
	HostName         string  `json:"tag.Hostname"`
	Capacity         float64 `stm:"capacity_total"`
	DfsUsed          float64 `stm:"capacity_used_dfs"`
	Remaining        float64 `stm:"capacity_remaining"`
	NumFailedVolumes float64 `stm:"num_failed_volumes"`
	//LastVolumeFailureDate      float64 `stm:"LastVolumeFailureDate"`
	//EstimatedCapacityLostTotal float64 `stm:"EstimatedCapacityLostTotal"`
	//CacheUsed                  float64 `stm:"CacheUsed"`
	//CacheCapacity              float64 `stm:"CacheCapacity"`
	//NumBlocksCached            float64 `stm:"NumBlocksCached"`
	//NumBlocksFailedToCache     float64 `stm:"NumBlocksFailedToCache"`
	//NumBlocksFailedToUnCache   float64 `stm:"NumBlocksFailedToUnCache"`

	// custom attributes
	CapacityUsedNonDFS float64 `stm:"capacity_used_non_dfs"`
	CapacityUsed       float64 `stm:"capacity_used"`
}

type dataNodeActivityMetrics struct {
	HostName     string  `json:"tag.Hostname"`
	BytesWritten float64 `stm:"bytes_written"`
	//TotalWriteTime                             float64
	BytesRead float64 `stm:"bytes_read"`
	//TotalReadTime                              float64
	//BlocksWritten float64
	//BlocksRead    float64
	//BlocksReplicated                           float64
	//BlocksRemoved                              float64
	//BlocksVerified                             float64
	//BlockVerificationFailures                  float64
	//BlocksCached                               float64
	//BlocksUncached                             float64
	//ReadsFromLocalClient                       float64
	//ReadsFromRemoteClient                      float64
	//WritesFromLocalClient                      float64
	//WritesFromRemoteClient                     float64
	//BlocksGetLocalPathInfo                     float64
	//RemoteBytesRead                            float64
	//RemoteBytesWritten                         float64
	//RamDiskBlocksWrite                         float64
	//RamDiskBlocksWriteFallback                 float64
	//RamDiskBytesWrite                          float64
	//RamDiskBlocksReadHits                      float64
	//RamDiskBlocksEvicted                       float64
	//RamDiskBlocksEvictedWithoutRead            float64
	//RamDiskBlocksEvictionWindowMsNumOps        float64
	//RamDiskBlocksEvictionWindowMsAvgTime       float64
	//RamDiskBlocksLazyPersisted                 float64
	//RamDiskBlocksDeletedBeforeLazyPersisted    float64
	//RamDiskBytesLazyPersisted                  float64
	//RamDiskBlocksLazyPersistWindowMsNumOps     float64
	//RamDiskBlocksLazyPersistWindowMsAvgTime    float64
	//FsyncCount                                 float64
	//VolumeFailures                             float64
	//DatanodeNetworkErrors                      float64
	//DataNodeActiveXceiversCount                float64
	//ReadBlockOpNumOps                          float64
	//ReadBlockOpAvgTime                         float64
	//WriteBlockOpNumOps                         float64
	//WriteBlockOpAvgTime                        float64
	//BlockChecksumOpNumOps                      float64
	//BlockChecksumOpAvgTime                     float64
	//CopyBlockOpNumOps                          float64
	//CopyBlockOpAvgTime                         float64
	//ReplaceBlockOpNumOps                       float64
	//ReplaceBlockOpAvgTime                      float64
	//HeartbeatsNumOps                           float64
	//HeartbeatsAvgTime                          float64
	//HeartbeatsTotalNumOps                      float64
	//HeartbeatsTotalAvgTime                     float64
	//LifelinesNumOps                            float64
	//LifelinesAvgTime                           float64
	//BlockReportsNumOps                         float64
	//BlockReportsAvgTime                        float64
	//IncrementalBlockReportsNumOps              float64
	//IncrementalBlockReportsAvgTime             float64
	//CacheReportsNumOps                         float64
	//CacheReportsAvgTime                        float64
	//PacketAckRoundTripTimeNanosNumOps          float64
	//PacketAckRoundTripTimeNanosAvgTime         float64
	//FlushNanosNumOps                           float64
	//FlushNanosAvgTime                          float64
	//FsyncNanosNumOps                           float64
	//FsyncNanosAvgTime                          float64
	//SendDataPacketBlockedOnNetworkNanosNumOps  float64
	//SendDataPacketBlockedOnNetworkNanosAvgTime float64
	//SendDataPacketTransferNanosNumOps          float64
	//SendDataPacketTransferNanosAvgTime         float64
	//BlocksInPendingIBR                         float64
	//BlocksReceivingInPendingIBR                float64
	//BlocksReceivedInPendingIBR                 float64
	//BlocksDeletedInPendingIBR                  float64
	//EcReconstructionTasks                      float64
	//EcFailedReconstructionTasks                float64
	//EcDecodingTimeNanos                        float64
	//EcReconstructionBytesRead                  float64
	//EcReconstructionBytesWritten               float64
	//EcReconstructionRemoteBytesRead            float64
	//EcReconstructionReadTimeMillis             float64
	//EcReconstructionDecodingTimeMillis         float64
	//EcReconstructionWriteTimeMillis            float64
}
