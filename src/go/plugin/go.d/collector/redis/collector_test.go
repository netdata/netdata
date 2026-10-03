// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
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
			prepare:       prepareRedisV609,
			wantCollected: readInfoMetrics(t, "v6.0.9"),
		},
		"success on valid response garnet (commandstats enabled)": {
			prepare:       prepareGarnetWithCommandstats,
			wantCollected: garnetWantCollected(t, true),
			dimsSkip:      skipGarnetMissingDims,
		},
		"success on valid response garnet (commandstats disabled)": {
			prepare:       prepareGarnetCommandstatsDisabled,
			wantCollected: garnetWantCollected(t, false),
			dimsSkip:      skipGarnetMissingDims,
		},
		"success on valid response valkey 9.1.2": {
			prepare:       prepareValkey,
			wantCollected: readInfoMetrics(t, "valkey"),
		},
		"success on valid response dragonfly df-v2.0.0": {
			prepare:       prepareDragonfly,
			dimsSkip:      skipDragonflyMissingDims,
			wantCollected: readInfoMetrics(t, "dragonfly"),
		},
		"success on valid response keydb 6.3.4": {
			prepare:       prepareKeydb,
			wantCollected: readInfoMetrics(t, "keydb"),
		},
		"success on valid response kvrocks 2.17.0": {
			prepare:       prepareKvrocks,
			dimsSkip:      skipKvrocksMissingDims,
			wantCollected: readInfoMetrics(t, "kvrocks"),
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
		wantDims := len(collr.collectedCommands)
		if collr.server == "garnet" && id != chartCommandsCalls.ID {
			wantDims = 0
		}
		assert.Lenf(t, chart.Dims, wantDims,
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
