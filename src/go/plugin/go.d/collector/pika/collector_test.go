// SPDX-License-Identifier: GPL-3.0-or-later

package pika

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataRedisInfoAll, _  = os.ReadFile("testdata/redis/info_all.txt")
	dataVer340InfoAll, _ = os.ReadFile("testdata/v3.4.0/info_all.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":    dataConfigJSON,
		"dataConfigYAML":    dataConfigYAML,
		"dataRedisInfoAll":  dataRedisInfoAll,
		"dataVer340InfoAll": dataVer340InfoAll,
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
			config: New().Config,
		},
		"fails on unset 'address'": {
			wantFail: true,
			config:   Config{Address: ""},
		},
		"fails on invalid 'address' format": {
			wantFail: true,
			config:   Config{Address: "127.0.0.1:9221"},
		},
		"fails on invalid TLSCA": {
			wantFail: true,
			config: Config{
				Address:   "redis://@127.0.0.1:9221",
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
		"success on valid response v3.4.0": {
			prepare: preparePikaV340,
		},
		"fails on error on Info": {
			wantFail: true,
			prepare:  preparePikaErrorOnInfo,
		},
		"fails on response from not Pika instance": {
			wantFail: true,
			prepare:  preparePikaWithRedisMetrics,
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
	collr.pdb = m

	collr.Cleanup(context.Background())

	assert.True(t, m.calledClose)
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func(t *testing.T) *Collector
		wantCollected map[string]int64
	}{
		"success on valid response v3.4.0": {
			prepare: preparePikaV340,
			wantCollected: map[string]int64{
				"cmd_INFO_calls":             1,
				"cmd_SET_calls":              2,
				"arch_bits":                  64,
				"connected_clients":          1,
				"connected_slaves":           0,
				"db0_hashes_expires_keys":    0,
				"db0_hashes_invalid_keys":    0,
				"db0_hashes_keys":            0,
				"db0_lists_expires_keys":     0,
				"db0_lists_invalid_keys":     0,
				"db0_lists_keys":             0,
				"db0_sets_expires_keys":      0,
				"db0_sets_invalid_keys":      0,
				"db0_sets_keys":              0,
				"db0_strings_expires_keys":   0,
				"db0_strings_invalid_keys":   0,
				"db0_strings_keys":           0,
				"db0_zsets_expires_keys":     0,
				"db0_zsets_invalid_keys":     0,
				"db0_zsets_keys":             0,
				"instantaneous_ops_per_sec":  0,
				"log_size":                   4272814,
				"process_id":                 1,
				"server_id":                  1,
				"sync_thread_num":            6,
				"tcp_port":                   9221,
				"thread_num":                 1,
				"total_commands_processed":   3,
				"total_connections_received": 3,
				"uptime_in_days":             1,
				"uptime_in_seconds":          1884,
				"used_cpu_sys":               158200,
				"used_cpu_sys_children":      30,
				"used_cpu_user":              22050,
				"used_cpu_user_children":     20,
				"used_memory":                8198,
			},
		},
		"fails on error on Info": {
			prepare: preparePikaErrorOnInfo,
		},
		"fails on response from not Pika instance": {
			prepare: preparePikaWithRedisMetrics,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare(t)

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantCollected, mx)
			if len(test.wantCollected) > 0 {
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
				ensureCollectedCommandsAddedToCharts(t, collr)
				ensureCollectedDbsAddedToCharts(t, collr)
			}
		})
	}
}

func preparePikaV340(t *testing.T) *Collector {
	collr := New()
	require.NoError(t, collr.Init(context.Background()))
	collr.pdb = &mockRedisClient{
		result: dataVer340InfoAll,
	}
	return collr
}

func preparePikaErrorOnInfo(t *testing.T) *Collector {
	collr := New()
	require.NoError(t, collr.Init(context.Background()))
	collr.pdb = &mockRedisClient{
		errOnInfo: true,
	}
	return collr
}

func preparePikaWithRedisMetrics(t *testing.T) *Collector {
	collr := New()
	require.NoError(t, collr.Init(context.Background()))
	collr.pdb = &mockRedisClient{
		result: dataRedisInfoAll,
	}
	return collr
}

func ensureCollectedCommandsAddedToCharts(t *testing.T, collr *Collector) {
	for _, id := range []string{
		chartCommandsCalls.ID,
	} {
		chart := collr.Charts().Get(id)
		require.NotNilf(t, chart, "'%s' chart is not in charts", id)
		assert.Lenf(t, chart.Dims, len(collr.collectedCommands),
			"'%s' chart unexpected number of dimensions", id)
	}
}

func ensureCollectedDbsAddedToCharts(t *testing.T, collr *Collector) {
	for _, id := range []string{
		chartDbStringsKeys.ID,
		chartDbStringsExpiresKeys.ID,
		chartDbStringsInvalidKeys.ID,
		chartDbHashesKeys.ID,
		chartDbHashesExpiresKeys.ID,
		chartDbHashesInvalidKeys.ID,
		chartDbListsKeys.ID,
		chartDbListsExpiresKeys.ID,
		chartDbListsInvalidKeys.ID,
		chartDbZsetsKeys.ID,
		chartDbZsetsExpiresKeys.ID,
		chartDbZsetsInvalidKeys.ID,
		chartDbSetsKeys.ID,
		chartDbSetsExpiresKeys.ID,
		chartDbSetsInvalidKeys.ID,
	} {
		chart := collr.Charts().Get(id)
		require.NotNilf(t, chart, "'%s' chart is not in charts", id)
		assert.Lenf(t, chart.Dims, len(collr.collectedDbs),
			"'%s' chart unexpected number of dimensions", id)
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

func (m *mockRedisClient) Close() error {
	m.calledClose = true
	return nil
}
