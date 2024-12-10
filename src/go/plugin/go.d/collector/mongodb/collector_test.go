// SPDX-License-Identifier: GPL-3.0-or-later

package mongo

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataVer6MongodServerStatus, _ = os.ReadFile("testdata/v6.0.3/mongod-serverStatus.json")
	dataVer6MongosServerStatus, _ = os.ReadFile("testdata/v6.0.3/mongos-serverStatus.json")
	dataVer6DbStats, _            = os.ReadFile("testdata/v6.0.3/dbStats.json")
	dataVer6ReplSetGetStatus, _   = os.ReadFile("testdata/v6.0.3/replSetGetStatus.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":             dataConfigJSON,
		"dataConfigYAML":             dataConfigYAML,
		"dataVer6MongodServerStatus": dataVer6MongodServerStatus,
		"dataVer6MongosServerStatus": dataVer6MongosServerStatus,
		"dataVer6DbStats":            dataVer6DbStats,
		"dataVer6ReplSetGetStatus":   dataVer6ReplSetGetStatus,
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
		"success on default config": {
			wantFail: false,
			config:   New().Config,
		},
		"fails on unset 'address'": {
			wantFail: true,
			config: Config{
				URI: "",
			},
		},
		"fails on invalid database selector": {
			wantFail: true,
			config: Config{
				URI: "mongodb://localhost:27017",
				Databases: matcher.SimpleExpr{
					Includes: []string{"!@#"},
				},
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

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare   func(t *testing.T) *Collector
		wantClose bool
	}{
		"client not initialized": {
			wantClose: false,
			prepare: func(t *testing.T) *Collector {
				return New()
			},
		},
		"client initialized": {
			wantClose: true,
			prepare: func(t *testing.T) *Collector {
				collr := New()
				collr.conn = caseMongod()
				_ = collr.conn.initClient("", 0)

				return collr
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare(t)

			require.NotPanics(t, func() { collr.Cleanup(context.Background()) })
			if test.wantClose {
				collr, ok := collr.conn.(*mockMongoClient)
				require.True(t, ok)
				assert.True(t, collr.closeCalled)
			}
		})
	}
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() *mockMongoClient
		wantFail bool
	}{
		"success on Mongod (v6)": {
			wantFail: false,
			prepare:  caseMongod,
		},
		"success on Mongod Replicas Set(v6)": {
			wantFail: false,
			prepare:  caseMongodReplicaSet,
		},
		"success on Mongos (v6)": {
			wantFail: false,
			prepare:  caseMongos,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := prepareMongo()
			defer collr.Cleanup(context.Background())
			collr.conn = test.prepare()

			require.NoError(t, collr.Init(context.Background()))

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func() *mockMongoClient
		wantCollected map[string]int64
	}{
		"success on Mongod (v6)": {
			prepare: caseMongod,
			wantCollected: map[string]int64{
				"asserts_msg":                                              0,
				"asserts_regular":                                          0,
				"asserts_rollovers":                                        0,
				"asserts_tripwire":                                         0,
				"asserts_user":                                             246,
				"asserts_warning":                                          0,
				"connections_active":                                       7,
				"connections_available":                                    838841,
				"connections_awaiting_topology_changes":                    5,
				"connections_current":                                      19,
				"connections_exhaust_hello":                                2,
				"connections_exhaust_is_master":                            1,
				"connections_threaded":                                     19,
				"connections_total_created":                                77,
				"database_admin_collections":                               3,
				"database_admin_data_size":                                 796,
				"database_admin_documents":                                 5,
				"database_admin_index_size":                                81920,
				"database_admin_indexes":                                   4,
				"database_admin_storage_size":                              61440,
				"database_admin_views":                                     0,
				"database_config_collections":                              3,
				"database_config_data_size":                                796,
				"database_config_documents":                                5,
				"database_config_index_size":                               81920,
				"database_config_indexes":                                  4,
				"database_config_storage_size":                             61440,
				"database_config_views":                                    0,
				"database_local_collections":                               3,
				"database_local_data_size":                                 796,
				"database_local_documents":                                 5,
				"database_local_index_size":                                81920,
				"database_local_indexes":                                   4,
				"database_local_storage_size":                              61440,
				"database_local_views":                                     0,
				"extra_info_page_faults":                                   0,
				"global_lock_active_clients_readers":                       0,
				"global_lock_active_clients_writers":                       0,
				"global_lock_current_queue_readers":                        0,
				"global_lock_current_queue_writers":                        0,
				"locks_collection_acquire_exclusive":                       6,
				"locks_collection_acquire_intent_exclusive":                172523,
				"locks_collection_acquire_intent_shared":                   336370,
				"locks_collection_acquire_shared":                          0,
				"locks_database_acquire_exclusive":                         3,
				"locks_database_acquire_intent_exclusive":                  172539,
				"locks_database_acquire_intent_shared":                     50971,
				"locks_database_acquire_shared":                            0,
				"locks_global_acquire_exclusive":                           6,
				"locks_global_acquire_intent_exclusive":                    174228,
				"locks_global_acquire_intent_shared":                       437905,
				"locks_global_acquire_shared":                              0,
				"locks_mutex_acquire_exclusive":                            0,
				"locks_mutex_acquire_intent_exclusive":                     0,
				"locks_mutex_acquire_intent_shared":                        245077,
				"locks_mutex_acquire_shared":                               0,
				"locks_oplog_acquire_exclusive":                            0,
				"locks_oplog_acquire_intent_exclusive":                     1,
				"locks_oplog_acquire_intent_shared":                        16788,
				"locks_oplog_acquire_shared":                               0,
				"memory_resident":                                          193986560,
				"memory_virtual":                                           3023044608,
				"metrics_cursor_lifespan_greater_than_or_equal_10_minutes": 0,
				"metrics_cursor_lifespan_less_than_10_minutes":             0,
				"metrics_cursor_lifespan_less_than_15_seconds":             0,
				"metrics_cursor_lifespan_less_than_1_minute":               0,
				"metrics_cursor_lifespan_less_than_1_second":               0,
				"metrics_cursor_lifespan_less_than_30_seconds":             0,
				"metrics_cursor_lifespan_less_than_5_seconds":              0,
				"metrics_cursor_open_no_timeout":                           0,
				"metrics_cursor_open_total":                                1,
				"metrics_cursor_timed_out":                                 0,
				"metrics_cursor_total_opened":                              1,
				"metrics_document_deleted":                                 7,
				"metrics_document_inserted":                                0,
				"metrics_document_returned":                                1699,
				"metrics_document_updated":                                 52,
				"metrics_query_executor_scanned":                           61,
				"metrics_query_executor_scanned_objects":                   1760,
				"network_bytes_in":                                         38851356,
				"network_bytes_out":                                        706335836,
				"network_requests":                                         130530,
				"network_slow_dns_operations":                              0,
				"network_slow_ssl_operations":                              0,
				"operations_command":                                       125531,
				"operations_delete":                                        7,
				"operations_getmore":                                       5110,
				"operations_insert":                                        0,
				"operations_latencies_commands_latency":                    46432082,
				"operations_latencies_commands_ops":                        125412,
				"operations_latencies_reads_latency":                       1009868,
				"operations_latencies_reads_ops":                           5111,
				"operations_latencies_writes_latency":                      0,
				"operations_latencies_writes_ops":                          0,
				"operations_query":                                         76,
				"operations_update":                                        59,
				"tcmalloc_aggressive_memory_decommit":                      0,
				"tcmalloc_central_cache_free_bytes":                        406680,
				"tcmalloc_current_total_thread_cache_bytes":                2490832,
				"tcmalloc_generic_current_allocated_bytes":                 109050648,
				"tcmalloc_generic_heap_size":                               127213568,
				"tcmalloc_max_total_thread_cache_bytes":                    1073741824,
				"tcmalloc_pageheap_commit_count":                           376,
				"tcmalloc_pageheap_committed_bytes":                        127086592,
				"tcmalloc_pageheap_decommit_count":                         122,
				"tcmalloc_pageheap_free_bytes":                             13959168,
				"tcmalloc_pageheap_reserve_count":                          60,
				"tcmalloc_pageheap_scavenge_bytes":                         0,
				"tcmalloc_pageheap_total_commit_bytes":                     229060608,
				"tcmalloc_pageheap_total_decommit_bytes":                   101974016,
				"tcmalloc_pageheap_total_reserve_bytes":                    127213568,
				"tcmalloc_pageheap_unmapped_bytes":                         126976,
				"tcmalloc_spinlock_total_delay_ns":                         33426251,
				"tcmalloc_thread_cache_free_bytes":                         2490832,
				"tcmalloc_total_free_bytes":                                4076776,
				"tcmalloc_transfer_cache_free_bytes":                       1179264,
				"txn_active":                                               0,
				"txn_inactive":                                             0,
				"txn_open":                                                 0,
				"txn_prepared":                                             0,
				"txn_total_aborted":                                        0,
				"txn_total_committed":                                      0,
				"txn_total_prepared":                                       0,
				"txn_total_started":                                        0,
				"wiredtiger_cache_currently_in_cache_bytes":                814375,
				"wiredtiger_cache_maximum_configured_bytes":                7854882816,
				"wiredtiger_cache_modified_evicted_pages":                  0,
				"wiredtiger_cache_read_into_cache_pages":                   108,
				"wiredtiger_cache_tracked_dirty_in_the_cache_bytes":        456446,
				"wiredtiger_cache_unmodified_evicted_pages":                0,
				"wiredtiger_cache_written_from_cache_pages":                3177,
				"wiredtiger_concurrent_txn_read_available":                 128,
				"wiredtiger_concurrent_txn_read_out":                       0,
				"wiredtiger_concurrent_txn_write_available":                128,
				"wiredtiger_concurrent_txn_write_out":                      0,
			},
		},
		"success on Mongod Replica Set (v6)": {
			prepare: caseMongodReplicaSet,
			wantCollected: map[string]int64{
				"asserts_msg":                                                0,
				"asserts_regular":                                            0,
				"asserts_rollovers":                                          0,
				"asserts_tripwire":                                           0,
				"asserts_user":                                               246,
				"asserts_warning":                                            0,
				"connections_active":                                         7,
				"connections_available":                                      838841,
				"connections_awaiting_topology_changes":                      5,
				"connections_current":                                        19,
				"connections_exhaust_hello":                                  2,
				"connections_exhaust_is_master":                              1,
				"connections_threaded":                                       19,
				"connections_total_created":                                  77,
				"database_admin_collections":                                 3,
				"database_admin_data_size":                                   796,
				"database_admin_documents":                                   5,
				"database_admin_index_size":                                  81920,
				"database_admin_indexes":                                     4,
				"database_admin_storage_size":                                61440,
				"database_admin_views":                                       0,
				"database_config_collections":                                3,
				"database_config_data_size":                                  796,
				"database_config_documents":                                  5,
				"database_config_index_size":                                 81920,
				"database_config_indexes":                                    4,
				"database_config_storage_size":                               61440,
				"database_config_views":                                      0,
				"database_local_collections":                                 3,
				"database_local_data_size":                                   796,
				"database_local_documents":                                   5,
				"database_local_index_size":                                  81920,
				"database_local_indexes":                                     4,
				"database_local_storage_size":                                61440,
				"database_local_views":                                       0,
				"extra_info_page_faults":                                     0,
				"global_lock_active_clients_readers":                         0,
				"global_lock_active_clients_writers":                         0,
				"global_lock_current_queue_readers":                          0,
				"global_lock_current_queue_writers":                          0,
				"locks_collection_acquire_exclusive":                         6,
				"locks_collection_acquire_intent_exclusive":                  172523,
				"locks_collection_acquire_intent_shared":                     336370,
				"locks_collection_acquire_shared":                            0,
				"locks_database_acquire_exclusive":                           3,
				"locks_database_acquire_intent_exclusive":                    172539,
				"locks_database_acquire_intent_shared":                       50971,
				"locks_database_acquire_shared":                              0,
				"locks_global_acquire_exclusive":                             6,
				"locks_global_acquire_intent_exclusive":                      174228,
				"locks_global_acquire_intent_shared":                         437905,
				"locks_global_acquire_shared":                                0,
				"locks_mutex_acquire_exclusive":                              0,
				"locks_mutex_acquire_intent_exclusive":                       0,
				"locks_mutex_acquire_intent_shared":                          245077,
				"locks_mutex_acquire_shared":                                 0,
				"locks_oplog_acquire_exclusive":                              0,
				"locks_oplog_acquire_intent_exclusive":                       1,
				"locks_oplog_acquire_intent_shared":                          16788,
				"locks_oplog_acquire_shared":                                 0,
				"memory_resident":                                            193986560,
				"memory_virtual":                                             3023044608,
				"metrics_cursor_lifespan_greater_than_or_equal_10_minutes":   0,
				"metrics_cursor_lifespan_less_than_10_minutes":               0,
				"metrics_cursor_lifespan_less_than_15_seconds":               0,
				"metrics_cursor_lifespan_less_than_1_minute":                 0,
				"metrics_cursor_lifespan_less_than_1_second":                 0,
				"metrics_cursor_lifespan_less_than_30_seconds":               0,
				"metrics_cursor_lifespan_less_than_5_seconds":                0,
				"metrics_cursor_open_no_timeout":                             0,
				"metrics_cursor_open_total":                                  1,
				"metrics_cursor_timed_out":                                   0,
				"metrics_cursor_total_opened":                                1,
				"metrics_document_deleted":                                   7,
				"metrics_document_inserted":                                  0,
				"metrics_document_returned":                                  1699,
				"metrics_document_updated":                                   52,
				"metrics_query_executor_scanned":                             61,
				"metrics_query_executor_scanned_objects":                     1760,
				"network_bytes_in":                                           38851356,
				"network_bytes_out":                                          706335836,
				"network_requests":                                           130530,
				"network_slow_dns_operations":                                0,
				"network_slow_ssl_operations":                                0,
				"operations_command":                                         125531,
				"operations_delete":                                          7,
				"operations_getmore":                                         5110,
				"operations_insert":                                          0,
				"operations_latencies_commands_latency":                      46432082,
				"operations_latencies_commands_ops":                          125412,
				"operations_latencies_reads_latency":                         1009868,
				"operations_latencies_reads_ops":                             5111,
				"operations_latencies_writes_latency":                        0,
				"operations_latencies_writes_ops":                            0,
				"operations_query":                                           76,
				"operations_update":                                          59,
				"repl_set_member_mongodb-primary:27017_health_status_down":   0,
				"repl_set_member_mongodb-primary:27017_health_status_up":     1,
				"repl_set_member_mongodb-primary:27017_replication_lag":      4572,
				"repl_set_member_mongodb-primary:27017_state_arbiter":        0,
				"repl_set_member_mongodb-primary:27017_state_down":           0,
				"repl_set_member_mongodb-primary:27017_state_primary":        1,
				"repl_set_member_mongodb-primary:27017_state_recovering":     0,
				"repl_set_member_mongodb-primary:27017_state_removed":        0,
				"repl_set_member_mongodb-primary:27017_state_rollback":       0,
				"repl_set_member_mongodb-primary:27017_state_secondary":      0,
				"repl_set_member_mongodb-primary:27017_state_startup":        0,
				"repl_set_member_mongodb-primary:27017_state_startup2":       0,
				"repl_set_member_mongodb-primary:27017_state_unknown":        0,
				"repl_set_member_mongodb-secondary:27017_health_status_down": 0,
				"repl_set_member_mongodb-secondary:27017_health_status_up":   1,
				"repl_set_member_mongodb-secondary:27017_heartbeat_latency":  1359,
				"repl_set_member_mongodb-secondary:27017_ping_rtt":           0,
				"repl_set_member_mongodb-secondary:27017_replication_lag":    4572,
				"repl_set_member_mongodb-secondary:27017_state_arbiter":      0,
				"repl_set_member_mongodb-secondary:27017_state_down":         0,
				"repl_set_member_mongodb-secondary:27017_state_primary":      0,
				"repl_set_member_mongodb-secondary:27017_state_recovering":   0,
				"repl_set_member_mongodb-secondary:27017_state_removed":      0,
				"repl_set_member_mongodb-secondary:27017_state_rollback":     0,
				"repl_set_member_mongodb-secondary:27017_state_secondary":    1,
				"repl_set_member_mongodb-secondary:27017_state_startup":      0,
				"repl_set_member_mongodb-secondary:27017_state_startup2":     0,
				"repl_set_member_mongodb-secondary:27017_state_unknown":      0,
				"repl_set_member_mongodb-secondary:27017_uptime":             192370,
				"tcmalloc_aggressive_memory_decommit":                        0,
				"tcmalloc_central_cache_free_bytes":                          406680,
				"tcmalloc_current_total_thread_cache_bytes":                  2490832,
				"tcmalloc_generic_current_allocated_bytes":                   109050648,
				"tcmalloc_generic_heap_size":                                 127213568,
				"tcmalloc_max_total_thread_cache_bytes":                      1073741824,
				"tcmalloc_pageheap_commit_count":                             376,
				"tcmalloc_pageheap_committed_bytes":                          127086592,
				"tcmalloc_pageheap_decommit_count":                           122,
				"tcmalloc_pageheap_free_bytes":                               13959168,
				"tcmalloc_pageheap_reserve_count":                            60,
				"tcmalloc_pageheap_scavenge_bytes":                           0,
				"tcmalloc_pageheap_total_commit_bytes":                       229060608,
				"tcmalloc_pageheap_total_decommit_bytes":                     101974016,
				"tcmalloc_pageheap_total_reserve_bytes":                      127213568,
				"tcmalloc_pageheap_unmapped_bytes":                           126976,
				"tcmalloc_spinlock_total_delay_ns":                           33426251,
				"tcmalloc_thread_cache_free_bytes":                           2490832,
				"tcmalloc_total_free_bytes":                                  4076776,
				"tcmalloc_transfer_cache_free_bytes":                         1179264,
				"txn_active":                                                 0,
				"txn_inactive":                                               0,
				"txn_open":                                                   0,
				"txn_prepared":                                               0,
				"txn_total_aborted":                                          0,
				"txn_total_committed":                                        0,
				"txn_total_prepared":                                         0,
				"txn_total_started":                                          0,
				"wiredtiger_cache_currently_in_cache_bytes":                  814375,
				"wiredtiger_cache_maximum_configured_bytes":                  7854882816,
				"wiredtiger_cache_modified_evicted_pages":                    0,
				"wiredtiger_cache_read_into_cache_pages":                     108,
				"wiredtiger_cache_tracked_dirty_in_the_cache_bytes":          456446,
				"wiredtiger_cache_unmodified_evicted_pages":                  0,
				"wiredtiger_cache_written_from_cache_pages":                  3177,
				"wiredtiger_concurrent_txn_read_available":                   128,
				"wiredtiger_concurrent_txn_read_out":                         0,
				"wiredtiger_concurrent_txn_write_available":                  128,
				"wiredtiger_concurrent_txn_write_out":                        0,
			},
		},
		"success on Mongos (v6)": {
			prepare: caseMongos,
			wantCollected: map[string]int64{
				"asserts_msg":                                                    0,
				"asserts_regular":                                                0,
				"asserts_rollovers":                                              0,
				"asserts_tripwire":                                               0,
				"asserts_user":                                                   352,
				"asserts_warning":                                                0,
				"connections_active":                                             5,
				"connections_available":                                          838842,
				"connections_awaiting_topology_changes":                          4,
				"connections_current":                                            18,
				"connections_exhaust_hello":                                      3,
				"connections_exhaust_is_master":                                  0,
				"connections_threaded":                                           18,
				"connections_total_created":                                      89,
				"database_admin_collections":                                     3,
				"database_admin_data_size":                                       796,
				"database_admin_documents":                                       5,
				"database_admin_index_size":                                      81920,
				"database_admin_indexes":                                         4,
				"database_admin_storage_size":                                    61440,
				"database_admin_views":                                           0,
				"database_config_collections":                                    3,
				"database_config_data_size":                                      796,
				"database_config_documents":                                      5,
				"database_config_index_size":                                     81920,
				"database_config_indexes":                                        4,
				"database_config_storage_size":                                   61440,
				"database_config_views":                                          0,
				"database_local_collections":                                     3,
				"database_local_data_size":                                       796,
				"database_local_documents":                                       5,
				"database_local_index_size":                                      81920,
				"database_local_indexes":                                         4,
				"database_local_storage_size":                                    61440,
				"database_local_views":                                           0,
				"extra_info_page_faults":                                         526,
				"memory_resident":                                                84934656,
				"memory_virtual":                                                 2596274176,
				"metrics_document_deleted":                                       0,
				"metrics_document_inserted":                                      0,
				"metrics_document_returned":                                      0,
				"metrics_document_updated":                                       0,
				"metrics_query_executor_scanned":                                 0,
				"metrics_query_executor_scanned_objects":                         0,
				"network_bytes_in":                                               57943348,
				"network_bytes_out":                                              247343709,
				"network_requests":                                               227310,
				"network_slow_dns_operations":                                    0,
				"network_slow_ssl_operations":                                    0,
				"operations_command":                                             227283,
				"operations_delete":                                              0,
				"operations_getmore":                                             0,
				"operations_insert":                                              0,
				"operations_query":                                               10,
				"operations_update":                                              0,
				"shard_collections_partitioned":                                  1,
				"shard_collections_unpartitioned":                                1,
				"shard_databases_partitioned":                                    1,
				"shard_databases_unpartitioned":                                  1,
				"shard_id_shard0_chunks":                                         1,
				"shard_id_shard1_chunks":                                         1,
				"shard_nodes_aware":                                              1,
				"shard_nodes_unaware":                                            1,
				"tcmalloc_aggressive_memory_decommit":                            0,
				"tcmalloc_central_cache_free_bytes":                              736960,
				"tcmalloc_current_total_thread_cache_bytes":                      1638104,
				"tcmalloc_generic_current_allocated_bytes":                       13519784,
				"tcmalloc_generic_heap_size":                                     24576000,
				"tcmalloc_max_total_thread_cache_bytes":                          1042284544,
				"tcmalloc_pageheap_commit_count":                                 480,
				"tcmalloc_pageheap_committed_bytes":                              24518656,
				"tcmalloc_pageheap_decommit_count":                               127,
				"tcmalloc_pageheap_free_bytes":                                   5697536,
				"tcmalloc_pageheap_reserve_count":                                15,
				"tcmalloc_pageheap_scavenge_bytes":                               0,
				"tcmalloc_pageheap_total_commit_bytes":                           84799488,
				"tcmalloc_pageheap_total_decommit_bytes":                         60280832,
				"tcmalloc_pageheap_total_reserve_bytes":                          24576000,
				"tcmalloc_pageheap_unmapped_bytes":                               57344,
				"tcmalloc_spinlock_total_delay_ns":                               96785212,
				"tcmalloc_thread_cache_free_bytes":                               1638104,
				"tcmalloc_total_free_bytes":                                      5301336,
				"tcmalloc_transfer_cache_free_bytes":                             2926272,
				"txn_active":                                                     0,
				"txn_commit_types_no_shards_initiated":                           0,
				"txn_commit_types_no_shards_successful":                          0,
				"txn_commit_types_no_shards_successful_duration_micros":          0,
				"txn_commit_types_no_shards_unsuccessful":                        0,
				"txn_commit_types_read_only_initiated":                           0,
				"txn_commit_types_read_only_successful":                          0,
				"txn_commit_types_read_only_successful_duration_micros":          0,
				"txn_commit_types_read_only_unsuccessful":                        0,
				"txn_commit_types_recover_with_token_initiated":                  0,
				"txn_commit_types_recover_with_token_successful":                 0,
				"txn_commit_types_recover_with_token_successful_duration_micros": 0,
				"txn_commit_types_recover_with_token_unsuccessful":               0,
				"txn_commit_types_single_shard_initiated":                        0,
				"txn_commit_types_single_shard_successful":                       0,
				"txn_commit_types_single_shard_successful_duration_micros":       0,
				"txn_commit_types_single_shard_unsuccessful":                     0,
				"txn_commit_types_single_write_shard_initiated":                  0,
				"txn_commit_types_single_write_shard_successful":                 0,
				"txn_commit_types_single_write_shard_successful_duration_micros": 0,
				"txn_commit_types_single_write_shard_unsuccessful":               0,
				"txn_commit_types_two_phase_commit_initiated":                    0,
				"txn_commit_types_two_phase_commit_successful":                   0,
				"txn_commit_types_two_phase_commit_successful_duration_micros":   0,
				"txn_commit_types_two_phase_commit_unsuccessful":                 0,
				"txn_inactive":                                                   0,
				"txn_open":                                                       0,
				"txn_total_aborted":                                              0,
				"txn_total_committed":                                            0,
				"txn_total_started":                                              0,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := prepareMongo()
			defer collr.Cleanup(context.Background())
			collr.conn = test.prepare()

			require.NoError(t, collr.Init(context.Background()))

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantCollected, mx)
		})
	}
}

