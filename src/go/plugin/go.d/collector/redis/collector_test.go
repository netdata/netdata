// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"context"
	"errors"
	"maps"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataPikaInfoAll, _   = os.ReadFile("testdata/pika/info_all.txt")
	dataVer609InfoAll, _ = os.ReadFile("testdata/v6.0.9/info_all.txt")

	dataGarnetInfoAll, _             = os.ReadFile("testdata/garnet/info_all.txt")
	dataGarnetInfoKeyspace, _        = os.ReadFile("testdata/garnet/info_keyspace.txt")
	dataGarnetInfoCommandstats, _    = os.ReadFile("testdata/garnet/info_commandstats.txt")
	dataGarnetInfoCommandstatsOff, _ = os.ReadFile("testdata/garnet/info_commandstats_disabled.txt")

	dataValkeyInfoAll, _    = os.ReadFile("testdata/valkey/info_all.txt")
	dataDragonflyInfoAll, _ = os.ReadFile("testdata/dragonfly/info_all.txt")
	dataKeydbInfoAll, _     = os.ReadFile("testdata/keydb/info_all.txt")
	dataKvrocksInfoAll, _   = os.ReadFile("testdata/kvrocks/info_all.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":                dataConfigJSON,
		"dataConfigYAML":                dataConfigYAML,
		"dataPikaInfoAll":               dataPikaInfoAll,
		"dataVer609InfoAll":             dataVer609InfoAll,
		"dataGarnetInfoAll":             dataGarnetInfoAll,
		"dataGarnetInfoKeyspace":        dataGarnetInfoKeyspace,
		"dataGarnetInfoCommandstats":    dataGarnetInfoCommandstats,
		"dataGarnetInfoCommandstatsOff": dataGarnetInfoCommandstatsOff,
		"dataValkeyInfoAll":             dataValkeyInfoAll,
		"dataDragonflyInfoAll":          dataDragonflyInfoAll,
		"dataKeydbInfoAll":              dataKeydbInfoAll,
		"dataKvrocksInfoAll":            dataKvrocksInfoAll,
	} {
		require.NotNil(t, data, name)
	}
}

// garnetBaseMetrics is the complete collected map for the captured Garnet
// fixture (INFO all + INFO keyspace), i.e. with commandstats reporting no data.
var garnetBaseMetrics = map[string]int64{
	"CurrentVersion":                              1,
	"IndexBucketCount":                            2097152,
	"IndexBucketSizeBytes":                        64,
	"IndexMemorySizeBytes":                        134217728,
	"IndexOverflowBucketCount":                    16,
	"IndexOverflowMemorySizeBytes":                1024,
	"IndexTotalMemorySizeBytes":                   134218752,
	"LastCheckpointedVersion":                     0,
	"Log.AllocatedPageCount":                      2,
	"Log.BeginAddress":                            64,
	"Log.CurrentHeapSizeBytes":                    0,
	"Log.CurrentMemorySizeBytes":                  33554432,
	"Log.FlushedUntilAddress":                     64,
	"Log.HeadAddress":                             64,
	"Log.MaxMemorySizeBytes":                      17179869184,
	"Log.MaxPageCount":                            1024,
	"Log.PageSizeBytes":                           16777216,
	"Log.SafeReadOnlyAddress":                     64,
	"Log.TailAddress":                             64,
	"aof_memory_size":                             -1,
	"arch_bits":                                   64,
	"available_system_memory":                     -1,
	"available_system_memory(MB)":                 -1,
	"cluster_enabled":                             0,
	"connected_clients":                           0,
	"connected_slaves":                            0,
	"db0_expires_keys":                            1,
	"db0_keys":                                    6,
	"garnet_hit_rate":                             0,
	"gc_committed_bytes":                          181575680,
	"gc_fragmented_bytes":                         29912,
	"gc_heap_bytes":                               178939640,
	"gc_managed_memory_bytes_excluding_heap":      2636040,
	"instantaneous_net_input_KBps":                0,
	"instantaneous_net_output_KBps":               0,
	"instantaneous_ops_per_sec":                   0,
	"keyspace_hit_rate":                           0,
	"keyspace_hits":                               0,
	"keyspace_misses":                             0,
	"master_replid":                               0,
	"master_replid2":                              0,
	"monitor_freq":                                0,
	"native_allocator_bytes":                      0,
	"ping_latency_avg":                            0,
	"ping_latency_count":                          5,
	"ping_latency_max":                            0,
	"ping_latency_min":                            0,
	"ping_latency_sum":                            0,
	"proc_pageable_memory_size":                   0,
	"proc_pageable_memory_size(MB)":               0,
	"proc_paged_memory_size":                      0,
	"proc_paged_memory_size(MB)":                  0,
	"proc_peak_paged_memory_size":                 0,
	"proc_peak_paged_memory_size(MB)":             0,
	"proc_peak_physical_memory_size":              78344192,
	"proc_peak_physical_memory_size(MB)":          74,
	"proc_peak_virtual_memory_size":               282296131584,
	"proc_peak_virtual_memory_size(MB)":           269218,
	"proc_physical_memory_size(MB)":               74,
	"proc_private_memory_size":                    356024320,
	"proc_private_memory_size(MB)":                339,
	"proc_virtual_memory_size":                    282229350400,
	"proc_virtual_memory_size(MB)":                269154,
	"processor_count":                             8,
	"rdb_last_bgsave_status":                      0,
	"rejected_connections":                        0,
	"store_heap_memory_target_size":               17179869184,
	"store_index_size":                            134218752,
	"store_mainlog_memory_size":                   33554432,
	"store_readcache_memory_size":                 0,
	"system_page_size":                            4096,
	"total_cluster_commands_processed":            0,
	"total_commands_processed":                    0,
	"total_connections_active":                    0,
	"total_connections_disposed":                  0,
	"total_connections_received":                  0,
	"total_main_store_size":                       167773184,
	"total_net_input_bytes":                       0,
	"total_net_output_bytes":                      0,
	"total_number_resp_server_session_exceptions": 0,
	"total_output_buffer_rentals":                 0,
	"total_pending":                               0,
	"total_read_commands_processed":               0,
	"total_system_memory":                         8589934592,
	"total_system_memory(MB)":                     8192,
	"total_transaction_commands_execution_failed": 0,
	"total_transaction_commands_received":         0,
	"total_write_commands_processed":              0,
	"uptime_in_days":                              0,
	"uptime_in_seconds":                           0,
	"used_memory_rss":                             78344192,
}

// garnetCommandstatsMetrics is the cmd_* subset of the captured Garnet
// commandstats fixture (Garnet reports usec=0 for all commands).
var garnetCommandstatsMetrics = map[string]int64{
	"cmd_client|setinfo_calls":         2,
	"cmd_client|setinfo_usec":          0,
	"cmd_client|setinfo_usec_per_call": 0,
	"cmd_dbsize_calls":                 1,
	"cmd_dbsize_usec":                  0,
	"cmd_dbsize_usec_per_call":         0,
	"cmd_get_calls":                    1,
	"cmd_get_usec":                     0,
	"cmd_get_usec_per_call":            0,
	"cmd_hello_calls":                  1,
	"cmd_hello_usec":                   0,
	"cmd_hello_usec_per_call":          0,
	"cmd_hset_calls":                   1,
	"cmd_hset_usec":                    0,
	"cmd_hset_usec_per_call":           0,
	"cmd_info_calls":                   4,
	"cmd_info_usec":                    0,
	"cmd_info_usec_per_call":           0,
	"cmd_lpush_calls":                  1,
	"cmd_lpush_usec":                   0,
	"cmd_lpush_usec_per_call":          0,
	"cmd_ping_calls":                   1,
	"cmd_ping_usec":                    0,
	"cmd_ping_usec_per_call":           0,
	"cmd_set_calls":                    3,
	"cmd_set_usec":                     0,
	"cmd_set_usec_per_call":            0,
}

