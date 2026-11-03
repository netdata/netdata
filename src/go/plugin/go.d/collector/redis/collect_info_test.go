// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_CommandTimingAvailability(t *testing.T) {
	for name, test := range map[string]struct {
		server string
		stats  string
		want   map[string]int64
	}{
		"garnet placeholders": {
			server: "garnet",
			stats:  "calls=3,usec=0,usec_per_call=0.00",
			want:   map[string]int64{"cmd_get_calls": 3},
		},
		"redis measured zero": {
			server: "redis",
			stats:  "calls=3,usec=0,usec_per_call=0.00",
			want:   map[string]int64{"cmd_get_calls": 3, "cmd_get_usec": 0, "cmd_get_usec_per_call": 0},
		},
		"kvrocks measured duration": {
			server: "kvrocks",
			stats:  "calls=3,usec=6,usec_per_call=2.00",
			want:   map[string]int64{"cmd_get_calls": 3, "cmd_get_usec": 6, "cmd_get_usec_per_call": 2000},
		},
	} {
		t.Run(name, func(t *testing.T) {
			info := fmt.Sprintf(
				"# Server\n%s_version:2.2.0\n\n# Commandstats\ncmdstat_get:%s\n",
				test.server,
				test.stats,
			)
			c := newInfoOnlyCollector(t, map[string][]byte{"all": []byte(info), "keyspace": {}, "commandstats": {}})
			test.want["ping_latency_count"] = 0
			test.want["ping_latency_sum"] = 0
			assert.Equal(t, test.want, c.Collect(context.Background()))
			assert.Len(t, c.Charts().Get(chartCommandsCalls.ID).Dims, 1)
			for _, id := range []string{chartCommandsUsec.ID, chartCommandsUsecPerSec.ID} {
				if test.server == "garnet" {
					assert.Empty(t, c.Charts().Get(id).Dims)
				} else {
					assert.Len(t, c.Charts().Get(id).Dims, 1)
				}
			}
		})
	}
}

func TestCollector_GarnetSamplingAvailability(t *testing.T) {
	for name, test := range map[string]struct {
		monitor string
		want    map[string]int64
	}{
		"disabled": {monitor: "disabled", want: map[string]int64{}},
		"enabled": {
			monitor: "enabled",
			want: map[string]int64{
				"connected_clients": 2, "total_commands_processed": 12,
				"total_connections_received": 3, "total_net_input_bytes": 128,
				"total_net_output_bytes": 256, "rejected_connections": 0,
				"keyspace_hits": 3, "keyspace_misses": 1, "keyspace_hit_rate": 75000,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			info := "# Server\ngarnet_version:2.2.0\nmonitor_task:" + test.monitor + "\nuptime_in_seconds:60\n\n" +
				"# Stats\ntotal_commands_processed:12\ntotal_connections_received:3\nrejected_connections:0\n" +
				"total_net_input_bytes:128\ntotal_net_output_bytes:256\ntotal_found:3\ntotal_notfound:1\n\n" +
				"# Clients\nconnected_clients:2\n# Memory\nproc_physical_memory_size:4096\n\n" +
				"# Persistence_DB_0\nrdb_last_bgsave_status:ok\n"
			c := newInfoOnlyCollector(t, map[string][]byte{
				"all": []byte(info), "keyspace": []byte("# Keyspace\ndb0:keys=6,expires=1\n"),
				"commandstats": []byte("# Commandstats\ncmdstat_get:calls=3,usec=0,usec_per_call=0.00\n"),
			})
			for k, v := range map[string]int64{
				"uptime_in_seconds": 60, "used_memory_rss": 4096, "rdb_last_bgsave_status": 0,
				"db0_keys": 6, "db0_expires_keys": 1, "cmd_get_calls": 3,
				"ping_latency_count": 0, "ping_latency_sum": 0,
			} {
				test.want[k] = v
			}
			assert.Equal(t, test.want, c.Collect(context.Background()))
		})
	}
}