func prepareMongo() *Collector {
	collr := New()
	collr.Databases = matcher.SimpleExpr{Includes: []string{"* *"}}
	return collr
}

func caseMongodReplicaSet() *mockMongoClient {
	return &mockMongoClient{replicaSet: true}
}

func caseMongod() *mockMongoClient {
	return &mockMongoClient{}
}

func caseMongos() *mockMongoClient {
	return &mockMongoClient{mongos: true}
}

type mockMongoClient struct {
	replicaSet                        bool
	mongos                            bool
	errOnServerStatus                 bool
	errOnListDatabaseNames            bool
	errOnDbStats                      bool
	errOnReplSetGetStatus             bool
	errOnShardNodes                   bool
	errOnShardDatabasesPartitioning   bool
	errOnShardCollectionsPartitioning bool
	errOnShardChunks                  bool
	errOnInitClient                   bool
	clientInited                      bool
	closeCalled                       bool
}

func (m *mockMongoClient) serverStatus() (*documentServerStatus, error) {
	if !m.clientInited {
		return nil, errors.New("mock.serverStatus() error: mongo client not inited")
	}
	if m.errOnServerStatus {
		return nil, errors.New("mock.serverStatus() error")
	}

	data := dataVer6MongodServerStatus
	if m.mongos {
		data = dataVer6MongosServerStatus
	}

	var s documentServerStatus
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}

	return &s, nil
}