// garnetWantCollected returns a fresh complete expected map for the garnet
// fixture; a per-call copy is required because copyTimeRelatedMetrics mutates it.
func garnetWantCollected(withCommandstats bool) map[string]int64 {
	want := make(map[string]int64, len(garnetBaseMetrics)+len(garnetCommandstatsMetrics))
	maps.Copy(want, garnetBaseMetrics)
	if withCommandstats {
		maps.Copy(want, garnetCommandstatsMetrics)
	}
	return want
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"success on default config": {
			config: New().Config,
		},
		"fails on unset 'address'": {
			wantFail: true,
			config:   Config{Address: ""},
		},
		"fails on invalid 'address' format": {
			wantFail: true,
			config:   Config{Address: "127.0.0.1:6379"},
		},
		"fails on invalid TLSCA": {
			wantFail: true,
			config: Config{
				Address:   "redis://127.0.0.1:6379",
				TLSConfig: tlscfg.TLSConfig{TLSCA: "testdata/tls"},
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
		prepare  func(t *testing.T) *Collector
		wantFail bool
	}{
		"success on valid response v6.0.9": {
			prepare: prepareRedisV609,
		},
		"success on valid response garnet": {
			prepare: prepareGarnetWithCommandstats,
		},
		"success on garnet with commandstats disabled": {
			prepare: prepareGarnetCommandstatsDisabled,
		},
		"success on valid response valkey 9.1.2": {
			prepare: prepareValkey,
		},
		"success on valid response dragonfly df-v2.0.0": {
			prepare: prepareDragonfly,
		},
		"success on valid response keydb 6.3.4": {
			prepare: prepareKeydb,
		},
		"success on valid response kvrocks 2.17.0": {
			prepare: prepareKvrocks,
		},
		"fails on error on Info": {
			wantFail: true,
			prepare:  prepareRedisErrorOnInfo,
		},
		"fails on response from not Redis instance": {
			wantFail: true,
			prepare:  prepareRedisWithPikaMetrics,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare(t)

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	collr := New()
	require.NoError(t, collr.Init(context.Background()))

	assert.NotNil(t, collr.Charts())
}

func TestCollector_Cleanup(t *testing.T) {
	collr := New()
	assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })

	require.NoError(t, collr.Init(context.Background()))
	m := &mockRedisClient{}
	collr.rdb = m

	collr.Cleanup(context.Background())

	assert.True(t, m.calledClose)
}

// garnetMissingDims holds chart dimension IDs whose backing INFO fields Garnet
// does not emit (no equivalent data): CPU counters, redis allocator memory
// fields, client timeout/tracking, expiration/eviction and persistence fields.
// Their chart dimensions stay empty for Garnet.
var garnetMissingDims = map[string]bool{
	"blocked_clients":             true,
	"clients_in_timeout_table":    true,
	"evicted_keys":                true,
	"expired_keys":                true,
	"maxmemory":                   true,
	"mem_fragmentation_ratio":     true,
	"rdb_changes_since_last_save": true,
	"rdb_current_bgsave_time_sec": true,
	"rdb_last_save_time":          true,
	"tracking_clients":            true,
	"used_memory":                 true,
	"used_memory_dataset":         true,
	"used_memory_lua":             true,
	"used_memory_peak":            true,
	"used_memory_scripts":         true,
	"used_cpu_sys":                true,
	"used_cpu_sys_children":       true,
	"used_cpu_user":               true,
	"used_cpu_user_children":      true,
}

func skipGarnetMissingDims(_ *collectorapi.Chart, dim *collectorapi.Dim) bool {
	return garnetMissingDims[dim.ID]
}

// dragonflyMissingDims holds chart dimension IDs whose backing INFO fields
// Dragonfly does not emit; their chart dimensions stay empty for Dragonfly.
var dragonflyMissingDims = map[string]bool{
	"clients_in_timeout_table":    true,
	"mem_fragmentation_ratio":     true,
	"rdb_changes_since_last_save": true,
	"rdb_current_bgsave_time_sec": true,
	"rdb_last_save_time":          true,
	"tracking_clients":            true,
	"used_memory_dataset":         true,
	"used_memory_scripts":         true,
}

func skipDragonflyMissingDims(_ *collectorapi.Chart, dim *collectorapi.Dim) bool {
	return dragonflyMissingDims[dim.ID]
}

// kvrocksMissingDims holds chart dimension IDs whose backing INFO fields
// Kvrocks does not emit; their chart dimensions stay empty for Kvrocks.
var kvrocksMissingDims = map[string]bool{
	"clients_in_timeout_table":    true,
	"evicted_keys":                true,
	"expired_keys":                true,
	"maxmemory":                   true,
	"mem_fragmentation_ratio":     true,
	"rdb_changes_since_last_save": true,
	"rdb_current_bgsave_time_sec": true,
	"rdb_last_bgsave_status":      true,
	"rdb_last_save_time":          true,
	"rejected_connections":        true,
	"tracking_clients":            true,
	"used_memory":                 true,
	"used_memory_dataset":         true,
	"used_memory_peak":            true,
	"used_memory_scripts":         true,
}

func skipKvrocksMissingDims(_ *collectorapi.Chart, dim *collectorapi.Dim) bool {
	return kvrocksMissingDims[dim.ID]
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func(t *testing.T) *Collector
		wantCollected map[string]int64
		dimsSkip      func(chart *collectorapi.Chart, dim *collectorapi.Dim) bool
	}{
		"success on valid response v6.0.9": {
			prepare: prepareRedisV609,
			wantCollected: map[string]int64{
				"active_defrag_hits":              0,
				"active_defrag_key_hits":          0,
				"active_defrag_key_misses":        0,
				"active_defrag_misses":            0,
				"active_defrag_running":           0,
				"allocator_active":                1208320,
				"allocator_allocated":             903408,
				"allocator_frag_bytes":            304912,
				"allocator_frag_ratio":            1340,
				"allocator_resident":              3723264,
				"allocator_rss_bytes":             2514944,
				"allocator_rss_ratio":             3080,
				"aof_base_size":                   116,
				"aof_buffer_length":               0,
				"aof_current_rewrite_time_sec":    -1,
				"aof_current_size":                294,
				"aof_delayed_fsync":               0,
				"aof_enabled":                     0,
				"aof_last_cow_size":               0,
				"aof_last_rewrite_time_sec":       -1,
				"aof_pending_bio_fsync":           0,
				"aof_pending_rewrite":             0,
				"aof_rewrite_buffer_length":       0,
				"aof_rewrite_in_progress":         0,
				"aof_rewrite_scheduled":           0,
				"arch_bits":                       64,
				"blocked_clients":                 0,
				"client_recent_max_input_buffer":  8,
				"client_recent_max_output_buffer": 0,
				"clients_in_timeout_table":        0,
				"cluster_enabled":                 0,
				"cmd_command_calls":               2,
				"cmd_command_usec":                2182,
				"cmd_command_usec_per_call":       1091000,
				"cmd_get_calls":                   2,
				"cmd_get_usec":                    29,
				"cmd_get_usec_per_call":           14500,
				"cmd_hello_calls":                 1,
				"cmd_hello_usec":                  15,
				"cmd_hello_usec_per_call":         15000,
				"cmd_hmset_calls":                 2,
				"cmd_hmset_usec":                  408,
				"cmd_hmset_usec_per_call":         204000,
				"cmd_info_calls":                  132,
				"cmd_info_usec":                   37296,
				"cmd_info_usec_per_call":          282550,
				"cmd_ping_calls":                  19,
				"cmd_ping_usec":                   286,
				"cmd_ping_usec_per_call":          15050,
				"cmd_set_calls":                   3,
				"cmd_set_usec":                    140,
				"cmd_set_usec_per_call":           46670,
				"configured_hz":                   10,
				"connected_clients":               1,
				"connected_slaves":                0,
				"db0_expires_keys":                0,
				"db0_keys":                        4,
				"evicted_keys":                    0,
				"expire_cycle_cpu_milliseconds":   28362,
				"expired_keys":                    0,
				"expired_stale_perc":              0,
				"expired_time_cap_reached_count":  0,
				"hz":                              10,
				"instantaneous_input_kbps":        0,
				"instantaneous_ops_per_sec":       0,
				"instantaneous_output_kbps":       0,
				"io_threaded_reads_processed":     0,
				"io_threaded_writes_processed":    0,
				"io_threads_active":               0,
				"keyspace_hit_rate":               100000,
				"keyspace_hits":                   2,
				"keyspace_misses":                 0,
				"latest_fork_usec":                810,
				"lazyfree_pending_objects":        0,
				"loading":                         0,
				"lru_clock":                       13181377,
				"master_repl_offset":              0,
				"master_replid2":                  0,
				"maxmemory":                       0,
				"mem_aof_buffer":                  0,
				"mem_clients_normal":              0,
				"mem_clients_slaves":              0,
				"mem_fragmentation_bytes":         3185848,
				"mem_fragmentation_ratio":         4960,
				"mem_not_counted_for_evict":       0,
				"mem_replication_backlog":         0,
				"migrate_cached_sockets":          0,
				"module_fork_in_progress":         0,
				"module_fork_last_cow_size":       0,
				"number_of_cached_scripts":        0,
				"ping_latency_avg":                0,
				"ping_latency_count":              5,
				"ping_latency_max":                0,
				"ping_latency_min":                0,
				"ping_latency_sum":                0,
				"process_id":                      1,
				"pubsub_channels":                 0,
				"pubsub_patterns":                 0,
				"rdb_bgsave_in_progress":          0,
				"rdb_changes_since_last_save":     0,
				"rdb_current_bgsave_time_sec":     0,
				"rdb_last_bgsave_status":          0,
				"rdb_last_bgsave_time_sec":        0,
				"rdb_last_cow_size":               290816,
				"rdb_last_save_time":              125697993,
				"redis_git_dirty":                 0,
				"redis_git_sha1":                  0,
				"rejected_connections":            0,
				"repl_backlog_active":             0,
				"repl_backlog_first_byte_offset":  0,
				"repl_backlog_histlen":            0,
				"repl_backlog_size":               1048576,
				"rss_overhead_bytes":              266240,
				"rss_overhead_ratio":              1070,
				"second_repl_offset":              -1,
				"slave_expires_tracked_keys":      0,
				"sync_full":                       0,
				"sync_partial_err":                0,
				"sync_partial_ok":                 0,
				"tcp_port":                        6379,
				"total_commands_processed":        161,
				"total_connections_received":      87,
				"total_net_input_bytes":           2301,
				"total_net_output_bytes":          507187,
				"total_reads_processed":           250,
				"total_system_memory":             2084032512,
				"total_writes_processed":          163,
				"tracking_clients":                0,
				"tracking_total_items":            0,
				"tracking_total_keys":             0,
				"tracking_total_prefixes":         0,
				"unexpected_error_replies":        0,
				"uptime_in_days":                  2,
				"uptime_in_seconds":               252812,
				"used_cpu_sys":                    630829,
				"used_cpu_sys_children":           20,
				"used_cpu_user":                   188394,
				"used_cpu_user_children":          2,
				"used_memory":                     867160,
				"used_memory_dataset":             63816,
				"used_memory_lua":                 37888,
				"used_memory_overhead":            803344,
				"used_memory_peak":                923360,
				"used_memory_rss":                 3989504,
				"used_memory_scripts":             0,
				"used_memory_startup":             803152,
			},
		},
		"success on valid response garnet (commandstats enabled)": {
			prepare:       prepareGarnetWithCommandstats,
			wantCollected: garnetWantCollected(true),
			dimsSkip:      skipGarnetMissingDims,
		},
		"success on valid response garnet (commandstats disabled)": {
			prepare:       prepareGarnetCommandstatsDisabled,
			wantCollected: garnetWantCollected(false),
			dimsSkip:      skipGarnetMissingDims,
		},
		"success on valid response valkey 9.1.2": {
			prepare: prepareValkey,
			wantCollected: map[string]int64{
				"acl_access_denied_auth":                      0,
				"acl_access_denied_channel":                   0,
				"acl_access_denied_cmd":                       0,
				"acl_access_denied_db":                        0,
				"acl_access_denied_key":                       0,
				"acl_access_denied_tls_cert":                  0,
				"active_defrag_hits":                          0,
				"active_defrag_key_hits":                      0,
				"active_defrag_key_misses":                    0,
				"active_defrag_misses":                        0,
				"active_defrag_running":                       0,
				"allocator_active":                            1413120,
				"allocator_allocated":                         1293088,
				"allocator_frag_bytes":                        120032,
				"allocator_frag_ratio":                        1090,
				"allocator_muzzy":                             0,
				"allocator_resident":                          7446528,
				"allocator_rss_bytes":                         6033408,
				"allocator_rss_ratio":                         5270,
				"aof_current_rewrite_time_sec":                -1,
				"aof_enabled":                                 0,
				"aof_last_cow_size":                           0,
				"aof_last_rewrite_time_sec":                   -1,
				"aof_rewrite_in_progress":                     0,
				"aof_rewrite_scheduled":                       0,
				"aof_rewrites":                                0,
				"aof_rewrites_consecutive_failures":           0,
				"arch_bits":                                   64,
				"async_loading":                               0,
				"blocked_clients":                             0,
				"client_output_buffer_limit_disconnections":   0,
				"client_query_buffer_limit_disconnections":    0,
				"client_recent_max_input_buffer":              0,
				"client_recent_max_output_buffer":             0,
				"clients_hz":                                  10,
				"clients_in_timeout_table":                    0,
				"cluster_connections":                         0,
				"cluster_enabled":                             0,
				"cmd_config|get_calls":                        2,
				"cmd_config|get_usec":                         8,
				"cmd_config|get_usec_per_call":                4000,
				"configured_hz":                               10,
				"connected_clients":                           1,
				"connected_slaves":                            0,
				"current_active_defrag_time":                  0,
				"current_cow_peak":                            0,
				"current_cow_size":                            0,
				"current_cow_size_age":                        0,
				"current_eviction_exceeded_time":              0,
				"current_fork_perc":                           0,
				"current_save_keys_processed":                 0,
				"current_save_keys_total":                     0,
				"dump_payload_sanitizations":                  0,
				"engines_count":                               1,
				"engines_total_memory_overhead":               56,
				"engines_total_used_memory":                   69632,
				"eventloop_cycles":                            12,
				"eventloop_duration_cmd_sum":                  0,
				"eventloop_duration_sum":                      930,
				"evicted_clients":                             0,
				"evicted_keys":                                0,
				"evicted_scripts":                             0,
				"expire_cycle_cpu_milliseconds":               0,
				"expired_fields":                              0,
				"expired_keys":                                0,
				"expired_keys_with_volatile_items_stale_perc": 0,
				"expired_stale_perc":                          0,
				"expired_time_cap_reached_count":              0,
				"hz":                                          10,
				"instantaneous_eventloop_cycles_per_sec":      3,
				"instantaneous_eventloop_duration_usec":       34,
				"instantaneous_input_kbps":                    0,
				"instantaneous_input_repl_kbps":               0,
				"instantaneous_ops_per_sec":                   0,
				"instantaneous_output_kbps":                   0,
				"instantaneous_output_repl_kbps":              0,
				"io_threaded_accept_processed":                0,
				"io_threaded_freed_objects":                   0,
				"io_threaded_poll_processed":                  0,
				"io_threaded_reads_processed":                 0,
				"io_threaded_total_prefetch_batches":          0,
				"io_threaded_total_prefetch_entries":          0,
				"io_threaded_writes_processed":                0,
				"io_threads_active":                           0,
				"keyspace_hit_rate":                           0,
				"keyspace_hits":                               0,
				"keyspace_misses":                             0,
				"latest_fork_usec":                            0,
				"lazyfree_pending_objects":                    0,
				"lazyfreed_objects":                           0,
				"loading":                                     0,
				"lru_clock":                                   12650136,
				"master_repl_offset":                          0,
				"maxclients":                                  10000,
				"maxmemory":                                   0,
				"mem_aof_buffer":                              0,
				"mem_clients_normal":                          0,
				"mem_clients_slaves":                          0,
				"mem_cluster_links":                           0,
				"mem_cluster_slot_export":                     0,
				"mem_cluster_slot_import":                     0,
				"mem_fragmentation_bytes":                     18241736,
				"mem_fragmentation_ratio":                     20670,
				"mem_not_counted_for_evict":                   0,
				"mem_overhead_db_hashtable_rehashing":         0,
				"mem_replicas_repl_buffer":                    0,
				"mem_replication_backlog":                     0,
				"mem_total_replication_buffers":               0,
				"migrate_cached_sockets":                      0,
				"module_fork_in_progress":                     0,
				"module_fork_last_cow_size":                   0,
				"number_of_cached_scripts":                    0,
				"number_of_functions":                         0,
				"number_of_libraries":                         0,
				"paused_timeout_milliseconds":                 0,
				"ping_latency_avg":                            0,
				"ping_latency_count":                          5,
				"ping_latency_max":                            0,
				"ping_latency_min":                            0,
				"ping_latency_sum":                            0,
				"process_id":                                  1,
				"pubsub_channels":                             0,
				"pubsub_clients":                              0,
				"pubsub_patterns":                             0,
				"pubsubshard_channels":                        0,
				"rdb_bgsave_in_progress":                      0,
				"rdb_changes_since_last_save":                 0,
				"rdb_current_bgsave_time_sec":                 0,
				"rdb_last_bgsave_status":                      0,
				"rdb_last_bgsave_time_sec":                    -1,
				"rdb_last_cow_size":                           0,
				"rdb_last_load_keys_expired":                  0,
				"rdb_last_load_keys_loaded":                   0,
				"rdb_last_save_time":                          70,
				"rdb_saves":                                   0,
				"redis_git_dirty":                             0,
				"redis_git_sha1":                              0,
				"rejected_connections":                        0,
				"repl_backlog_active":                         0,
				"repl_backlog_first_byte_offset":              0,
				"repl_backlog_histlen":                        0,
				"repl_backlog_size":                           10485760,
				"replicas_waiting_psync":                      0,
				"reply_buffer_expands":                        0,
				"reply_buffer_shrinks":                        0,
				"rss_overhead_bytes":                          11722752,
				"rss_overhead_ratio":                          2570,
				"second_repl_offset":                          -1,
				"server_time_usec":                            1791035032007398,
				"slave_expires_tracked_keys":                  0,
				"slot_migration_fork_in_progress":             0,
				"sync_full":                                   0,
				"sync_partial_err":                            0,
				"sync_partial_ok":                             0,
				"tcp_port":                                    6379,
				"tls_ca_cert_expires_in_seconds":              0,
				"tls_client_cert_expires_in_seconds":          0,
				"tls_server_cert_expires_in_seconds":          0,
				"total_active_defrag_time":                    0,
				"total_blocking_keys":                         0,
				"total_blocking_keys_on_nokey":                0,
				"total_commands_processed":                    2,
				"total_connections_received":                  2,
				"total_error_replies":                         0,
				"total_eviction_exceeded_time":                0,
				"total_forks":                                 0,
				"total_net_cluster_slot_export_bytes":         0,
				"total_net_cluster_slot_import_bytes":         0,
				"total_net_input_bytes":                       24,
				"total_net_output_bytes":                      0,
				"total_net_repl_input_bytes":                  0,
				"total_net_repl_output_bytes":                 0,
				"total_reads_processed":                       3,
				"total_system_memory":                         8589934592,
				"total_watched_keys":                          0,
				"total_writes_processed":                      0,
				"tracking_clients":                            0,
				"tracking_total_items":                        0,
				"tracking_total_keys":                         0,
				"tracking_total_prefixes":                     0,
				"unexpected_error_replies":                    0,
				"uptime_in_days":                              0,
				"uptime_in_seconds":                           1,
				"used_active_time_main_thread":                0,
				"used_cpu_sys":                                4,
				"used_cpu_sys_children":                       1,
				"used_cpu_sys_main_thread":                    5,
				"used_cpu_user":                               4,
				"used_cpu_user_children":                      1,
				"used_cpu_user_main_thread":                   4,
				"used_memory":                                 946920,
				"used_memory_dataset":                         19240,
				"used_memory_functions":                       328,
				"used_memory_lua":                             33792,
				"used_memory_overhead":                        927680,
				"used_memory_peak":                            946920,
				"used_memory_rss":                             19169280,
				"used_memory_scripts":                         328,
				"used_memory_scripts_eval":                    0,
				"used_memory_startup":                         927128,
				"used_memory_vm_eval":                         33792,
				"used_memory_vm_functions":                    35840,
				"used_memory_vm_total":                        69632,
				"watching_clients":                            0,
			},
		},
		"success on valid response dragonfly df-v2.0.0": {
			prepare:  prepareDragonfly,
			dimsSkip: skipDragonflyMissingDims,
			wantCollected: map[string]int64{
				"acl_key_globs_bytes":                 0,
				"acl_num_cat_changes":                 1,
				"acl_num_cmd_changes":                 0,
				"acl_num_key_globs":                   0,
				"acl_num_passwords":                   0,
				"acl_num_pubsub_globs":                0,
				"acl_num_users":                       1,
				"acl_pubsub_globs_bytes":              0,
				"acl_total_bytes":                     428,
				"arch_bits":                           64,
				"async_delete_task_invocation_total":  0,
				"batch_read_commands_bytes":           0,
				"batch_read_commands_total":           0,
				"batch_write_commands_bytes":          0,
				"batch_write_commands_total":          0,
				"big_value_preemptions":               0,
				"blocked_clients":                     0,
				"blocked_on_interpreter":              0,
				"borrowed_strings_sent_total":         0,
				"bump_ups":                            0,
				"client_read_buffer_bytes":            256,
				"client_read_buffer_peak_bytes":       256,
				"cluster_enabled":                     0,
				"commands_squashing_replies_bytes":    0,
				"compressed_blobs":                    0,
				"connected_clients":                   1,
				"connected_slaves":                    0,
				"connection_migrations":               0,
				"connection_recv_provided_calls":      0,
				"current_save_duration_sec":           0,
				"current_save_keys_processed":         0,
				"current_save_keys_total":             0,
				"current_snapshot_perc":               0,
				"db0_expires_keys":                    0,
				"db0_keys":                            0,
				"defrag_attempt_total":                0,
				"defrag_realloc_total":                0,
				"defrag_task_invocation_total":        0,
				"delete_ttl_sec":                      0,
				"dispatch_queue_bytes":                0,
				"dispatch_queue_peak_bytes":           0,
				"dispatch_queue_subscriber_bytes":     0,
				"eval_io_coordination_total":          0,
				"eval_shardlocal_coordination_total":  0,
				"eval_squashed_flushes":               0,
				"evicted_keys":                        0,
				"expired_keys":                        0,
				"fibers_count":                        147,
				"fibers_stack_vms":                    9609200,
				"garbage_checked":                     0,
				"garbage_collected":                   0,
				"hard_evictions":                      0,
				"huffenc_attempt_total":               0,
				"huffenc_success_total":               0,
				"hz":                                  100,
				"inline_keys":                         0,
				"instantaneous_input_kbps":            -1,
				"instantaneous_ops_per_sec":           0,
				"instantaneous_output_kbps":           -1,
				"keyspace_hit_rate":                   0,
				"keyspace_hits":                       0,
				"keyspace_misses":                     0,
				"keyspace_mutations":                  0,
				"last_failed_save":                    0,
				"last_failed_save_duration_sec":       0,
				"last_success_save":                   1791035031,
				"last_success_save_duration_sec":      0,
				"loading":                             0,
				"lua_blocked_total":                   0,
				"lua_force_gc_calls":                  0,
				"lua_gc_duration_total_sec":           0,
				"lua_gc_freed_memory_total":           0,
				"lua_interpreter_cnt":                 0,
				"lua_interpreter_return":              0,
				"max_clients":                         64000,
				"maxmemory":                           48540578611,
				"migration_errors_total":              0,
				"num_entries":                         0,
				"object_used_memory":                  0,
				"oom_rejections":                      0,
				"ping_latency_avg":                    0,
				"ping_latency_count":                  5,
				"ping_latency_max":                    0,
				"ping_latency_min":                    0,
				"ping_latency_sum":                    0,
				"pipeline_cache_bytes":                0,
				"pipeline_queue_bytes":                0,
				"pipeline_queue_length":               0,
				"pipeline_throttle_total":             0,
				"pipelined_latency_usec":              0,
				"prime_capacity":                      161280,
				"process_id":                          1,
				"psync_buffer_bytes":                  0,
				"psync_buffer_size":                   0,
				"rdb_bgsave_in_progress":              0,
				"rdb_changes_since_last_success_save": 0,
				"rdb_last_bgsave_status":              0,
				"rdb_save_count":                      0,
				"rdb_save_usec":                       0,
				"rejected_connections":                -1,
				"replication_full_sync_buffer_bytes":  0,
				"replication_streaming_buffer_bytes":  0,
				"rw_throttle_batches_total":           0,
				"saving":                              0,
				"search_memory":                       0,
				"search_num_entries":                  0,
				"search_num_indices":                  0,
				"send_delay_ms":                       0,
				"small_string_bytes":                  0,
				"snapshot_serialization_bytes":        0,
				"stash_unloaded":                      0,
				"table_used_memory":                   6203904,
				"tcp_port":                            6379,
				"thread_count":                        8,
				"tiered_allocated_bytes":              0,
				"tiered_capacity_bytes":               0,
				"tiered_clients_throttled":            0,
				"tiered_cold_storage_bytes":           0,
				"tiered_defrag_usec":                  0,
				"tiered_entries":                      0,
				"tiered_entries_bytes":                0,
				"tiered_heap_buf_allocations":         0,
				"tiered_offloading_stashes":           0,
				"tiered_offloading_usec":              0,
				"tiered_pending_read_cnt":             0,
				"tiered_pending_stash_bytes":          0,
				"tiered_pending_stash_cnt":            0,
				"tiered_ram_cool_hits":                0,
				"tiered_ram_hits":                     0,
				"tiered_ram_misses":                   0,
				"tiered_registered_buf_allocations":   0,
				"tiered_small_bins_cnt":               0,
				"tiered_small_bins_entries_bytes":     0,
				"tiered_small_bins_entries_cnt":       0,
				"tiered_small_bins_filling_bytes":     0,
				"tiered_total_cancels":                0,
				"tiered_total_clients_throttled":      0,
				"tiered_total_defrags":                0,
				"tiered_total_deletes":                0,
				"tiered_total_fetches":                0,
				"tiered_total_stash_overflows":        0,
				"tiered_total_stashes":                0,
				"tiered_total_uploads":                0,
				"timeout_disconnects":                 0,
				"tls_bytes":                           244624,
				"total_commands_processed":            1,
				"total_connections_received":          2,
				"total_handshakes_completed":          0,
				"total_handshakes_started":            0,
				"total_heartbeat_expired_bytes":       0,
				"total_heartbeat_expired_calls":       0,
				"total_heartbeat_expired_keys":        0,
				"total_journal_omits":                 0,
				"total_migrated_keys":                 0,
				"total_net_input_bytes":               0,
				"total_net_output_bytes":              0,
				"total_pipelined_commands":            0,
				"total_reads_processed":               0,
				"total_writes_processed":              0,
				"traverse_ttl_sec":                    0,
				"tx_global_total":                     0,
				"tx_inline_runs_total":                0,
				"tx_normal_total":                     0,
				"tx_queue_len":                        0,
				"tx_schedule_cancel_total":            0,
				"tx_shard_ooo_total":                  0,
				"tx_shard_optimistic_total":           0,
				"tx_shard_polls":                      0,
				"uptime_in_days":                      0,
				"uptime_in_seconds":                   1,
				"used_cpu_sys":                        625,
				"used_cpu_sys_children":               778,
				"used_cpu_sys_main_thread":            190,
				"used_cpu_user":                       167,
				"used_cpu_user_children":              158,
				"used_cpu_user_main_thread":           0,
				"used_memory":                         6784704,
				"used_memory_lua":                     0,
				"used_memory_peak":                    6784704,
				"used_memory_peak_rss":                59592704,
				"used_memory_rss":                     60502016,
			},
		},
		"success on valid response keydb 6.3.4": {
			prepare: prepareKeydb,
			wantCollected: map[string]int64{
				"active_defrag_hits":              0,
				"active_defrag_key_hits":          0,
				"active_defrag_key_misses":        0,
				"active_defrag_misses":            0,
				"active_defrag_running":           0,
				"allocator_active":                3670016,
				"allocator_allocated":             3116992,
				"allocator_frag_bytes":            553024,
				"allocator_frag_ratio":            1180,
				"allocator_resident":              9699328,
				"allocator_rss_bytes":             6029312,
				"allocator_rss_ratio":             2640,
				"aof_current_rewrite_time_sec":    -1,
				"aof_enabled":                     0,
				"aof_last_cow_size":               0,
				"aof_last_rewrite_time_sec":       -1,
				"aof_rewrite_in_progress":         0,
				"aof_rewrite_scheduled":           0,
				"arch_bits":                       64,
				"avg_lock_contention":             0,
				"blocked_clients":                 0,
				"client_recent_max_input_buffer":  8,
				"client_recent_max_output_buffer": 0,
				"clients_in_timeout_table":        0,
				"cluster_connections":             0,
				"cluster_enabled":                 0,
				"configured_hz":                   10,
				"connected_clients":               2,
				"connected_slaves":                0,
				"current_client_thread":           0,
				"current_cow_size":                0,
				"current_cow_size_age":            0,
				"current_fork_perc":               0,
				"current_save_keys_processed":     0,
				"current_save_keys_total":         0,
				"dump_payload_sanitizations":      0,
				"evicted_keys":                    0,
				"expire_cycle_cpu_milliseconds":   0,
				"expired_keys":                    0,
				"expired_stale_perc":              0,
				"expired_time_cap_reached_count":  0,
				"hz":                              10,
				"instantaneous_input_kbps":        0,
				"instantaneous_lock_contention":   1,
				"instantaneous_ops_per_sec":       0,
				"instantaneous_output_kbps":       0,
				"keyspace_hit_rate":               0,
				"keyspace_hits":                   0,
				"keyspace_misses":                 0,
				"latest_fork_usec":                0,
				"lazyfree_pending_objects":        0,
				"lazyfreed_objects":               0,
				"loading":                         0,
				"long_lock_waits":                 0,
				"lru_clock":                       12650136,
				"master_repl_offset":              0,
				"maxclients":                      10000,
				"maxmemory":                       0,
				"mem_aof_buffer":                  0,
				"mem_clients_normal":              40976,
				"mem_clients_slaves":              0,
				"mem_fragmentation_bytes":         17774288,
				"mem_fragmentation_ratio":         9030,
				"mem_not_counted_for_evict":       0,
				"mem_replication_backlog":         0,
				"migrate_cached_sockets":          0,
				"module_fork_in_progress":         0,
				"module_fork_last_cow_size":       0,
				"mvcc_depth":                      0,
				"number_of_cached_scripts":        0,
				"ping_latency_avg":                0,
				"ping_latency_count":              5,
				"ping_latency_max":                0,
				"ping_latency_min":                0,
				"ping_latency_sum":                0,
				"process_id":                      1,
				"pubsub_channels":                 0,
				"pubsub_patterns":                 0,
				"rdb_bgsave_in_progress":          0,
				"rdb_changes_since_last_save":     0,
				"rdb_current_bgsave_time_sec":     0,
				"rdb_last_bgsave_status":          0,
				"rdb_last_bgsave_time_sec":        -1,
				"rdb_last_cow_size":               0,
				"rdb_last_save_time":              70,
				"redis_git_dirty":                 1,
				"rejected_connections":            0,
				"repl_backlog_active":             0,
				"repl_backlog_first_byte_offset":  0,
				"repl_backlog_histlen":            0,
				"repl_backlog_size":               1048576,
				"rss_overhead_bytes":              10289152,
				"rss_overhead_ratio":              2060,
				"second_repl_offset":              -1,
				"server_threads":                  2,
				"server_time_usec":                1791035032201296,
				"slave_expires_tracked_keys":      0,
				"storage_provider_read_hits":      0,
				"storage_provider_read_misses":    0,
				"sync_full":                       0,
				"sync_partial_err":                0,
				"sync_partial_ok":                 0,
				"tcp_port":                        6379,
				"thread_0_clients":                2,
				"thread_1_clients":                0,
				"total_commands_processed":        0,
				"total_connections_received":      2,
				"total_error_replies":             0,
				"total_forks":                     0,
				"total_net_input_bytes":           24,
				"total_net_output_bytes":          0,
				"total_reads_processed":           2,
				"total_system_memory":             8589934592,
				"total_writes_processed":          0,
				"tracking_clients":                0,
				"tracking_total_items":            0,
				"tracking_total_keys":             0,
				"tracking_total_prefixes":         0,
				"unexpected_error_replies":        0,
				"uptime_in_days":                  0,
				"uptime_in_seconds":               1,
				"used_cpu_sys":                    27,
				"used_cpu_sys_children":           1,
				"used_cpu_sys_main_thread":        0,
				"used_cpu_user":                   18,
				"used_cpu_user_children":          1,
				"used_cpu_user_main_thread":       0,
				"used_memory":                     2297992,
				"used_memory_dataset":             86144,
				"used_memory_lua":                 37888,
				"used_memory_overhead":            2211848,
				"used_memory_peak":                2297992,
				"used_memory_rss":                 19988480,
				"used_memory_scripts":             0,
				"used_memory_startup":             2170872,
			},
		},
		"success on valid response kvrocks 2.17.0": {
			prepare:  prepareKvrocks,
			dimsSkip: skipKvrocksMissingDims,
			wantCollected: map[string]int64{
				"all_mem_tables":                                                 16384,
				"arch_bits":                                                      64,
				"bgsave_in_progress":                                             0,
				"block_cache_data_hit":                                           0,
				"block_cache_data_miss":                                          0,
				"block_cache_filter_hit":                                         0,
				"block_cache_filter_miss":                                        0,
				"block_cache_hit":                                                0,
				"block_cache_index_hit":                                          0,
				"block_cache_index_miss":                                         0,
				"block_cache_miss":                                               0,
				"block_cache_pinned_usage[default]":                              64,
				"block_cache_usage":                                              262144,
				"blocked_clients":                                                0,
				"client_output_buffer_limit_disconnections":                      0,
				"cluster_enabled":                                                0,
				"cmd_info_calls":                                                 1,
				"cmd_info_usec":                                                  0,
				"cmd_info_usec_per_call":                                         0,
				"compaction_count":                                               0,
				"compaction_pending":                                             0,
				"connected_clients":                                              1,
				"connected_slaves":                                               0,
				"cur_mem_tables":                                                 16384,
				"db0_expires_keys":                                               0,
				"db0_keys":                                                       0,
				"estimate_keys[default]":                                         0,
				"estimate_keys[index]":                                           0,
				"estimate_keys[metadata]":                                        0,
				"estimate_keys[propagate]":                                       0,
				"estimate_keys[pubsub]":                                          0,
				"estimate_keys[search]":                                          0,
				"estimate_keys[stream]":                                          0,
				"estimate_keys[zset_score]":                                      0,
				"estimate_pending_compaction_bytes[default]":                     0,
				"estimate_pending_compaction_bytes[index]":                       0,
				"estimate_pending_compaction_bytes[metadata]":                    0,
				"estimate_pending_compaction_bytes[propagate]":                   0,
				"estimate_pending_compaction_bytes[pubsub]":                      0,
				"estimate_pending_compaction_bytes[search]":                      0,
				"estimate_pending_compaction_bytes[stream]":                      0,
				"estimate_pending_compaction_bytes[zset_score]":                  0,
				"flush_count":                                                    0,
				"get_per_sec":                                                    0,
				"index_and_filter_cache_usage[default]":                          0,
				"index_and_filter_cache_usage[index]":                            0,
				"index_and_filter_cache_usage[metadata]":                         0,
				"index_and_filter_cache_usage[propagate]":                        0,
				"index_and_filter_cache_usage[pubsub]":                           0,
				"index_and_filter_cache_usage[search]":                           0,
				"index_and_filter_cache_usage[stream]":                           0,
				"index_and_filter_cache_usage[zset_score]":                       0,
				"instantaneous_input_kbps":                                       0,
				"instantaneous_ops_per_sec":                                      0,
				"instantaneous_output_kbps":                                      0,
				"keyspace_hit_rate":                                              0,
				"keyspace_hits":                                                  0,
				"keyspace_misses":                                                0,
				"last_bgsave_time":                                               1791035032,
				"last_bgsave_time_sec":                                           -1,
				"level0_file_limit_slowdown[default]":                            0,
				"level0_file_limit_slowdown[index]":                              0,
				"level0_file_limit_slowdown[metadata]":                           0,
				"level0_file_limit_slowdown[propagate]":                          0,
				"level0_file_limit_slowdown[pubsub]":                             0,
				"level0_file_limit_slowdown[search]":                             0,
				"level0_file_limit_slowdown[stream]":                             0,
				"level0_file_limit_slowdown[zset_score]":                         0,
				"level0_file_limit_slowdown_with_ongoing_compaction[default]":    0,
				"level0_file_limit_slowdown_with_ongoing_compaction[index]":      0,
				"level0_file_limit_slowdown_with_ongoing_compaction[metadata]":   0,
				"level0_file_limit_slowdown_with_ongoing_compaction[propagate]":  0,
				"level0_file_limit_slowdown_with_ongoing_compaction[pubsub]":     0,
				"level0_file_limit_slowdown_with_ongoing_compaction[search]":     0,
				"level0_file_limit_slowdown_with_ongoing_compaction[stream]":     0,
				"level0_file_limit_slowdown_with_ongoing_compaction[zset_score]": 0,
				"level0_file_limit_stop[default]":                                0,
				"level0_file_limit_stop[index]":                                  0,
				"level0_file_limit_stop[metadata]":                               0,
				"level0_file_limit_stop[propagate]":                              0,
				"level0_file_limit_stop[pubsub]":                                 0,
				"level0_file_limit_stop[search]":                                 0,
				"level0_file_limit_stop[stream]":                                 0,
				"level0_file_limit_stop[zset_score]":                             0,
				"level0_file_limit_stop_with_ongoing_compaction[default]":        0,
				"level0_file_limit_stop_with_ongoing_compaction[index]":          0,
				"level0_file_limit_stop_with_ongoing_compaction[metadata]":       0,
				"level0_file_limit_stop_with_ongoing_compaction[propagate]":      0,
				"level0_file_limit_stop_with_ongoing_compaction[pubsub]":         0,
				"level0_file_limit_stop_with_ongoing_compaction[search]":         0,
				"level0_file_limit_stop_with_ongoing_compaction[stream]":         0,
				"level0_file_limit_stop_with_ongoing_compaction[zset_score]":     0,
				"loading":                                       0,
				"master_repl_offset":                            0,
				"maxclients":                                    10000,
				"memtable_count_limit_slowdown[default]":        0,
				"memtable_count_limit_slowdown[index]":          0,
				"memtable_count_limit_slowdown[metadata]":       0,
				"memtable_count_limit_slowdown[propagate]":      0,
				"memtable_count_limit_slowdown[pubsub]":         0,
				"memtable_count_limit_slowdown[search]":         0,
				"memtable_count_limit_slowdown[stream]":         0,
				"memtable_count_limit_slowdown[zset_score]":     0,
				"memtable_count_limit_stop[default]":            0,
				"memtable_count_limit_stop[index]":              0,
				"memtable_count_limit_stop[metadata]":           0,
				"memtable_count_limit_stop[propagate]":          0,
				"memtable_count_limit_stop[pubsub]":             0,
				"memtable_count_limit_stop[search]":             0,
				"memtable_count_limit_stop[stream]":             0,
				"memtable_count_limit_stop[zset_score]":         0,
				"memtable_flush_pending":                        0,
				"monitor_clients":                               0,
				"next_per_sec":                                  0,
				"num_background_errors":                         0,
				"num_immutable_tables":                          0,
				"num_live_versions":                             8,
				"num_running_compactions":                       0,
				"num_running_flushes":                           0,
				"num_super_version":                             8,
				"pending_compaction_bytes_slowdown[default]":    0,
				"pending_compaction_bytes_slowdown[index]":      0,
				"pending_compaction_bytes_slowdown[metadata]":   0,
				"pending_compaction_bytes_slowdown[propagate]":  0,
				"pending_compaction_bytes_slowdown[pubsub]":     0,
				"pending_compaction_bytes_slowdown[search]":     0,
				"pending_compaction_bytes_slowdown[stream]":     0,
				"pending_compaction_bytes_slowdown[zset_score]": 0,
				"pending_compaction_bytes_stop[default]":        0,
				"pending_compaction_bytes_stop[index]":          0,
				"pending_compaction_bytes_stop[metadata]":       0,
				"pending_compaction_bytes_stop[propagate]":      0,
				"pending_compaction_bytes_stop[pubsub]":         0,
				"pending_compaction_bytes_stop[search]":         0,
				"pending_compaction_bytes_stop[stream]":         0,
				"pending_compaction_bytes_stop[zset_score]":     0,
				"ping_latency_avg":                              0,
				"ping_latency_count":                            5,
				"ping_latency_max":                              0,
				"ping_latency_min":                              0,
				"ping_latency_sum":                              0,
				"prev_per_sec":                                  0,
				"process_id":                                    1,
				"pubsub_channels":                               0,
				"pubsub_patterns":                               0,
				"put_per_sec":                                   0,
				"seek_per_sec":                                  0,
				"server_time_usec":                              1791035032240552,
				"snapshots":                                     0,
				"sync_full":                                     0,
				"sync_partial_err":                              0,
				"sync_partial_ok":                               0,
				"tcp_port":                                      6666,
				"total_commands_processed":                      1,
				"total_connections_received":                    1,
				"total_net_input_bytes":                         17,
				"total_net_output_bytes":                        0,
				"uptime_in_days":                                0,
				"uptime_in_seconds":                             0,
				"used_cpu_sys":                                  25,
				"used_cpu_user":                                 27,
				"used_memory_lua":                               319488,
				"used_memory_rss":                               39284736,
				"used_memory_startup":                           34586624,
			},
		},
		"fails on error on Info": {
			prepare: prepareRedisErrorOnInfo,
		},
		"fails on response from not Redis instance": {
			prepare: prepareRedisWithPikaMetrics,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare(t)

			mx := collr.Collect(context.Background())

			copyTimeRelatedMetrics(mx, test.wantCollected)

			assert.Equal(t, test.wantCollected, mx)
			if len(test.wantCollected) > 0 {
				collecttest.TestMetricsHasAllChartsDimsSkip(t, collr.Charts(), mx, test.dimsSkip)
				ensureCollectedCommandsAddedToCharts(t, collr)
				ensureCollectedDbsAddedToCharts(t, collr)
			}
		})
	}
}

