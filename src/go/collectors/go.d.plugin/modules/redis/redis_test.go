// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/netdata/go.d.plugin/pkg/tlscfg"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	pikaInfoAll, _ = os.ReadFile("testdata/pika/info_all.txt")
	v609InfoAll, _ = os.ReadFile("testdata/v6.0.9/info_all.txt")
)

func Test_Testdata(t *testing.T) {
	for name, data := range map[string][]byte{
		"pikaInfoAll": pikaInfoAll,
		"v609InfoAll": v609InfoAll,
	} {
		require.NotNilf(t, data, name)
	}
}

func TestNew(t *testing.T) {
	assert.IsType(t, (*Redis)(nil), New())
}

func TestRedis_Init(t *testing.T) {
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
			rdb := New()
			rdb.Config = test.config

			if test.wantFail {
				assert.False(t, rdb.Init())
			} else {
				assert.True(t, rdb.Init())
			}
		})
	}
}

func TestRedis_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func(t *testing.T) *Redis
		wantFail bool
	}{
		"success on valid response v6.0.9": {
			prepare: prepareRedisV609,
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
			rdb := test.prepare(t)

			if test.wantFail {
				assert.False(t, rdb.Check())
			} else {
				assert.True(t, rdb.Check())
			}
		})
	}
}

func TestRedis_Charts(t *testing.T) {
	rdb := New()
	require.True(t, rdb.Init())

	assert.NotNil(t, rdb.Charts())
}

func TestRedis_Cleanup(t *testing.T) {
	rdb := New()
	assert.NotPanics(t, rdb.Cleanup)

	require.True(t, rdb.Init())
	m := &mockRedisClient{}
	rdb.rdb = m

	rdb.Cleanup()

	assert.True(t, m.calledClose)
}

func TestRedis_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func(t *testing.T) *Redis
		wantCollected map[string]int64
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
				"rdb_last_save_time":              56978305,
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
		"fails on error on Info": {
			prepare: prepareRedisErrorOnInfo,
		},
		"fails on response from not Redis instance": {
			prepare: prepareRedisWithPikaMetrics,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			rdb := test.prepare(t)

			ms := rdb.Collect()

			copyTimeRelatedMetrics(ms, test.wantCollected)

			assert.Equal(t, test.wantCollected, ms)
			if len(test.wantCollected) > 0 {
				ensureCollectedHasAllChartsDimsVarsIDs(t, rdb, ms)
				ensureCollectedCommandsAddedToCharts(t, rdb)
				ensureCollectedDbsAddedToCharts(t, rdb)
			}
		})
	}
}

func prepareRedisV609(t *testing.T) *Redis {
	rdb := New()
	require.True(t, rdb.Init())
	rdb.rdb = &mockRedisClient{
		result: v609InfoAll,
	}
	return rdb
}

func prepareRedisErrorOnInfo(t *testing.T) *Redis {
	rdb := New()
	require.True(t, rdb.Init())
	rdb.rdb = &mockRedisClient{
		errOnInfo: true,
	}
	return rdb
}

func prepareRedisWithPikaMetrics(t *testing.T) *Redis {
	rdb := New()
	require.True(t, rdb.Init())
	rdb.rdb = &mockRedisClient{
		result: pikaInfoAll,
	}
	return rdb
}

func ensureCollectedHasAllChartsDimsVarsIDs(t *testing.T, rdb *Redis, ms map[string]int64) {
	for _, chart := range *rdb.Charts() {
		if chart.Obsolete {
			continue
		}
		for _, dim := range chart.Dims {
			_, ok := ms[dim.ID]
			assert.Truef(t, ok, "chart '%s' dim '%s': no dim in collected", dim.ID, chart.ID)
		}
		for _, v := range chart.Vars {
			_, ok := ms[v.ID]
			assert.Truef(t, ok, "chart '%s' dim '%s': no dim in collected", v.ID, chart.ID)
		}
	}
}

func ensureCollectedCommandsAddedToCharts(t *testing.T, rdb *Redis) {
	for _, id := range []string{
		chartCommandsCalls.ID,
		chartCommandsUsec.ID,
		chartCommandsUsecPerSec.ID,
	} {
		chart := rdb.Charts().Get(id)
		require.NotNilf(t, chart, "'%s' chart is not in charts", id)
		assert.Lenf(t, chart.Dims, len(rdb.collectedCommands),
			"'%s' chart unexpected number of dimensions", id)
	}
}

func ensureCollectedDbsAddedToCharts(t *testing.T, rdb *Redis) {
	for _, id := range []string{
		chartKeys.ID,
		chartExpiresKeys.ID,
	} {
		chart := rdb.Charts().Get(id)
		require.NotNilf(t, chart, "'%s' chart is not in charts", id)
		assert.Lenf(t, chart.Dims, len(rdb.collectedDbs),
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
	calledClose bool
}

func (m *mockRedisClient) Info(_ context.Context, _ ...string) (cmd *redis.StringCmd) {
	if m.errOnInfo {
		cmd = redis.NewStringResult("", errors.New("error on Info"))
	} else {
		cmd = redis.NewStringResult(string(m.result), nil)
	}
	return cmd
}

func (m *mockRedisClient) Ping(_ context.Context) (cmd *redis.StatusCmd) {
	return redis.NewStatusResult("PONG", nil)
}

func (m *mockRedisClient) Close() error {
	m.calledClose = true
	return nil
}
