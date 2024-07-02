// SPDX-License-Identifier: GPL-3.0-or-later

package elasticsearch

// https://www.elastic.co/guide/en/elasticsearch/reference/current/docker.html

type esMetrics struct {
	// https://www.elastic.co/guide/en/elasticsearch/reference/current/cluster-nodes-stats.html
	NodesStats *esNodesStats
	// https://www.elastic.co/guide/en/elasticsearch/reference/current/cluster-health.html
	ClusterHealth *esClusterHealth
	// https://www.elastic.co/guide/en/elasticsearch/reference/current/cluster-stats.html
	ClusterStats *esClusterStats
	// https://www.elastic.co/guide/en/elasticsearch/reference/current/cat-indices.html
	LocalIndicesStats []esIndexStats
}

func (m esMetrics) empty() bool {
	switch {
	case m.hasNodesStats(), m.hasClusterHealth(), m.hasClusterStats(), m.hasLocalIndicesStats():
		return false
	}
	return true
}

func (m esMetrics) hasNodesStats() bool        { return m.NodesStats != nil && len(m.NodesStats.Nodes) > 0 }
func (m esMetrics) hasClusterHealth() bool     { return m.ClusterHealth != nil }
func (m esMetrics) hasClusterStats() bool      { return m.ClusterStats != nil }
func (m esMetrics) hasLocalIndicesStats() bool { return len(m.LocalIndicesStats) > 0 }