func prepareRedisV609(t *testing.T) *Collector {
	collr := New()
	require.NoError(t, collr.Init(context.Background()))
	collr.rdb = &mockRedisClient{
		result: dataVer609InfoAll,
	}
	return collr
}

func prepareGarnetWithCommandstats(t *testing.T) *Collector {
	return prepareGarnet(t, dataGarnetInfoCommandstats)
}

func prepareGarnetCommandstatsDisabled(t *testing.T) *Collector {
	return prepareGarnet(t, dataGarnetInfoCommandstatsOff)
}

func prepareValkey(t *testing.T) *Collector {
	collr := New()
	require.NoError(t, collr.Init(context.Background()))
	collr.rdb = &mockRedisClient{result: dataValkeyInfoAll}
	return collr
}

func prepareDragonfly(t *testing.T) *Collector {
	collr := New()
	require.NoError(t, collr.Init(context.Background()))
	collr.rdb = &mockRedisClient{result: dataDragonflyInfoAll}
	return collr
}

func prepareKeydb(t *testing.T) *Collector {
	collr := New()
	require.NoError(t, collr.Init(context.Background()))
	collr.rdb = &mockRedisClient{result: dataKeydbInfoAll}
	return collr
}

func prepareKvrocks(t *testing.T) *Collector {
	collr := New()
	require.NoError(t, collr.Init(context.Background()))
	collr.rdb = &mockRedisClient{result: dataKvrocksInfoAll}
	return collr
}