func TestCollector_ReplicationDowntimeAvailability(t *testing.T) {
	for name, test := range map[string]struct {
		status   string
		duration string
		want     map[string]int64
	}{
		"up without duration": {status: "up", want: map[string]int64{
			"master_link_status_up": 1, "master_link_status_down": 0, "master_link_down_since_seconds": 0,
		}},
		"down without duration": {status: "down", want: map[string]int64{
			"master_link_status_up": 0, "master_link_status_down": 1,
		}},
		"down with measured duration": {
			status:   "down",
			duration: "master_link_down_since_seconds:15\n",
			want: map[string]int64{
				"master_link_status_up": 0, "master_link_status_down": 1, "master_link_down_since_seconds": 15,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := newInfoOnlyCollector(t, map[string][]byte{
				"all": []byte("# Server\nkvrocks_version:2.17.0\n\n# Replication\n" +
					"master_link_status:" + test.status + "\nmaster_last_io_seconds_ago:20\n" + test.duration),
			})
			test.want["master_last_io_seconds_ago"] = 20
			test.want["ping_latency_count"] = 0
			test.want["ping_latency_sum"] = 0
			assert.Equal(t, test.want, c.Collect(context.Background()))
		})
	}
}

func TestCollector_RejectedConnectionsAvailability(t *testing.T) {
	for name, test := range map[string]struct {
		value string
		want  map[string]int64
	}{
		"unsupported sentinel": {value: "-1", want: map[string]int64{}},
		"measured zero":        {value: "0", want: map[string]int64{"rejected_connections": 0}},
		"measured rejections":  {value: "3", want: map[string]int64{"rejected_connections": 3}},
	} {
		t.Run(name, func(t *testing.T) {
			c := newInfoOnlyCollector(t, map[string][]byte{
				"all": []byte("# Server\nredis_version:7.4.0\n\n# Stats\nrejected_connections:" + test.value + "\n"),
			})
			test.want["ping_latency_count"] = 0
			test.want["ping_latency_sum"] = 0
			assert.Equal(t, test.want, c.Collect(context.Background()))
		})
	}
}

func TestCollector_InvalidLastSaveTime(t *testing.T) {
	c := newInfoOnlyCollector(t, map[string][]byte{
		"all": []byte("# Server\nredis_version:7.4.0\n\n# Persistence\nrdb_last_save_time:never\n"),
	})

	assert.Equal(t, map[string]int64{"ping_latency_count": 0, "ping_latency_sum": 0}, c.Collect(context.Background()))
}

// newInfoOnlyCollector returns an initialized collector that serves the given INFO sections and sends no PINGs.
func newInfoOnlyCollector(t *testing.T, info map[string][]byte) *Collector {
	t.Helper()
	c := newTestCollector(t, &mockRedisClient{
		info: info,
	})
	c.PingSamples = 0
	return c
}

// The parser is O(INFO bytes + metrics). Repeated cycles reuse discovered dimensions.
func BenchmarkCollectorCollectInfo(b *testing.B) {
	garnetEnabled := strings.ReplaceAll(string(dataGarnetInfoAll), "monitor_task:disabled", "monitor_task:enabled")
	for name, test := range map[string]struct{ server, info string }{
		"redis":           {server: "redis", info: string(dataVer609InfoAll)},
		"kvrocks":         {server: "kvrocks", info: string(dataKvrocksInfoAll)},
		"garnet_disabled": {server: "garnet", info: string(dataGarnetInfoAll) + string(dataGarnetInfoCommandstats)},
		"garnet_enabled":  {server: "garnet", info: garnetEnabled + string(dataGarnetInfoCommandstats)},
	} {
		b.Run(name, func(b *testing.B) {
			c := New()
			require.NoError(b, c.Init(context.Background()))
			c.server = test.server
			c.collectInfo(make(map[string]int64), test.info)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				c.collectInfo(make(map[string]int64), test.info)
			}
		})
	}
}