func (m *mockMongoClient) listDatabaseNames() ([]string, error) {
	if !m.clientInited {
		return nil, errors.New("mock.listDatabaseNames() error: mongo client not inited")
	}
	if m.errOnListDatabaseNames {
		return nil, errors.New("mock.listDatabaseNames() error")
	}
	return []string{"admin", "config", "local"}, nil
}

func (m *mockMongoClient) dbStats(_ string) (*documentDBStats, error) {
	if !m.clientInited {
		return nil, errors.New("mock.dbStats() error: mongo client not inited")
	}
	if m.errOnDbStats {
		return nil, errors.New("mock.dbStats() error")
	}

	var s documentDBStats
	if err := json.Unmarshal(dataVer6DbStats, &s); err != nil {
		return nil, err
	}

	return &s, nil
}

func (m *mockMongoClient) isReplicaSet() bool {
	return m.replicaSet
}

func (m *mockMongoClient) isMongos() bool {
	return m.mongos
}

func (m *mockMongoClient) replSetGetStatus() (*documentReplSetStatus, error) {
	if !m.clientInited {
		return nil, errors.New("mock.replSetGetStatus() error: mongo client not inited")
	}
	if m.mongos {
		return nil, errors.New("mock.replSetGetStatus() error: shouldn't be called for mongos")
	}
	if !m.replicaSet {
		return nil, errors.New("mock.replSetGetStatus() error: should be called for replica set")
	}
	if m.errOnReplSetGetStatus {
		return nil, errors.New("mock.replSetGetStatus() error")
	}

	var s documentReplSetStatus
	if err := json.Unmarshal(dataVer6ReplSetGetStatus, &s); err != nil {
		return nil, err
	}

	return &s, nil
}