func prepareGarnet(t *testing.T, commandstats []byte) *Collector {
	collr := New()
	require.NoError(t, collr.Init(context.Background()))
	collr.rdb = &mockRedisClient{
		results: map[string][]byte{
			"all":          dataGarnetInfoAll,
			"keyspace":     dataGarnetInfoKeyspace,
			"commandstats": commandstats,
		},
	}
	return collr
}

func prepareRedisErrorOnInfo(t *testing.T) *Collector {
	collr := New()
	require.NoError(t, collr.Init(context.Background()))
	collr.rdb = &mockRedisClient{
		errOnInfo: true,
	}
	return collr
}

func prepareRedisWithPikaMetrics(t *testing.T) *Collector {
	collr := New()
	require.NoError(t, collr.Init(context.Background()))
	collr.rdb = &mockRedisClient{
		result: dataPikaInfoAll,
	}
	return collr
}

func TestCollector_GarnetExtraInfoRequests(t *testing.T) {
	collr := New()
	require.NoError(t, collr.Init(context.Background()))
	// Make the refresh interval longer than any test run so the "at most one
	// request per interval" assertion does not depend on wall-clock time.
	collr.garnetKeyspaceRefreshInterval = time.Hour
	mock := &mockRedisClient{
		results: map[string][]byte{
			"all":          dataGarnetInfoAll,
			"keyspace":     dataGarnetInfoKeyspace,
			"commandstats": dataGarnetInfoCommandstats,
		},
		infoCalls: make(map[string]int),
	}
	collr.rdb = mock

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		assert.NotEmpty(t, collr.Collect(ctx))
	}

	// keyspace is expensive on Garnet (full store scan): at most one request
	// within the refresh interval; commandstats is requested every cycle.
	assert.Equal(t, 3, mock.infoCalls["all"])
	assert.Equal(t, 1, mock.infoCalls["keyspace"])
	assert.Equal(t, 3, mock.infoCalls["commandstats"])
}

