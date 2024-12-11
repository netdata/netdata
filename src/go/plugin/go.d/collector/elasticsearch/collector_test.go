// SPDX-License-Identifier: GPL-3.0-or-later

package elasticsearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataVer842NodesLocalStats, _ = os.ReadFile("testdata/v8.4.2/nodes_local_stats.json")
	dataVer842NodesStats, _      = os.ReadFile("testdata/v8.4.2/nodes_stats.json")
	dataVer842ClusterHealth, _   = os.ReadFile("testdata/v8.4.2/cluster_health.json")
	dataVer842ClusterStats, _    = os.ReadFile("testdata/v8.4.2/cluster_stats.json")
	dataVer842CatIndicesStats, _ = os.ReadFile("testdata/v8.4.2/cat_indices_stats.json")
	dataVer842Info, _            = os.ReadFile("testdata/v8.4.2/info.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":            dataConfigJSON,
		"dataConfigYAML":            dataConfigYAML,
		"dataVer842NodesLocalStats": dataVer842NodesLocalStats,
		"dataVer842NodesStats":      dataVer842NodesStats,
		"dataVer842ClusterHealth":   dataVer842ClusterHealth,
		"dataVer842ClusterStats":    dataVer842ClusterStats,
		"dataVer842CatIndicesStats": dataVer842CatIndicesStats,
		"dataVer842Info":            dataVer842Info,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"default": {
			config: New().Config,
		},
		"all stats": {
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:38001"},
				},
				DoNodeStats:     true,
				DoClusterHealth: true,
				DoClusterStats:  true,
				DoIndicesStats:  true,
			},
		},
		"only node_stats": {
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:38001"},
				},
				DoNodeStats:     true,
				DoClusterHealth: false,
				DoClusterStats:  false,
				DoIndicesStats:  false,
			},
		},
		"URL not set": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: ""},
				}},
		},
		"invalid TLSCA": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{
					ClientConfig: web.ClientConfig{
						TLSConfig: tlscfg.TLSConfig{TLSCA: "testdata/tls"},
					},
				}},
		},
		"all API calls are disabled": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:38001"},
				},
				DoNodeStats:     false,
				DoClusterHealth: false,
				DoClusterStats:  false,
				DoIndicesStats:  false,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = test.config

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				assert.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func(*testing.T) (collr *Collector, cleanup func())
		wantFail bool
	}{
		"valid data":         {prepare: prepareElasticsearchValidData},
		"invalid data":       {prepare: prepareElasticsearchInvalidData, wantFail: true},
		"404":                {prepare: prepareElasticsearch404, wantFail: true},
		"connection refused": {prepare: prepareElasticsearchConnectionRefused, wantFail: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare(t)
			defer cleanup()

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Cleanup(t *testing.T) {
	assert.NotPanics(t, func() { New().Cleanup(context.Background()) })
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func() *Collector
		wantCollected map[string]int64
		wantCharts    int
	}{
		"v842: all nodes stats": {
			prepare: func() *Collector {
				collr := New()
				collr.ClusterMode = true
				collr.DoNodeStats = true
				collr.DoClusterHealth = false
				collr.DoClusterStats = false
				collr.DoIndicesStats = false
				return collr
			},
			wantCharts: len(nodeChartsTmpl) * 3,
			wantCollected: map[string]int64{
				"node_Klg1CjgMTouentQcJlRGuA_breakers_accounting_tripped":                       0,
				"node_Klg1CjgMTouentQcJlRGuA_breakers_fielddata_tripped":                        0,
				"node_Klg1CjgMTouentQcJlRGuA_breakers_in_flight_requests_tripped":               0,
				"node_Klg1CjgMTouentQcJlRGuA_breakers_model_inference_tripped":                  0,
				"node_Klg1CjgMTouentQcJlRGuA_breakers_parent_tripped":                           0,
				"node_Klg1CjgMTouentQcJlRGuA_breakers_request_tripped":                          0,
				"node_Klg1CjgMTouentQcJlRGuA_http_current_open":                                 75,
				"node_Klg1CjgMTouentQcJlRGuA_indices_fielddata_evictions":                       0,
				"node_Klg1CjgMTouentQcJlRGuA_indices_fielddata_memory_size_in_bytes":            600,
				"node_Klg1CjgMTouentQcJlRGuA_indices_flush_total":                               35130,
				"node_Klg1CjgMTouentQcJlRGuA_indices_flush_total_time_in_millis":                22204637,
				"node_Klg1CjgMTouentQcJlRGuA_indices_indexing_index_current":                    0,
				"node_Klg1CjgMTouentQcJlRGuA_indices_indexing_index_time_in_millis":             1100012973,
				"node_Klg1CjgMTouentQcJlRGuA_indices_indexing_index_total":                      3667364815,
				"node_Klg1CjgMTouentQcJlRGuA_indices_refresh_total":                             7720800,
				"node_Klg1CjgMTouentQcJlRGuA_indices_refresh_total_time_in_millis":              94297737,
				"node_Klg1CjgMTouentQcJlRGuA_indices_search_fetch_current":                      0,
				"node_Klg1CjgMTouentQcJlRGuA_indices_search_fetch_time_in_millis":               21316723,
				"node_Klg1CjgMTouentQcJlRGuA_indices_search_fetch_total":                        42642621,
				"node_Klg1CjgMTouentQcJlRGuA_indices_search_query_current":                      0,
				"node_Klg1CjgMTouentQcJlRGuA_indices_search_query_time_in_millis":               51262303,
				"node_Klg1CjgMTouentQcJlRGuA_indices_search_query_total":                        166820275,
				"node_Klg1CjgMTouentQcJlRGuA_indices_segments_count":                            320,
				"node_Klg1CjgMTouentQcJlRGuA_indices_segments_doc_values_memory_in_bytes":       0,
				"node_Klg1CjgMTouentQcJlRGuA_indices_segments_fixed_bit_set_memory_in_bytes":    1904,
				"node_Klg1CjgMTouentQcJlRGuA_indices_segments_index_writer_memory_in_bytes":     262022568,
				"node_Klg1CjgMTouentQcJlRGuA_indices_segments_memory_in_bytes":                  0,
				"node_Klg1CjgMTouentQcJlRGuA_indices_segments_norms_memory_in_bytes":            0,
				"node_Klg1CjgMTouentQcJlRGuA_indices_segments_points_memory_in_bytes":           0,
				"node_Klg1CjgMTouentQcJlRGuA_indices_segments_stored_fields_memory_in_bytes":    0,
				"node_Klg1CjgMTouentQcJlRGuA_indices_segments_term_vectors_memory_in_bytes":     0,
				"node_Klg1CjgMTouentQcJlRGuA_indices_segments_terms_memory_in_bytes":            0,
				"node_Klg1CjgMTouentQcJlRGuA_indices_segments_version_map_memory_in_bytes":      49200018,
				"node_Klg1CjgMTouentQcJlRGuA_indices_translog_operations":                       352376,
				"node_Klg1CjgMTouentQcJlRGuA_indices_translog_size_in_bytes":                    447695989,
				"node_Klg1CjgMTouentQcJlRGuA_indices_translog_uncommitted_operations":           352376,
				"node_Klg1CjgMTouentQcJlRGuA_indices_translog_uncommitted_size_in_bytes":        447695989,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_buffer_pools_direct_count":                     94,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_buffer_pools_direct_total_capacity_in_bytes":   4654848,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_buffer_pools_direct_used_in_bytes":             4654850,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_buffer_pools_mapped_count":                     858,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_buffer_pools_mapped_total_capacity_in_bytes":   103114998135,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_buffer_pools_mapped_used_in_bytes":             103114998135,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_gc_collectors_old_collection_count":            0,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_gc_collectors_old_collection_time_in_millis":   0,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_gc_collectors_young_collection_count":          78652,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_gc_collectors_young_collection_time_in_millis": 6014274,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_mem_heap_committed_in_bytes":                   7864320000,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_mem_heap_used_in_bytes":                        5059735552,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_mem_heap_used_percent":                         64,
				"node_Klg1CjgMTouentQcJlRGuA_process_max_file_descriptors":                      1048576,
				"node_Klg1CjgMTouentQcJlRGuA_process_open_file_descriptors":                     1156,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_analyze_queue":                         0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_analyze_rejected":                      0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_fetch_shard_started_queue":             0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_fetch_shard_started_rejected":          0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_fetch_shard_store_queue":               0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_fetch_shard_store_rejected":            0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_flush_queue":                           0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_flush_rejected":                        0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_force_merge_queue":                     0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_force_merge_rejected":                  0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_generic_queue":                         0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_generic_rejected":                      0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_get_queue":                             0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_get_rejected":                          0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_listener_queue":                        0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_listener_rejected":                     0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_management_queue":                      0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_management_rejected":                   0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_refresh_queue":                         0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_refresh_rejected":                      0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_search_queue":                          0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_search_rejected":                       0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_search_throttled_queue":                0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_search_throttled_rejected":             0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_snapshot_queue":                        0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_snapshot_rejected":                     0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_warmer_queue":                          0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_warmer_rejected":                       0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_write_queue":                           0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_write_rejected":                        0,
				"node_Klg1CjgMTouentQcJlRGuA_transport_rx_count":                                1300324276,
				"node_Klg1CjgMTouentQcJlRGuA_transport_rx_size_in_bytes":                        1789333458217,
				"node_Klg1CjgMTouentQcJlRGuA_transport_tx_count":                                1300324275,
				"node_Klg1CjgMTouentQcJlRGuA_transport_tx_size_in_bytes":                        2927487680282,
				"node_k_AifYMWQTykjUq3pgE_-w_breakers_accounting_tripped":                       0,
				"node_k_AifYMWQTykjUq3pgE_-w_breakers_fielddata_tripped":                        0,
				"node_k_AifYMWQTykjUq3pgE_-w_breakers_in_flight_requests_tripped":               0,
				"node_k_AifYMWQTykjUq3pgE_-w_breakers_model_inference_tripped":                  0,
				"node_k_AifYMWQTykjUq3pgE_-w_breakers_parent_tripped":                           0,
				"node_k_AifYMWQTykjUq3pgE_-w_breakers_request_tripped":                          0,
				"node_k_AifYMWQTykjUq3pgE_-w_http_current_open":                                 14,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_fielddata_evictions":                       0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_fielddata_memory_size_in_bytes":            0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_flush_total":                               0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_flush_total_time_in_millis":                0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_indexing_index_current":                    0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_indexing_index_time_in_millis":             0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_indexing_index_total":                      0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_refresh_total":                             0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_refresh_total_time_in_millis":              0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_search_fetch_current":                      0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_search_fetch_time_in_millis":               0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_search_fetch_total":                        0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_search_query_current":                      0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_search_query_time_in_millis":               0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_search_query_total":                        0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_segments_count":                            0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_segments_doc_values_memory_in_bytes":       0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_segments_fixed_bit_set_memory_in_bytes":    0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_segments_index_writer_memory_in_bytes":     0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_segments_memory_in_bytes":                  0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_segments_norms_memory_in_bytes":            0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_segments_points_memory_in_bytes":           0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_segments_stored_fields_memory_in_bytes":    0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_segments_term_vectors_memory_in_bytes":     0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_segments_terms_memory_in_bytes":            0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_segments_version_map_memory_in_bytes":      0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_translog_operations":                       0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_translog_size_in_bytes":                    0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_translog_uncommitted_operations":           0,
				"node_k_AifYMWQTykjUq3pgE_-w_indices_translog_uncommitted_size_in_bytes":        0,
				"node_k_AifYMWQTykjUq3pgE_-w_jvm_buffer_pools_direct_count":                     19,
				"node_k_AifYMWQTykjUq3pgE_-w_jvm_buffer_pools_direct_total_capacity_in_bytes":   2142214,
				"node_k_AifYMWQTykjUq3pgE_-w_jvm_buffer_pools_direct_used_in_bytes":             2142216,
				"node_k_AifYMWQTykjUq3pgE_-w_jvm_buffer_pools_mapped_count":                     0,
				"node_k_AifYMWQTykjUq3pgE_-w_jvm_buffer_pools_mapped_total_capacity_in_bytes":   0,
				"node_k_AifYMWQTykjUq3pgE_-w_jvm_buffer_pools_mapped_used_in_bytes":             0,
				"node_k_AifYMWQTykjUq3pgE_-w_jvm_gc_collectors_old_collection_count":            0,
				"node_k_AifYMWQTykjUq3pgE_-w_jvm_gc_collectors_old_collection_time_in_millis":   0,
				"node_k_AifYMWQTykjUq3pgE_-w_jvm_gc_collectors_young_collection_count":          342994,
				"node_k_AifYMWQTykjUq3pgE_-w_jvm_gc_collectors_young_collection_time_in_millis": 768917,
				"node_k_AifYMWQTykjUq3pgE_-w_jvm_mem_heap_committed_in_bytes":                   281018368,
				"node_k_AifYMWQTykjUq3pgE_-w_jvm_mem_heap_used_in_bytes":                        178362704,
				"node_k_AifYMWQTykjUq3pgE_-w_jvm_mem_heap_used_percent":                         63,
				"node_k_AifYMWQTykjUq3pgE_-w_process_max_file_descriptors":                      1048576,
				"node_k_AifYMWQTykjUq3pgE_-w_process_open_file_descriptors":                     557,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_analyze_queue":                         0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_analyze_rejected":                      0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_fetch_shard_started_queue":             0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_fetch_shard_started_rejected":          0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_fetch_shard_store_queue":               0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_fetch_shard_store_rejected":            0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_flush_queue":                           0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_flush_rejected":                        0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_force_merge_queue":                     0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_force_merge_rejected":                  0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_generic_queue":                         0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_generic_rejected":                      0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_get_queue":                             0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_get_rejected":                          0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_listener_queue":                        0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_listener_rejected":                     0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_management_queue":                      0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_management_rejected":                   0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_refresh_queue":                         0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_refresh_rejected":                      0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_search_queue":                          0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_search_rejected":                       0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_search_throttled_queue":                0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_search_throttled_rejected":             0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_snapshot_queue":                        0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_snapshot_rejected":                     0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_warmer_queue":                          0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_warmer_rejected":                       0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_write_queue":                           0,
				"node_k_AifYMWQTykjUq3pgE_-w_thread_pool_write_rejected":                        0,
				"node_k_AifYMWQTykjUq3pgE_-w_transport_rx_count":                                107632996,
				"node_k_AifYMWQTykjUq3pgE_-w_transport_rx_size_in_bytes":                        180620082152,
				"node_k_AifYMWQTykjUq3pgE_-w_transport_tx_count":                                107633007,
				"node_k_AifYMWQTykjUq3pgE_-w_transport_tx_size_in_bytes":                        420999501235,
				"node_tk_U7GMCRkCG4FoOvusrng_breakers_accounting_tripped":                       0,
				"node_tk_U7GMCRkCG4FoOvusrng_breakers_fielddata_tripped":                        0,
				"node_tk_U7GMCRkCG4FoOvusrng_breakers_in_flight_requests_tripped":               0,
				"node_tk_U7GMCRkCG4FoOvusrng_breakers_model_inference_tripped":                  0,
				"node_tk_U7GMCRkCG4FoOvusrng_breakers_parent_tripped":                           93,
				"node_tk_U7GMCRkCG4FoOvusrng_breakers_request_tripped":                          1,
				"node_tk_U7GMCRkCG4FoOvusrng_http_current_open":                                 84,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_fielddata_evictions":                       0,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_fielddata_memory_size_in_bytes":            0,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_flush_total":                               67895,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_flush_total_time_in_millis":                81917283,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_indexing_index_current":                    0,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_indexing_index_time_in_millis":             1244633519,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_indexing_index_total":                      6550378755,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_refresh_total":                             12359783,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_refresh_total_time_in_millis":              300152615,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_search_fetch_current":                      0,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_search_fetch_time_in_millis":               24517851,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_search_fetch_total":                        25105951,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_search_query_current":                      0,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_search_query_time_in_millis":               158980385,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_search_query_total":                        157912598,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_segments_count":                            291,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_segments_doc_values_memory_in_bytes":       0,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_segments_fixed_bit_set_memory_in_bytes":    55672,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_segments_index_writer_memory_in_bytes":     57432664,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_segments_memory_in_bytes":                  0,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_segments_norms_memory_in_bytes":            0,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_segments_points_memory_in_bytes":           0,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_segments_stored_fields_memory_in_bytes":    0,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_segments_term_vectors_memory_in_bytes":     0,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_segments_terms_memory_in_bytes":            0,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_segments_version_map_memory_in_bytes":      568,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_translog_operations":                       1449698,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_translog_size_in_bytes":                    1214204014,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_translog_uncommitted_operations":           1449698,
				"node_tk_U7GMCRkCG4FoOvusrng_indices_translog_uncommitted_size_in_bytes":        1214204014,
				"node_tk_U7GMCRkCG4FoOvusrng_jvm_buffer_pools_direct_count":                     90,
				"node_tk_U7GMCRkCG4FoOvusrng_jvm_buffer_pools_direct_total_capacity_in_bytes":   4571711,
				"node_tk_U7GMCRkCG4FoOvusrng_jvm_buffer_pools_direct_used_in_bytes":             4571713,
				"node_tk_U7GMCRkCG4FoOvusrng_jvm_buffer_pools_mapped_count":                     831,
				"node_tk_U7GMCRkCG4FoOvusrng_jvm_buffer_pools_mapped_total_capacity_in_bytes":   99844219805,
				"node_tk_U7GMCRkCG4FoOvusrng_jvm_buffer_pools_mapped_used_in_bytes":             99844219805,
				"node_tk_U7GMCRkCG4FoOvusrng_jvm_gc_collectors_old_collection_count":            1,
				"node_tk_U7GMCRkCG4FoOvusrng_jvm_gc_collectors_old_collection_time_in_millis":   796,
				"node_tk_U7GMCRkCG4FoOvusrng_jvm_gc_collectors_young_collection_count":          139959,
				"node_tk_U7GMCRkCG4FoOvusrng_jvm_gc_collectors_young_collection_time_in_millis": 3581668,
				"node_tk_U7GMCRkCG4FoOvusrng_jvm_mem_heap_committed_in_bytes":                   7864320000,
				"node_tk_U7GMCRkCG4FoOvusrng_jvm_mem_heap_used_in_bytes":                        1884124192,
				"node_tk_U7GMCRkCG4FoOvusrng_jvm_mem_heap_used_percent":                         23,
				"node_tk_U7GMCRkCG4FoOvusrng_process_max_file_descriptors":                      1048576,
				"node_tk_U7GMCRkCG4FoOvusrng_process_open_file_descriptors":                     1180,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_analyze_queue":                         0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_analyze_rejected":                      0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_fetch_shard_started_queue":             0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_fetch_shard_started_rejected":          0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_fetch_shard_store_queue":               0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_fetch_shard_store_rejected":            0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_flush_queue":                           0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_flush_rejected":                        0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_force_merge_queue":                     0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_force_merge_rejected":                  0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_generic_queue":                         0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_generic_rejected":                      0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_get_queue":                             0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_get_rejected":                          0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_listener_queue":                        0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_listener_rejected":                     0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_management_queue":                      0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_management_rejected":                   0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_refresh_queue":                         0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_refresh_rejected":                      0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_search_queue":                          0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_search_rejected":                       0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_search_throttled_queue":                0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_search_throttled_rejected":             0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_snapshot_queue":                        0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_snapshot_rejected":                     0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_warmer_queue":                          0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_warmer_rejected":                       0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_write_queue":                           0,
				"node_tk_U7GMCRkCG4FoOvusrng_thread_pool_write_rejected":                        0,
				"node_tk_U7GMCRkCG4FoOvusrng_transport_rx_count":                                2167879292,
				"node_tk_U7GMCRkCG4FoOvusrng_transport_rx_size_in_bytes":                        4905919297323,
				"node_tk_U7GMCRkCG4FoOvusrng_transport_tx_count":                                2167879293,
				"node_tk_U7GMCRkCG4FoOvusrng_transport_tx_size_in_bytes":                        2964638852652,
			},
		},
		"v842: local node stats": {
			prepare: func() *Collector {
				collr := New()
				collr.DoNodeStats = true
				collr.DoClusterHealth = false
				collr.DoClusterStats = false
				collr.DoIndicesStats = false
				return collr
			},
			wantCharts: len(nodeChartsTmpl),
			wantCollected: map[string]int64{
				"node_Klg1CjgMTouentQcJlRGuA_breakers_accounting_tripped":                       0,
				"node_Klg1CjgMTouentQcJlRGuA_breakers_fielddata_tripped":                        0,
				"node_Klg1CjgMTouentQcJlRGuA_breakers_in_flight_requests_tripped":               0,
				"node_Klg1CjgMTouentQcJlRGuA_breakers_model_inference_tripped":                  0,
				"node_Klg1CjgMTouentQcJlRGuA_breakers_parent_tripped":                           0,
				"node_Klg1CjgMTouentQcJlRGuA_breakers_request_tripped":                          0,
				"node_Klg1CjgMTouentQcJlRGuA_http_current_open":                                 73,
				"node_Klg1CjgMTouentQcJlRGuA_indices_fielddata_evictions":                       0,
				"node_Klg1CjgMTouentQcJlRGuA_indices_fielddata_memory_size_in_bytes":            600,
				"node_Klg1CjgMTouentQcJlRGuA_indices_flush_total":                               35134,
				"node_Klg1CjgMTouentQcJlRGuA_indices_flush_total_time_in_millis":                22213090,
				"node_Klg1CjgMTouentQcJlRGuA_indices_indexing_index_current":                    1,
				"node_Klg1CjgMTouentQcJlRGuA_indices_indexing_index_time_in_millis":             1100149051,
				"node_Klg1CjgMTouentQcJlRGuA_indices_indexing_index_total":                      3667793202,
				"node_Klg1CjgMTouentQcJlRGuA_indices_refresh_total":                             7721472,
				"node_Klg1CjgMTouentQcJlRGuA_indices_refresh_total_time_in_millis":              94304142,
				"node_Klg1CjgMTouentQcJlRGuA_indices_search_fetch_current":                      0,
				"node_Klg1CjgMTouentQcJlRGuA_indices_search_fetch_time_in_millis":               21316820,
				"node_Klg1CjgMTouentQcJlRGuA_indices_search_fetch_total":                        42645288,
				"node_Klg1CjgMTouentQcJlRGuA_indices_search_query_current":                      0,
				"node_Klg1CjgMTouentQcJlRGuA_indices_search_query_time_in_millis":               51265805,
				"node_Klg1CjgMTouentQcJlRGuA_indices_search_query_total":                        166823028,
				"node_Klg1CjgMTouentQcJlRGuA_indices_segments_count":                            307,
				"node_Klg1CjgMTouentQcJlRGuA_indices_segments_doc_values_memory_in_bytes":       0,
				"node_Klg1CjgMTouentQcJlRGuA_indices_segments_fixed_bit_set_memory_in_bytes":    2008,
				"node_Klg1CjgMTouentQcJlRGuA_indices_segments_index_writer_memory_in_bytes":     240481008,
				"node_Klg1CjgMTouentQcJlRGuA_indices_segments_memory_in_bytes":                  0,
				"node_Klg1CjgMTouentQcJlRGuA_indices_segments_norms_memory_in_bytes":            0,
				"node_Klg1CjgMTouentQcJlRGuA_indices_segments_points_memory_in_bytes":           0,
				"node_Klg1CjgMTouentQcJlRGuA_indices_segments_stored_fields_memory_in_bytes":    0,
				"node_Klg1CjgMTouentQcJlRGuA_indices_segments_term_vectors_memory_in_bytes":     0,
				"node_Klg1CjgMTouentQcJlRGuA_indices_segments_terms_memory_in_bytes":            0,
				"node_Klg1CjgMTouentQcJlRGuA_indices_segments_version_map_memory_in_bytes":      44339216,
				"node_Klg1CjgMTouentQcJlRGuA_indices_translog_operations":                       362831,
				"node_Klg1CjgMTouentQcJlRGuA_indices_translog_size_in_bytes":                    453491882,
				"node_Klg1CjgMTouentQcJlRGuA_indices_translog_uncommitted_operations":           362831,
				"node_Klg1CjgMTouentQcJlRGuA_indices_translog_uncommitted_size_in_bytes":        453491882,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_buffer_pools_direct_count":                     94,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_buffer_pools_direct_total_capacity_in_bytes":   4654848,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_buffer_pools_direct_used_in_bytes":             4654850,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_buffer_pools_mapped_count":                     844,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_buffer_pools_mapped_total_capacity_in_bytes":   103411995802,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_buffer_pools_mapped_used_in_bytes":             103411995802,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_gc_collectors_old_collection_count":            0,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_gc_collectors_old_collection_time_in_millis":   0,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_gc_collectors_young_collection_count":          78661,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_gc_collectors_young_collection_time_in_millis": 6014901,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_mem_heap_committed_in_bytes":                   7864320000,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_mem_heap_used_in_bytes":                        4337402488,
				"node_Klg1CjgMTouentQcJlRGuA_jvm_mem_heap_used_percent":                         55,
				"node_Klg1CjgMTouentQcJlRGuA_process_max_file_descriptors":                      1048576,
				"node_Klg1CjgMTouentQcJlRGuA_process_open_file_descriptors":                     1149,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_analyze_queue":                         0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_analyze_rejected":                      0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_fetch_shard_started_queue":             0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_fetch_shard_started_rejected":          0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_fetch_shard_store_queue":               0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_fetch_shard_store_rejected":            0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_flush_queue":                           0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_flush_rejected":                        0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_force_merge_queue":                     0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_force_merge_rejected":                  0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_generic_queue":                         0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_generic_rejected":                      0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_get_queue":                             0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_get_rejected":                          0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_listener_queue":                        0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_listener_rejected":                     0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_management_queue":                      0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_management_rejected":                   0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_refresh_queue":                         0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_refresh_rejected":                      0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_search_queue":                          0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_search_rejected":                       0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_search_throttled_queue":                0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_search_throttled_rejected":             0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_snapshot_queue":                        0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_snapshot_rejected":                     0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_warmer_queue":                          0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_warmer_rejected":                       0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_write_queue":                           0,
				"node_Klg1CjgMTouentQcJlRGuA_thread_pool_write_rejected":                        0,
				"node_Klg1CjgMTouentQcJlRGuA_transport_rx_count":                                1300468666,
				"node_Klg1CjgMTouentQcJlRGuA_transport_rx_size_in_bytes":                        1789647854011,
				"node_Klg1CjgMTouentQcJlRGuA_transport_tx_count":                                1300468665,
				"node_Klg1CjgMTouentQcJlRGuA_transport_tx_size_in_bytes":                        2927853534431,
			},
		},
		"v842: only cluster_health": {
			prepare: func() *Collector {
				collr := New()
				collr.DoNodeStats = false
				collr.DoClusterHealth = true
				collr.DoClusterStats = false
				collr.DoIndicesStats = false
				return collr
			},
			wantCharts: len(clusterHealthChartsTmpl),
			wantCollected: map[string]int64{
				"cluster_active_primary_shards":           97,
				"cluster_active_shards":                   194,
				"cluster_active_shards_percent_as_number": 100,
				"cluster_delayed_unassigned_shards":       0,
				"cluster_initializing_shards":             0,
				"cluster_number_of_data_nodes":            2,
				"cluster_number_of_in_flight_fetch":       0,
				"cluster_number_of_nodes":                 3,
				"cluster_number_of_pending_tasks":         0,
				"cluster_relocating_shards":               0,
				"cluster_status_green":                    1,
				"cluster_status_red":                      0,
				"cluster_status_yellow":                   0,
				"cluster_unassigned_shards":               0,
			},
		},
		"v842: only cluster_stats": {
			prepare: func() *Collector {
				collr := New()
				collr.DoNodeStats = false
				collr.DoClusterHealth = false
				collr.DoClusterStats = true
				collr.DoIndicesStats = false
				return collr
			},
			wantCharts: len(clusterStatsChartsTmpl),
			wantCollected: map[string]int64{
				"cluster_indices_count":                     97,
				"cluster_indices_docs_count":                402750703,
				"cluster_indices_query_cache_hit_count":     96838726,
				"cluster_indices_query_cache_miss_count":    587768226,
				"cluster_indices_shards_primaries":          97,
				"cluster_indices_shards_replication":        1,
				"cluster_indices_shards_total":              194,
				"cluster_indices_store_size_in_bytes":       380826136962,
				"cluster_nodes_count_coordinating_only":     0,
				"cluster_nodes_count_data":                  0,
				"cluster_nodes_count_data_cold":             0,
				"cluster_nodes_count_data_content":          2,
				"cluster_nodes_count_data_frozen":           0,
				"cluster_nodes_count_data_hot":              2,
				"cluster_nodes_count_data_warm":             0,
				"cluster_nodes_count_ingest":                2,
				"cluster_nodes_count_master":                3,
				"cluster_nodes_count_ml":                    0,
				"cluster_nodes_count_remote_cluster_client": 2,
				"cluster_nodes_count_total":                 3,
				"cluster_nodes_count_transform":             2,
				"cluster_nodes_count_voting_only":           1,
			},
		},
		"v842: only indices_stats": {
			prepare: func() *Collector {
				collr := New()
				collr.DoNodeStats = false
				collr.DoClusterHealth = false
				collr.DoClusterStats = false
				collr.DoIndicesStats = true
				return collr
			},
			wantCharts: len(nodeIndexChartsTmpl) * 3,
			wantCollected: map[string]int64{
				"node_index_my-index-000001_stats_docs_count":          1,
				"node_index_my-index-000001_stats_health_green":        0,
				"node_index_my-index-000001_stats_health_red":          0,
				"node_index_my-index-000001_stats_health_yellow":       1,
				"node_index_my-index-000001_stats_shards_count":        1,
				"node_index_my-index-000001_stats_store_size_in_bytes": 208,
				"node_index_my-index-000002_stats_docs_count":          1,
				"node_index_my-index-000002_stats_health_green":        0,
				"node_index_my-index-000002_stats_health_red":          0,
				"node_index_my-index-000002_stats_health_yellow":       1,
				"node_index_my-index-000002_stats_shards_count":        1,
				"node_index_my-index-000002_stats_store_size_in_bytes": 208,
				"node_index_my-index-000003_stats_docs_count":          1,
				"node_index_my-index-000003_stats_health_green":        0,
				"node_index_my-index-000003_stats_health_red":          0,
				"node_index_my-index-000003_stats_health_yellow":       1,
				"node_index_my-index-000003_stats_shards_count":        1,
				"node_index_my-index-000003_stats_store_size_in_bytes": 208,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := prepareElasticsearch(t, test.prepare)
			defer cleanup()

			var mx map[string]int64
			for i := 0; i < 10; i++ {
				mx = collr.Collect(context.Background())
			}

			assert.Equal(t, test.wantCollected, mx)
			assert.Len(t, *collr.Charts(), test.wantCharts)
			module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
		})
	}
}

func prepareElasticsearch(t *testing.T, createES func() *Collector) (collr *Collector, cleanup func()) {
	t.Helper()
	srv := prepareElasticsearchEndpoint()

	collr = createES()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func prepareElasticsearchValidData(t *testing.T) (es *Collector, cleanup func()) {
	return prepareElasticsearch(t, New)
}

func prepareElasticsearchInvalidData(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))
	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func prepareElasticsearch404(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func prepareElasticsearchConnectionRefused(t *testing.T) (*Collector, func()) {
	t.Helper()
	collr := New()
	collr.URL = "http://127.0.0.1:38001"
	require.NoError(t, collr.Init(context.Background()))

	return collr, func() {}
}

func prepareElasticsearchEndpoint() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case urlPathNodesStats:
				_, _ = w.Write(dataVer842NodesStats)
			case urlPathLocalNodeStats:
				_, _ = w.Write(dataVer842NodesLocalStats)
			case urlPathClusterHealth:
				_, _ = w.Write(dataVer842ClusterHealth)
			case urlPathClusterStats:
				_, _ = w.Write(dataVer842ClusterStats)
			case urlPathIndicesStats:
				_, _ = w.Write(dataVer842CatIndicesStats)
			case "/":
				_, _ = w.Write(dataVer842Info)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
}
