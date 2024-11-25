// SPDX-License-Identifier: GPL-3.0-or-later

package riakkv

// FIXME: old data (likely wrong) from https://github.com/netdata/netdata/issues/2413#issuecomment-500867044
type riakStats struct {
	NodeGetsTotal *int64 `json:"node_gets_total" stm:"node_gets_total"`
	NodePutsTotal *int64 `json:"node_puts_total" stm:"node_puts_total"`

	VnodeCounterUpdateTotal *int64 `json:"vnode_counter_update_total" stm:"vnode_counter_update_total"`
	VnodeSetUpdateTotal     *int64 `json:"vnode_set_update_total" stm:"vnode_set_update_total"`
	VnodeMapUpdateTotal     *int64 `json:"vnode_map_update_total" stm:"vnode_map_update_total"`

	SearchQueryThroughputCount *int64 `json:"search_query_throughput_count" stm:"search_query_throughput_count"`
	SearchIndexThroughputCount *int64 `json:"search_index_throughput_count" stm:"search_index_throughput_count"`

	ConsistentGetsTotal *int64 `json:"consistent_gets_total" stm:"consistent_gets_total"`
	ConsistentPutsTotal *int64 `json:"consistent_puts_total" stm:"consistent_puts_total"`

	NodeGetFsmTimeMean   *int64 `json:"node_get_fsm_time_mean" stm:"node_get_fsm_time_mean"`
	NodeGetFsmTimeMedian *int64 `json:"node_get_fsm_time_median" stm:"node_get_fsm_time_median"`
	NodeGetFsmTime95     *int64 `json:"node_get_fsm_time_95" stm:"node_get_fsm_time_95"`
	NodeGetFsmTime99     *int64 `json:"node_get_fsm_time_99" stm:"node_get_fsm_time_99"`
	NodeGetFsmTime100    *int64 `json:"node_get_fsm_time_100" stm:"node_get_fsm_time_100"`

	NodePutFsmTimeMean   *int64 `json:"node_put_fsm_time_mean" stm:"node_put_fsm_time_mean"`
	NodePutFsmTimeMedian *int64 `json:"node_put_fsm_time_median" stm:"node_put_fsm_time_median"`
	NodePutFsmTime95     *int64 `json:"node_put_fsm_time_95" stm:"node_put_fsm_time_95"`
	NodePutFsmTime99     *int64 `json:"node_put_fsm_time_99" stm:"node_put_fsm_time_99"`
	NodePutFsmTime100    *int64 `json:"node_put_fsm_time_100" stm:"node_put_fsm_time_100"`

	ObjectCounterMergeTimeMean   *int64 `json:"object_counter_merge_time_mean" stm:"object_counter_merge_time_mean"`
	ObjectCounterMergeTimeMedian *int64 `json:"object_counter_merge_time_median" stm:"object_counter_merge_time_median"`
	ObjectCounterMergeTime95     *int64 `json:"object_counter_merge_time_95" stm:"object_counter_merge_time_95"`
	ObjectCounterMergeTime99     *int64 `json:"object_counter_merge_time_99" stm:"object_counter_merge_time_99"`
	ObjectCounterMergeTime100    *int64 `json:"object_counter_merge_time_100" stm:"object_counter_merge_time_100"`

	ObjectSetMergeTimeMean   *int64 `json:"object_set_merge_time_mean" stm:"object_set_merge_time_mean"`
	ObjectSetMergeTimeMedian *int64 `json:"object_set_merge_time_median" stm:"object_set_merge_time_median"`
	ObjectSetMergeTime95     *int64 `json:"object_set_merge_time_95" stm:"object_set_merge_time_95"`
	ObjectSetMergeTime99     *int64 `json:"object_set_merge_time_99" stm:"object_set_merge_time_99"`
	ObjectSetMergeTime100    *int64 `json:"object_set_merge_time_100" stm:"object_set_merge_time_100"`

	ObjectMapMergeTimeMean   *int64 `json:"object_map_merge_time_mean" stm:"object_map_merge_time_mean"`
	ObjectMapMergeTimeMedian *int64 `json:"object_map_merge_time_median" stm:"object_map_merge_time_median"`
	ObjectMapMergeTime95     *int64 `json:"object_map_merge_time_95" stm:"object_map_merge_time_95"`
	ObjectMapMergeTime99     *int64 `json:"object_map_merge_time_99" stm:"object_map_merge_time_99"`
	ObjectMapMergeTime100    *int64 `json:"object_map_merge_time_100" stm:"object_map_merge_time_100"`

	SearchQueryLatencyMin    *int64 `json:"search_query_latency_min" stm:"search_query_latency_min"`
	SearchQueryLatencyMedian *int64 `json:"search_query_latency_median" stm:"search_query_latency_median"`
	SearchQueryLatency95     *int64 `json:"search_query_latency_95" stm:"search_query_latency_95"`
	SearchQueryLatency99     *int64 `json:"search_query_latency_99" stm:"search_query_latency_99"`
	SearchQueryLatency999    *int64 `json:"search_query_latency_999" stm:"search_query_latency_999"`
	SearchQueryLatencyMax    *int64 `json:"search_query_latency_max" stm:"search_query_latency_max"`

	SearchIndexLatencyMin    *int64 `json:"search_index_latency_min" stm:"search_index_latency_min"`
	SearchIndexLatencyMedian *int64 `json:"search_index_latency_median" stm:"search_index_latency_median"`
	SearchIndexLatency95     *int64 `json:"search_index_latency_95" stm:"search_index_latency_95"`
	SearchIndexLatency99     *int64 `json:"search_index_latency_99" stm:"search_index_latency_99"`
	SearchIndexLatency999    *int64 `json:"search_index_latency_999" stm:"search_index_latency_999"`
	SearchIndexLatencyMax    *int64 `json:"search_index_latency_max" stm:"search_index_latency_max"`

	ConsistentGetTimeMean   *int64 `json:"consistent_get_time_mean" stm:"consistent_get_time_mean"`
	ConsistentGetTimeMedian *int64 `json:"consistent_get_time_median" stm:"consistent_get_time_median"`
	ConsistentGetTime95     *int64 `json:"consistent_get_time_95" stm:"consistent_get_time_95"`
	ConsistentGetTime99     *int64 `json:"consistent_get_time_99" stm:"consistent_get_time_99"`
	ConsistentGetTime100    *int64 `json:"consistent_get_time_100" stm:"consistent_get_time_100"`

	ConsistentPutTimeMean   *int64 `json:"consistent_put_time_mean" stm:"consistent_put_time_mean"`
	ConsistentPutTimeMedian *int64 `json:"consistent_put_time_median" stm:"consistent_put_time_median"`
	ConsistentPutTime95     *int64 `json:"consistent_put_time_95" stm:"consistent_put_time_95"`
	ConsistentPutTime99     *int64 `json:"consistent_put_time_99" stm:"consistent_put_time_99"`
	ConsistentPutTime100    *int64 `json:"consistent_put_time_100" stm:"consistent_put_time_100"`

	SysProcesses        *int64 `json:"sys_processes" stm:"sys_processes"`
	MemoryProcesses     *int64 `json:"memory_processes" stm:"memory_processes"`
	MemoryProcessesUsed *int64 `json:"memory_processes_used" stm:"memory_processes_used"`

	NodeGetFsmSiblingsMean   *int64 `json:"node_get_fsm_siblings_mean" stm:"node_get_fsm_siblings_mean"`
	NodeGetFsmSiblingsMedian *int64 `json:"node_get_fsm_siblings_median" stm:"node_get_fsm_siblings_median"`
	NodeGetFsmSiblings99     *int64 `json:"node_get_fsm_siblings_99" stm:"node_get_fsm_siblings_99"`
	NodeGetFsmSiblings95     *int64 `json:"node_get_fsm_siblings_95" stm:"node_get_fsm_siblings_95"`
	NodeGetFsmSiblings100    *int64 `json:"node_get_fsm_siblings_100" stm:"node_get_fsm_siblings_100"`

	NodeGetFsmObjsizeMean   *int64 `json:"node_get_fsm_objsize_mean" stm:"node_get_fsm_objsize_mean"`
	NodeGetFsmObjsizeMedian *int64 `json:"node_get_fsm_objsize_median" stm:"node_get_fsm_objsize_median"`
	NodeGetFsmObjsize95     *int64 `json:"node_get_fsm_objsize_95" stm:"node_get_fsm_objsize_95"`
	NodeGetFsmObjsize99     *int64 `json:"node_get_fsm_objsize_99" stm:"node_get_fsm_objsize_99"`
	NodeGetFsmObjsize100    *int64 `json:"node_get_fsm_objsize_100" stm:"node_get_fsm_objsize_100"`

	RiakSearchVnodeqMean   *int64 `json:"riak_search_vnodeq_mean" stm:"riak_search_vnodeq_mean"`
	RiakSearchVnodeqMedian *int64 `json:"riak_search_vnodeq_median" stm:"riak_search_vnodeq_median"`
	RiakSearchVnodeq95     *int64 `json:"riak_search_vnodeq_95" stm:"riak_search_vnodeq_95"`
	RiakSearchVnodeq99     *int64 `json:"riak_search_vnodeq_99" stm:"riak_search_vnodeq_99"`
	RiakSearchVnodeq100    *int64 `json:"riak_search_vnodeq_100" stm:"riak_search_vnodeq_100"`

	SearchIndexFailCount *int64 `json:"search_index_fail_count" stm:"search_index_fail_count"`
	PbcActive            *int64 `json:"pbc_active" stm:"pbc_active"`
	ReadRepairs          *int64 `json:"read_repairs" stm:"read_repairs"`

	NodeGetFsmActive *int64 `json:"node_get_fsm_active" stm:"node_get_fsm_active"`
	NodePutFsmActive *int64 `json:"node_put_fsm_active" stm:"node_put_fsm_active"`
	IndexFsmActive   *int64 `json:"index_fsm_active" stm:"index_fsm_active"`
	ListFsmActive    *int64 `json:"list_fsm_active" stm:"list_fsm_active"`

	NodeGetFsmRejected *int64 `json:"node_get_fsm_rejected" stm:"node_get_fsm_rejected"`
	NodePutFsmRejected *int64 `json:"node_put_fsm_rejected" stm:"node_put_fsm_rejected"`

	SearchIndexBadEntryCount    *int64 `json:"search_index_bad_entry_count" stm:"search_index_bad_entry_count"`
	SearchIndexExtractFailCount *int64 `json:"search_index_extract_fail_count" stm:"search_index_extract_fail_count"`
}