// withRedisVersionFirst moves the redis_version line before the garnet_version
// line, as if a Garnet server reported its redis compatibility version first.
func withRedisVersionFirst(info []byte) []byte {
	lines := strings.Split(string(info), "\n")
	redisLine := ""
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(line, "redis_version:") {
			redisLine = line
			continue
		}
		kept = append(kept, line)
	}
	if redisLine == "" {
		return info
	}
	for i, line := range kept {
		if strings.HasPrefix(line, "garnet_version:") {
			kept = append(kept[:i], append([]string{redisLine}, kept[i:]...)...)
			break
		}
	}
	return []byte(strings.Join(kept, "\n"))
}

func Test_extractServerVersion(t *testing.T) {
	tests := map[string]struct {
		info    []byte
		wantSrv string
		wantVer string
	}{
		"redis 6.0.9":                     {info: dataVer609InfoAll, wantSrv: "redis", wantVer: "6.0.9"},
		"valkey 9.1.2":                    {info: dataValkeyInfoAll, wantSrv: "redis", wantVer: "7.2.4"},
		"dragonfly df-v2.0.0":             {info: dataDragonflyInfoAll, wantSrv: "redis", wantVer: "7.4.0"},
		"keydb 6.3.4":                     {info: dataKeydbInfoAll, wantSrv: "redis", wantVer: "6.3.4"},
		"kvrocks 2.17.0":                  {info: dataKvrocksInfoAll, wantSrv: "kvrocks", wantVer: "2.17.0"},
		"garnet 2.2.0":                    {info: dataGarnetInfoAll, wantSrv: "garnet", wantVer: "2.2.0"},
		"garnet with redis_version first": {info: withRedisVersionFirst(dataGarnetInfoAll), wantSrv: "garnet", wantVer: "2.2.0"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			server, ver, err := extractServerVersion(string(test.info))
			require.NoError(t, err)
			assert.Equal(t, test.wantSrv, server)
			assert.Equal(t, test.wantVer, ver.String())
		})
	}
}

