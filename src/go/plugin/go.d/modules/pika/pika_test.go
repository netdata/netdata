// SPDX-License-Identifier: GPL-3.0-or-later

package pika

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/tlscfg"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestPika_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Pika{}, dataConfigJSON, dataConfigYAML)
}

func TestPika_Init(t *testing.T) {
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
			pika := New()
			pika.Config = test.config

			if test.wantFail {
				assert.Error(t, pika.Init())
			} else {
				assert.NoError(t, pika.Init())
			}
		})
	}
}

func TestPika_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func(t *testing.T) *Pika
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
			pika := test.prepare(t)

			if test.wantFail {
				assert.Error(t, pika.Check())
			} else {
				assert.NoError(t, pika.Check())
			}
		})
	}
}

func TestPika_Charts(t *testing.T) {
	pika := New()
	require.NoError(t, pika.Init())

	assert.NotNil(t, pika.Charts())
}

func TestPika_Cleanup(t *testing.T) {
	pika := New()
	assert.NotPanics(t, pika.Cleanup)

	require.NoError(t, pika.Init())
	m := &mockRedisClient{}
	pika.pdb = m

	pika.Cleanup()

	assert.True(t, m.calledClose)
}

func TestPika_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func(t *testing.T) *Pika
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
			pika := test.prepare(t)

			ms := pika.Collect()

			assert.Equal(t, test.wantCollected, ms)
			if len(test.wantCollected) > 0 {
				ensureCollectedHasAllChartsDimsVarsIDs(t, pika, ms)
				ensureCollectedCommandsAddedToCharts(t, pika)
				ensureCollectedDbsAddedToCharts(t, pika)
			}
		})
	}
}

func preparePikaV340(t *testing.T) *Pika {
	pika := New()
	require.NoError(t, pika.Init())
	pika.pdb = &mockRedisClient{
		result: dataVer340InfoAll,
	}
	return pika
}

func preparePikaErrorOnInfo(t *testing.T) *Pika {
	pika := New()
	require.NoError(t, pika.Init())
	pika.pdb = &mockRedisClient{
		errOnInfo: true,
	}
	return pika
}

func preparePikaWithRedisMetrics(t *testing.T) *Pika {
	pika := New()
	require.NoError(t, pika.Init())
	pika.pdb = &mockRedisClient{
		result: dataRedisInfoAll,
	}
	return pika
}

func ensureCollectedHasAllChartsDimsVarsIDs(t *testing.T, pika *Pika, ms map[string]int64) {
	for _, chart := range *pika.Charts() {
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

func ensureCollectedCommandsAddedToCharts(t *testing.T, pika *Pika) {
	for _, id := range []string{
		chartCommandsCalls.ID,
	} {
		chart := pika.Charts().Get(id)
		require.NotNilf(t, chart, "'%s' chart is not in charts", id)
		assert.Lenf(t, chart.Dims, len(pika.collectedCommands),
			"'%s' chart unexpected number of dimensions", id)
	}
}

func ensureCollectedDbsAddedToCharts(t *testing.T, pika *Pika) {
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
		chart := pika.Charts().Get(id)
		require.NotNilf(t, chart, "'%s' chart is not in charts", id)
		assert.Lenf(t, chart.Dims, len(pika.collectedDbs),
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