type (
	esNodesStats struct {
		ClusterName string                  `json:"cluster_name"`
		Nodes       map[string]*esNodeStats `json:"nodes"`
	}
	esNodeStats struct {
		Name    string
		Host    string
		Indices struct {
			Indexing struct {
				IndexTotal        float64 `stm:"index_total" json:"index_total"`
				IndexCurrent      float64 `stm:"index_current" json:"index_current"`
				IndexTimeInMillis float64 `stm:"index_time_in_millis" json:"index_time_in_millis"`
			} `stm:"indexing"`
			Search struct {
				FetchTotal        float64 `stm:"fetch_total" json:"fetch_total"`
				FetchCurrent      float64 `stm:"fetch_current" json:"fetch_current"`
				FetchTimeInMillis float64 `stm:"fetch_time_in_millis" json:"fetch_time_in_millis"`
				QueryTotal        float64 `stm:"query_total" json:"query_total"`
				QueryCurrent      float64 `stm:"query_current" json:"query_current"`
				QueryTimeInMillis float64 `stm:"query_time_in_millis" json:"query_time_in_millis"`
			} `stm:"search"`
			Refresh struct {
				Total        float64 `stm:"total"`
				TimeInMillis float64 `stm:"total_time_in_millis" json:"total_time_in_millis"`
			} `stm:"refresh"`
			Flush struct {
				Total        float64 `stm:"total"`
				TimeInMillis float64 `stm:"total_time_in_millis" json:"total_time_in_millis"`
			} `stm:"flush"`
			FieldData struct {
				MemorySizeInBytes float64 `stm:"memory_size_in_bytes" json:"memory_size_in_bytes"`
				Evictions         float64 `stm:"evictions"`
			} `stm:"fielddata"`
			Segments struct {
				Count                     float64 `stm:"count" json:"count"`
				MemoryInBytes             float64 `stm:"memory_in_bytes" json:"memory_in_bytes"`
				TermsMemoryInBytes        float64 `stm:"terms_memory_in_bytes" json:"terms_memory_in_bytes"`
				StoredFieldsMemoryInBytes float64 `stm:"stored_fields_memory_in_bytes" json:"stored_fields_memory_in_bytes"`
				TermVectorsMemoryInBytes  float64 `stm:"term_vectors_memory_in_bytes" json:"term_vectors_memory_in_bytes"`
				NormsMemoryInBytes        float64 `stm:"norms_memory_in_bytes" json:"norms_memory_in_bytes"`
				PointsMemoryInBytes       float64 `stm:"points_memory_in_bytes" json:"points_memory_in_bytes"`
				DocValuesMemoryInBytes    float64 `stm:"doc_values_memory_in_bytes" json:"doc_values_memory_in_bytes"`
				IndexWriterMemoryInBytes  float64 `stm:"index_writer_memory_in_bytes" json:"index_writer_memory_in_bytes"`
				VersionMapMemoryInBytes   float64 `stm:"version_map_memory_in_bytes" json:"version_map_memory_in_bytes"`
				FixedBitSetMemoryInBytes  float64 `stm:"fixed_bit_set_memory_in_bytes" json:"fixed_bit_set_memory_in_bytes"`
			} `stm:"segments"`
			Translog struct {
				Operations             float64 `stm:"operations"`
				SizeInBytes            float64 `stm:"size_in_bytes" json:"size_in_bytes"`
				UncommittedOperations  float64 `stm:"uncommitted_operations" json:"uncommitted_operations"`
				UncommittedSizeInBytes float64 `stm:"uncommitted_size_in_bytes" json:"uncommitted_size_in_bytes"`
			} `stm:"translog"`
		} `stm:"indices"`
		Process struct {
			OpenFileDescriptors float64 `stm:"open_file_descriptors" json:"open_file_descriptors"`
			MaxFileDescriptors  float64 `stm:"max_file_descriptors" json:"max_file_descriptors"`
		} `stm:"process"`
		JVM struct {
			Mem struct {
				HeapUsedPercent      float64 `stm:"heap_used_percent" json:"heap_used_percent"`
				HeapUsedInBytes      float64 `stm:"heap_used_in_bytes" json:"heap_used_in_bytes"`
				HeapCommittedInBytes float64 `stm:"heap_committed_in_bytes" json:"heap_committed_in_bytes"`
			} `stm:"mem"`
			GC struct {
				Collectors struct {
					Young struct {
						CollectionCount        float64 `stm:"collection_count" json:"collection_count"`
						CollectionTimeInMillis float64 `stm:"collection_time_in_millis" json:"collection_time_in_millis"`
					} `stm:"young"`
					Old struct {
						CollectionCount        float64 `stm:"collection_count" json:"collection_count"`
						CollectionTimeInMillis float64 `stm:"collection_time_in_millis" json:"collection_time_in_millis"`
					} `stm:"old"`
				} `stm:"collectors"`
			} `stm:"gc"`
			BufferPools struct {
				Mapped struct {
					Count                float64 `stm:"count"`
					UsedInBytes          float64 `stm:"used_in_bytes" json:"used_in_bytes"`
					TotalCapacityInBytes float64 `stm:"total_capacity_in_bytes" json:"total_capacity_in_bytes"`
				} `stm:"mapped"`
				Direct struct {
					Count                float64 `stm:"count"`
					UsedInBytes          float64 `stm:"used_in_bytes" json:"used_in_bytes"`
					TotalCapacityInBytes float64 `stm:"total_capacity_in_bytes" json:"total_capacity_in_bytes"`
				} `stm:"direct"`
			} `stm:"buffer_pools" json:"buffer_pools"`
		} `stm:"jvm"`
		// https://www.elastic.co/guide/en/elasticsearch/reference/current/modules-threadpool.html
		ThreadPool struct {
			Generic struct {
				Queue    float64 `stm:"queue"`
				Rejected float64 `stm:"rejected"`
			} `stm:"generic"`
			Search struct {
				Queue    float64 `stm:"queue"`
				Rejected float64 `stm:"rejected"`
			} `stm:"search"`
			SearchThrottled struct {
				Queue    float64 `stm:"queue"`
				Rejected float64 `stm:"rejected"`
			} `stm:"search_throttled" json:"search_throttled"`
			Get struct {
				Queue    float64 `stm:"queue"`
				Rejected float64 `stm:"rejected"`
			} `stm:"get"`
			Analyze struct {
				Queue    float64 `stm:"queue"`
				Rejected float64 `stm:"rejected"`
			} `stm:"analyze"`
			Write struct {
				Queue    float64 `stm:"queue"`
				Rejected float64 `stm:"rejected"`
			} `stm:"write"`
			Snapshot struct {
				Queue    float64 `stm:"queue"`
				Rejected float64 `stm:"rejected"`
			} `stm:"snapshot"`
			Warmer struct {
				Queue    float64 `stm:"queue"`
				Rejected float64 `stm:"rejected"`
			} `stm:"warmer"`
			Refresh struct {
				Queue    float64 `stm:"queue"`
				Rejected float64 `stm:"rejected"`
			} `stm:"refresh"`
			Listener struct {
				Queue    float64 `stm:"queue"`
				Rejected float64 `stm:"rejected"`
			} `stm:"listener"`
			FetchShardStarted struct {
				Queue    float64 `stm:"queue"`
				Rejected float64 `stm:"rejected"`
			} `stm:"fetch_shard_started" json:"fetch_shard_started"`
			FetchShardStore struct {
				Queue    float64 `stm:"queue"`
				Rejected float64 `stm:"rejected"`
			} `stm:"fetch_shard_store" json:"fetch_shard_store"`
			Flush struct {
				Queue    float64 `stm:"queue"`
				Rejected float64 `stm:"rejected"`
			} `stm:"flush"`
			ForceMerge struct {
				Queue    float64 `stm:"queue"`
				Rejected float64 `stm:"rejected"`
			} `stm:"force_merge" json:"force_merge"`
			Management struct {
				Queue    float64 `stm:"queue"`
				Rejected float64 `stm:"rejected"`
			} `stm:"management"`
		} `stm:"thread_pool" json:"thread_pool"`
		Transport struct {
			RxCount       float64 `stm:"rx_count" json:"rx_count"`
			RxSizeInBytes float64 `stm:"rx_size_in_bytes" json:"rx_size_in_bytes"`
			TxCount       float64 `stm:"tx_count" json:"tx_count"`
			TxSizeInBytes float64 `stm:"tx_size_in_bytes" json:"tx_size_in_bytes"`
		} `stm:"transport"`
		HTTP struct {
			CurrentOpen float64 `stm:"current_open" json:"current_open"`
		} `stm:"http"`
		Breakers struct {
			Request struct {
				Tripped float64 `stm:"tripped"`
			} `stm:"request"`
			FieldData struct {
				Tripped float64 `stm:"tripped"`
			} `stm:"fielddata"`
			InFlightRequests struct {
				Tripped float64 `stm:"tripped"`
			} `stm:"in_flight_requests" json:"in_flight_requests"`
			ModelInference struct {
				Tripped float64 `stm:"tripped"`
			} `stm:"model_inference" json:"model_inference"`
			Accounting struct {
				Tripped float64 `stm:"tripped"`
			} `stm:"accounting"`
			Parent struct {
				Tripped float64 `stm:"tripped"`
			} `stm:"parent"`
		} `stm:"breakers"`
	}
)