func Test_garnet_collectGarnetExtraInfo_joins_sections(t *testing.T) {
	collr := New()
	require.NoError(t, collr.Init(context.Background()))
	collr.garnetKeyspaceRefreshInterval = time.Hour
	collr.rdb = &mockRedisClient{
		results: map[string][]byte{
			"all":          []byte(strings.TrimRight(string(dataGarnetInfoAll), "\n")),
			"keyspace":     []byte(strings.TrimRight(string(dataGarnetInfoKeyspace), "\n")),
			"commandstats": []byte(strings.TrimRight(string(dataGarnetInfoCommandstats), "\n")),
		},
		infoCalls: map[string]int{},
	}

	// base INFO and extra sections without trailing newlines: the join must
	// still put every section on its own lines (exactly one newline boundary,
	// whatever line endings the sections use)
	info := collr.collectGarnetExtraInfo(strings.TrimRight(string(dataGarnetInfoAll), "\n"))
	assert.Contains(t, info, "\n# Keyspace")
	assert.NotContains(t, info, "\n\n# Keyspace")
	assert.Contains(t, info, "\n# Commandstats")
	assert.NotContains(t, info, "\n\n# Commandstats")

	mx := make(map[string]int64)
	collr.collectInfo(mx, info)
	assert.Equal(t, int64(64), mx["arch_bits"])    // base section parsed
	assert.Equal(t, int64(6), mx["db0_keys"])      // keyspace section parsed
	assert.Equal(t, int64(1), mx["cmd_get_calls"]) // commandstats section parsed
}

