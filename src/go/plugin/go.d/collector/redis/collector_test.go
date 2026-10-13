// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
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

func TestCollector_ConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_ConfigSchemaMatchesMetadata(t *testing.T) {
	// Defaults are not compared: the form pre-fills autodetection_retry with 60 so UI-created jobs retry a failed
	// start, while file-based jobs default to 0 as documented.
	collecttest.AssertConfigSchemaMatchesMetadata(t, "config_schema.json", "metadata.yaml")
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
			config: Config{
				Address: "",
			},
		},
		"fails on invalid 'address' format": {
			wantFail: true,
			config: Config{
				Address: "127.0.0.1:6379",
			},
		},
		"fails on invalid TLSCA": {
			wantFail: true,
			config: Config{
				Address: "redis://127.0.0.1:6379",
				TLSConfig: tlscfg.TLSConfig{
					TLSCA: "testdata/tls",
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

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func(t *testing.T) *Collector
		wantFail bool
	}{
		"success on valid response v6.0.9": {
			prepare: prepareInfo(dataVer609InfoAll),
		},
		"success on valid response garnet": {
			prepare: prepareGarnet(dataGarnetInfoCommandstats),
		},
		"success on garnet with commandstats disabled": {
			prepare: prepareGarnet(dataGarnetInfoCommandstatsOff),
		},
		"success on valid response valkey 9.1.2": {
			prepare: prepareInfo(dataValkeyInfoAll),
		},
		"success on valid response dragonfly df-v2.0.0": {
			prepare: prepareInfo(dataDragonflyInfoAll),
		},
		"success on valid response keydb 6.3.4": {
			prepare: prepareInfo(dataKeydbInfoAll),
		},
		"success on valid response kvrocks 2.17.0": {
			prepare: prepareInfo(dataKvrocksInfoAll),
		},
		"fails on error on Info": {
			wantFail: true,
			prepare:  prepareInfoError,
		},
		"fails on response from not Redis instance": {
			wantFail: true,
			prepare:  prepareInfo(dataPikaInfoAll),
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

	m := &mockRedisClient{}
	collr = newTestCollector(t, m)

	collr.Cleanup(context.Background())

	assert.True(t, m.calledClose)
}

// garnetMissingDims holds chart dimension IDs whose backing INFO fields Garnet
// does not emit (no equivalent data): CPU counters, redis allocator memory
// fields, client timeout/tracking, expiration/eviction and persistence fields.
// The captured server also has periodic sampling disabled, so Stats/Clients
// dimensions stay empty alongside the unsupported fields.
var garnetMissingDims = map[string]bool{
	"connected_clients":           true,
	"keyspace_hit_rate":           true,
	"rejected_connections":        true,
	"total_commands_processed":    true,
	"total_connections_received":  true,
	"total_net_input_bytes":       true,
	"total_net_output_bytes":      true,
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

// dragonflyMissingDims holds chart dimension IDs whose backing INFO fields
// Dragonfly does not measure; their chart dimensions stay empty for Dragonfly.
var dragonflyMissingDims = map[string]bool{
	"rejected_connections":        true,
	"clients_in_timeout_table":    true,
	"mem_fragmentation_ratio":     true,
	"rdb_changes_since_last_save": true,
	"rdb_current_bgsave_time_sec": true,
	"rdb_last_save_time":          true,
	"tracking_clients":            true,
	"used_memory_dataset":         true,
	"used_memory_scripts":         true,
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

func skipDims(ids map[string]bool) func(*collectorapi.Chart, *collectorapi.Dim) bool {
	return func(_ *collectorapi.Chart, dim *collectorapi.Dim) bool { return ids[dim.ID] }
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func(t *testing.T) *Collector
		wantCollected map[string]int64
		dimsSkip      func(chart *collectorapi.Chart, dim *collectorapi.Dim) bool
	}{
		"success on valid response v6.0.9": {
			prepare:       prepareInfo(dataVer609InfoAll),
			wantCollected: readInfoMetrics(t, "v6.0.9"),
		},
		"success on valid response garnet (commandstats enabled)": {
			prepare:       prepareGarnet(dataGarnetInfoCommandstats),
			wantCollected: garnetWantCollected(t, true),
			dimsSkip:      skipDims(garnetMissingDims),
		},
		"success on valid response garnet (commandstats disabled)": {
			prepare:       prepareGarnet(dataGarnetInfoCommandstatsOff),
			wantCollected: garnetWantCollected(t, false),
			dimsSkip:      skipDims(garnetMissingDims),
		},
		"success on valid response valkey 9.1.2": {
			prepare:       prepareInfo(dataValkeyInfoAll),
			wantCollected: readInfoMetrics(t, "valkey"),
		},
		"success on valid response dragonfly df-v2.0.0": {
			prepare:       prepareInfo(dataDragonflyInfoAll),
			dimsSkip:      skipDims(dragonflyMissingDims),
			wantCollected: readInfoMetrics(t, "dragonfly"),
		},
		"success on valid response keydb 6.3.4": {
			prepare:       prepareInfo(dataKeydbInfoAll),
			wantCollected: readInfoMetrics(t, "keydb"),
		},
		"success on valid response kvrocks 2.17.0": {
			prepare:       prepareInfo(dataKvrocksInfoAll),
			dimsSkip:      skipDims(kvrocksMissingDims),
			wantCollected: readInfoMetrics(t, "kvrocks"),
		},
		"fails on error on Info": {
			prepare: prepareInfoError,
		},
		"fails on response from not Redis instance": {
			prepare: prepareInfo(dataPikaInfoAll),
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
				ensureCollectedDBsAddedToCharts(t, collr)
			}
		})
	}
}

// readInfoMetrics loads independently specified expectations kept beside each INFO fixture.
func readInfoMetrics(t *testing.T, server string) map[string]int64 {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", server, "metrics.json"))
	require.NoError(t, err)
	var want map[string]int64
	require.NoError(t, json.Unmarshal(data, &want))
	require.NotEmpty(t, want)
	return want
}

// garnetCommandstatsMetrics is the cmd_* subset of the captured Garnet
// commandstats fixture; its hardcoded timing placeholders are not measurements.
var garnetCommandstatsMetrics = map[string]int64{
	"cmd_client|setinfo_calls": 2,
	"cmd_dbsize_calls":         1,
	"cmd_get_calls":            1,
	"cmd_hello_calls":          1,
	"cmd_hset_calls":           1,
	"cmd_info_calls":           4,
	"cmd_lpush_calls":          1,
	"cmd_ping_calls":           1,
	"cmd_set_calls":            3,
}

// garnetWantCollected returns a fresh complete expected map for the garnet
// fixture; a per-call copy is required because copyTimeRelatedMetrics mutates it.
func garnetWantCollected(t *testing.T, withCommandstats bool) map[string]int64 {
	t.Helper()
	want := readInfoMetrics(t, "garnet")
	if withCommandstats {
		maps.Copy(want, garnetCommandstatsMetrics)
	}
	return want
}

func ensureCollectedCommandsAddedToCharts(t *testing.T, collr *Collector) {
	for _, id := range []string{
		chartCommandsCalls.ID,
		chartCommandsUsec.ID,
		chartCommandsUsecPerSec.ID,
	} {
		chart := collr.Charts().Get(id)
		require.NotNilf(t, chart, "'%s' chart is not in charts", id)
		wantDims := len(collr.collectedCommands)
		if collr.server == "garnet" && id != chartCommandsCalls.ID {
			wantDims = 0
		}
		assert.Lenf(t, chart.Dims, wantDims,
			"'%s' chart unexpected number of dimensions", id)
	}
}

func ensureCollectedDBsAddedToCharts(t *testing.T, collr *Collector) {
	for _, id := range []string{
		chartKeys.ID,
		chartExpiresKeys.ID,
	} {
		chart := collr.Charts().Get(id)
		require.NotNilf(t, chart, "'%s' chart is not in charts", id)
		assert.Lenf(t, chart.Dims, len(collr.collectedDBs),
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

func prepareInfo(infoAll []byte) func(t *testing.T) *Collector {
	return func(t *testing.T) *Collector {
		return newTestCollector(t, &mockRedisClient{
			info: map[string][]byte{"all": infoAll},
		})
	}
}

func prepareGarnet(commandstats []byte) func(t *testing.T) *Collector {
	return func(t *testing.T) *Collector {
		return newTestCollector(t, newGarnetMock(commandstats))
	}
}

func prepareInfoError(t *testing.T) *Collector {
	return newTestCollector(t, &mockRedisClient{})
}

func newGarnetMock(commandstats []byte) *mockRedisClient {
	return &mockRedisClient{
		info: map[string][]byte{
			"all":          dataGarnetInfoAll,
			"keyspace":     dataGarnetInfoKeyspace,
			"commandstats": commandstats,
		},
	}
}

func newTestCollector(t *testing.T, rdb *mockRedisClient) *Collector {
	t.Helper()
	collr := New()
	require.NoError(t, collr.Init(context.Background()))
	// Replace the client Init created; it never connected.
	require.NoError(t, collr.closeClient())
	collr.setClient(rdb)
	return collr
}

type mockRedisClient struct {
	info        map[string][]byte // INFO reply per section; requesting a missing section fails
	infoCalls   map[string]int
	calledClose bool
}

func (m *mockRedisClient) Info(_ context.Context, sections ...string) *redis.StringCmd {
	section := "all"
	if len(sections) > 0 {
		section = sections[0]
	}
	if m.infoCalls == nil {
		m.infoCalls = make(map[string]int)
	}
	m.infoCalls[section]++

	reply, ok := m.info[section]
	if !ok {
		return redis.NewStringResult("", fmt.Errorf("error on INFO %s", section))
	}
	return redis.NewStringResult(string(reply), nil)
}

func (m *mockRedisClient) Ping(context.Context) *redis.StatusCmd {
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