func (m *mockMongoClient) shardNodes() (*documentShardNodesResult, error) {
	if !m.clientInited {
		return nil, errors.New("mock.shardNodes() error: mongo client not inited")
	}
	if m.replicaSet {
		return nil, errors.New("mock.replSetGetStatus() error: shouldn't be called for replica set")
	}
	if !m.mongos {
		return nil, errors.New("mock.shardNodes() error: should be called for mongos")
	}
	if m.errOnShardNodes {
		return nil, errors.New("mock.shardNodes() error")
	}

	return &documentShardNodesResult{
		ShardAware:   1,
		ShardUnaware: 1,
	}, nil
}

func (m *mockMongoClient) shardDatabasesPartitioning() (*documentPartitionedResult, error) {
	if !m.clientInited {
		return nil, errors.New("mock.shardDatabasesPartitioning() error: mongo client not inited")
	}
	if m.replicaSet {
		return nil, errors.New("mock.shardDatabasesPartitioning() error: shouldn't be called for replica set")
	}
	if !m.mongos {
		return nil, errors.New("mock.shardDatabasesPartitioning() error: should be called for mongos")
	}
	if m.errOnShardDatabasesPartitioning {
		return nil, errors.New("mock.shardDatabasesPartitioning() error")
	}

	return &documentPartitionedResult{
		Partitioned:   1,
		UnPartitioned: 1,
	}, nil
}