func TestCollector_GarnetKeyspaceErrorClearsCache(t *testing.T) {
	collr := New()
	require.NoError(t, collr.Init(context.Background()))
	// interval 0: refresh on every cycle, so the test does not depend on timing
	collr.garnetKeyspaceRefreshInterval = 0
	mock := &mockRedisClient{
		results: map[string][]byte{
			"all":          dataGarnetInfoAll,
			"keyspace":     dataGarnetInfoKeyspace,
			"commandstats": dataGarnetInfoCommandstats,
		},
		infoCalls: map[string]int{},
	}
	collr.rdb = mock

	ctx := context.Background()
	mx := collr.Collect(ctx)
	assert.Equal(t, int64(6), mx["db0_keys"])

	mock.errOnKeyspace = true
	mx = collr.Collect(ctx)
	assert.NotContains(t, mx, "db0_keys")

	mx = collr.Collect(ctx)
	assert.NotContains(t, mx, "db0_keys")
}
func ensureCollectedCommandsAddedToCharts(t *testing.T, collr *Collector) {
	for _, id := range []string{
		chartCommandsCalls.ID,
		chartCommandsUsec.ID,
		chartCommandsUsecPerSec.ID,
	} {
		chart := collr.Charts().Get(id)
		require.NotNilf(t, chart, "'%s' chart is not in charts", id)
		assert.Lenf(t, chart.Dims, len(collr.collectedCommands),
			"'%s' chart unexpected number of dimensions", id)
	}
}