type esClusterHealth struct {
	ClusterName                 string `json:"cluster_name"`
	Status                      string
	NumOfNodes                  float64 `stm:"number_of_nodes" json:"number_of_nodes"`
	NumOfDataNodes              float64 `stm:"number_of_data_nodes" json:"number_of_data_nodes"`
	ActivePrimaryShards         float64 `stm:"active_primary_shards" json:"active_primary_shards"`
	ActiveShards                float64 `stm:"active_shards" json:"active_shards"`
	RelocatingShards            float64 `stm:"relocating_shards" json:"relocating_shards"`
	InitializingShards          float64 `stm:"initializing_shards" json:"initializing_shards"`
	UnassignedShards            float64 `stm:"unassigned_shards" json:"unassigned_shards"`
	DelayedUnassignedShards     float64 `stm:"delayed_unassigned_shards" json:"delayed_unassigned_shards"`
	NumOfPendingTasks           float64 `stm:"number_of_pending_tasks" json:"number_of_pending_tasks"`
	NumOfInFlightFetch          float64 `stm:"number_of_in_flight_fetch" json:"number_of_in_flight_fetch"`
	ActiveShardsPercentAsNumber float64 `stm:"active_shards_percent_as_number" json:"active_shards_percent_as_number"`
}

type esClusterStats struct {
	ClusterName string `json:"cluster_name"`
	Nodes       struct {
		Count struct {
			Total               float64 `stm:"total"`
			CoordinatingOnly    float64 `stm:"coordinating_only" json:"coordinating_only"`
			Data                float64 `stm:"data"`
			DataCold            float64 `stm:"data_cold" json:"data_cold"`
			DataContent         float64 `stm:"data_content" json:"data_content"`
			DataFrozen          float64 `stm:"data_frozen" json:"data_frozen"`
			DataHot             float64 `stm:"data_hot" json:"data_hot"`
			DataWarm            float64 `stm:"data_warm" json:"data_warm"`
			Ingest              float64 `stm:"ingest"`
			Master              float64 `stm:"master"`
			ML                  float64 `stm:"ml"`
			RemoteClusterClient float64 `stm:"remote_cluster_client" json:"remote_cluster_client"`
			Transform           float64 `stm:"transform"`
			VotingOnly          float64 `stm:"voting_only" json:"voting_only"`
		} `stm:"count"`
	} `stm:"nodes"`
	Indices struct {
		Count  float64 `stm:"count"`
		Shards struct {
			Total       float64 `stm:"total"`
			Primaries   float64 `stm:"primaries"`
			Replication float64 `stm:"replication"`
		} `stm:"shards"`
		Docs struct {
			Count float64 `stm:"count"`
		} `stm:"docs"`
		Store struct {
			SizeInBytes float64 `stm:"size_in_bytes" json:"size_in_bytes"`
		} `stm:"store"`
		QueryCache struct {
			HitCount  float64 `stm:"hit_count" json:"hit_count"`
			MissCount float64 `stm:"miss_count" json:"miss_count"`
		} `stm:"query_cache" json:"query_cache"`
	} `stm:"indices"`
}

type esIndexStats struct {
	Index     string
	Health    string
	Rep       string
	DocsCount string `json:"docs.count"`
	StoreSize string `json:"store.size"`
}