func (m *mockMongoClient) shardCollectionsPartitioning() (*documentPartitionedResult, error) {
	if !m.clientInited {
		return nil, errors.New("mock.shardCollectionsPartitioning() error: mongo client not inited")
	}
	if m.replicaSet {
		return nil, errors.New("mock.shardCollectionsPartitioning() error: shouldn't be called for replica set")
	}
	if !m.mongos {
		return nil, errors.New("mock.shardCollectionsPartitioning() error: should be called for mongos")
	}
	if m.errOnShardCollectionsPartitioning {
		return nil, errors.New("mock.shardCollectionsPartitioning() error")
	}

	return &documentPartitionedResult{
		Partitioned:   1,
		UnPartitioned: 1,
	}, nil
}

func (m *mockMongoClient) shardChunks() (map[string]int64, error) {
	if !m.clientInited {
		return nil, errors.New("mock.shardChunks() error: mongo client not inited")
	}
	if m.replicaSet {
		return nil, errors.New("mock.shardChunks() error: shouldn't be called for replica set")
	}
	if !m.mongos {
		return nil, errors.New("mock.shardChunks() error: should be called for mongos")
	}
	if m.errOnShardChunks {
		return nil, errors.New("mock.shardChunks() error")
	}

	return map[string]int64{
		"shard0": 1,
		"shard1": 1,
	}, nil
}

func (m *mockMongoClient) initClient(_ string, _ time.Duration) error {
	if m.errOnInitClient {
		return errors.New("mock.initClient() error")
	}
	m.clientInited = true
	return nil
}

func (m *mockMongoClient) close() error {
	if m.clientInited {
		m.closeCalled = true
	}
	return nil
}