func ensureCollectedDbsAddedToCharts(t *testing.T, collr *Collector) {
	for _, id := range []string{
		chartKeys.ID,
		chartExpiresKeys.ID,
	} {
		chart := collr.Charts().Get(id)
		require.NotNilf(t, chart, "'%s' chart is not in charts", id)
		assert.Lenf(t, chart.Dims, len(collr.collectedDbs),
			"'%s' chart unexpected number of dimensions", id)
	}
}

func copyTimeRelatedMetrics(dst, src map[string]int64) {
	for k, v := range src {
		switch {
		case k == "rdb_last_save_time",
			strings.HasPrefix(k, "ping_latency"):

			if _, ok := dst[k]; ok {
				dst[k] = v
			}
		}
	}
}

type mockRedisClient struct {
	errOnInfo     bool
	errOnKeyspace bool
	result        []byte
	results       map[string][]byte
	infoCalls     map[string]int
	calledClose   bool
}

func (m *mockRedisClient) Info(_ context.Context, sections ...string) (cmd *redis.StringCmd) {
	section := "all"
	if len(sections) > 0 {
		section = sections[0]
	}
	if m.infoCalls != nil {
		m.infoCalls[section]++
	}
	if m.errOnKeyspace && section == "keyspace" {
		return redis.NewStringResult("", errors.New("error on Info keyspace"))
	}
	if m.results != nil {
		result, ok := m.results[section]
		if !ok {
			return redis.NewStringResult("", errors.New("error on Info"))
		}
		return redis.NewStringResult(string(result), nil)
	}
	if m.errOnInfo {
		return redis.NewStringResult("", errors.New("error on Info"))
	}
	return redis.NewStringResult(string(m.result), nil)
}

func (m *mockRedisClient) Ping(_ context.Context) (cmd *redis.StatusCmd) {
	return redis.NewStatusResult("PONG", nil)
}

func (m *mockRedisClient) SlowLogGet(ctx context.Context, num int64) *redis.SlowLogCmd {
	cmd := redis.NewSlowLogCmd(ctx, "slowlog", "get", num)
	cmd.SetVal([]redis.SlowLog{})
	return cmd
}

func (m *mockRedisClient) Close() error {
	m.calledClose = true
	return nil
}
