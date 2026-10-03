// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"context"
	"errors"
	"maps"
	"os"
	"strings"
	"testing"

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
	"processor_count":                             24,
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
	"total_system_memory":                         134886170624,
	"total_system_memory(MB)":                     128637,
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
	errOnInfo   bool
	result      []byte
	results     map[string][]byte
	infoCalls   map[string]int
	calledClose bool
}

func (m *mockRedisClient) Info(_ context.Context, sections ...string) (cmd *redis.StringCmd) {
	section := "all"
	if len(sections) > 0 {
		section = sections[0]
	}
	if m.infoCalls != nil {
		m.infoCalls[section]++
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
