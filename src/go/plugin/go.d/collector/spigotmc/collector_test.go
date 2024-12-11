// SPDX-License-Identifier: GPL-3.0-or-later

package spigotmc

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataRespTsp = `
§6TPS from last 1m, 5m, 15m: §a*20.0, §a*20.0, §a*20.0
§6Current Memory Usage: §a332/392 mb (Max: 6008 mb)
`
	dataRespListClean             = `There are 4 of a max 50 players online: player1, player2, player3, player4`
	dataRespListDoubleS           = `There are 4 of a max 50 players online: player1, player2, player3, player4`
	dataRespListDoubleSWithHidden = `§6There are §c3§6/§c1§6 out of maximum §c50§6 players online.`
	dataRespListDoubleSNonEng     = `§6当前有 §c4§6 个玩家在线,最大在线人数为 §c50§6 个玩家.`
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
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
		"fails with default config": {
			wantFail: true,
			config:   New().Config,
		},
		"fails if address not set": {
			wantFail: true,
			config: func() Config {
				conf := New().Config
				conf.Address = ""
				conf.Password = "pass"
				return conf
			}(),
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

func TestCollector_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *Collector
	}{
		"not initialized": {
			prepare: func() *Collector {
				return New()
			},
		},
		"after check": {
			prepare: func() *Collector {
				collr := New()
				collr.newConn = func(Config) rconConn { return prepareMockOk() }
				_ = collr.Check(context.Background())
				return collr
			},
		},
		"after collect": {
			prepare: func() *Collector {
				collr := New()
				collr.newConn = func(Config) rconConn { return prepareMockOk() }
				_ = collr.Collect(context.Background())
				return collr
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()

			assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockRcon
		wantFail    bool
	}{
		"success case": {
			wantFail:    false,
			prepareMock: prepareMockOk,
		},
		"err on connect": {
			wantFail:    true,
			prepareMock: prepareMockErrOnConnect,
		},
		"unexpected response": {
			wantFail:    true,
			prepareMock: prepareMockUnexpectedResponse,
		},
		"empty response": {
			wantFail:    true,
			prepareMock: prepareMockEmptyResponse,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			mock := test.prepareMock()
			collr.newConn = func(Config) rconConn { return mock }

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
		prepareMock             func() *mockRcon
		wantMetrics             map[string]int64
		disconnectBeforeCleanup bool
		disconnectAfterCleanup  bool
	}{
		"success case: clean": {
			prepareMock:             prepareMockOkClean,
			disconnectBeforeCleanup: false,
			disconnectAfterCleanup:  true,
			wantMetrics: map[string]int64{
				"mem_alloc": 411041792,
				"mem_max":   6299844608,
				"mem_used":  348127232,
				"players":   4,
				"tps_15min": 2000,
				"tps_1min":  2000,
				"tps_5min":  2000,
			},
		},
		"success case: double s": {
			prepareMock:             prepareMockOk,
			disconnectBeforeCleanup: false,
			disconnectAfterCleanup:  true,
			wantMetrics: map[string]int64{
				"mem_alloc": 411041792,
				"mem_max":   6299844608,
				"mem_used":  348127232,
				"players":   4,
				"tps_15min": 2000,
				"tps_1min":  2000,
				"tps_5min":  2000,
			},
		},
		"success case: double s with hidden": {
			prepareMock:             prepareMockOkSWithHidden,
			disconnectBeforeCleanup: false,
			disconnectAfterCleanup:  true,
			wantMetrics: map[string]int64{
				"mem_alloc": 411041792,
				"mem_max":   6299844608,
				"mem_used":  348127232,
				"players":   4,
				"tps_15min": 2000,
				"tps_1min":  2000,
				"tps_5min":  2000,
			},
		},
		"success case: non english": {
			prepareMock:             prepareMockOkSNonEng,
			disconnectBeforeCleanup: false,
			disconnectAfterCleanup:  true,
			wantMetrics: map[string]int64{
				"mem_alloc": 411041792,
				"mem_max":   6299844608,
				"mem_used":  348127232,
				"players":   4,
				"tps_15min": 2000,
				"tps_1min":  2000,
				"tps_5min":  2000,
			},
		},
		"unexpected response": {
			prepareMock:             prepareMockUnexpectedResponse,
			disconnectBeforeCleanup: true,
			disconnectAfterCleanup:  true,
		},
		"empty response": {
			prepareMock:             prepareMockEmptyResponse,
			disconnectBeforeCleanup: true,
			disconnectAfterCleanup:  true,
			wantMetrics:             nil,
		},
		"err on connect": {
			prepareMock:             prepareMockErrOnConnect,
			disconnectBeforeCleanup: false,
			disconnectAfterCleanup:  false,
		},
		"err on query status": {
			prepareMock:             prepareMockErrOnQuery,
			disconnectBeforeCleanup: true,
			disconnectAfterCleanup:  true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			mock := test.prepareMock()
			collr.newConn = func(Config) rconConn { return mock }

			mx := collr.Collect(context.Background())

			require.Equal(t, test.wantMetrics, mx, "want metrics")

			if len(test.wantMetrics) > 0 {
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
				assert.Equal(t, len(charts), len(*collr.Charts()), "want charts")
			}

			assert.Equal(t, test.disconnectBeforeCleanup, mock.disconnectCalled, "disconnect before cleanup")
			collr.Cleanup(context.Background())
			assert.Equal(t, test.disconnectAfterCleanup, mock.disconnectCalled, "disconnect after cleanup")
		})
	}
}

func prepareMockOkClean() *mockRcon {
	return &mockRcon{
		responseTps:  dataRespTsp,
		responseList: dataRespListClean,
	}
}

func prepareMockOk() *mockRcon {
	return &mockRcon{
		responseTps:  dataRespTsp,
		responseList: dataRespListDoubleS,
	}
}

func prepareMockOkSWithHidden() *mockRcon {
	return &mockRcon{
		responseTps:  dataRespTsp,
		responseList: dataRespListDoubleSWithHidden,
	}
}

func prepareMockOkSNonEng() *mockRcon {
	return &mockRcon{
		responseTps:  dataRespTsp,
		responseList: dataRespListDoubleSNonEng,
	}
}

func prepareMockErrOnConnect() *mockRcon {
	return &mockRcon{
		errOnConnect: true,
	}
}

func prepareMockErrOnQuery() *mockRcon {
	return &mockRcon{
		errOnQuery: true,
	}
}

func prepareMockUnexpectedResponse() *mockRcon {
	resp := "Lorem ipsum dolor sit amet, consectetur adipiscing elit."

	return &mockRcon{
		responseTps:  resp,
		responseList: resp,
	}
}

func prepareMockEmptyResponse() *mockRcon {
	return &mockRcon{
		responseTps:  "",
		responseList: "",
	}
}

type mockRcon struct {
	errOnConnect     bool
	responseTps      string
	responseList     string
	errOnQuery       bool
	disconnectCalled bool
}

func (m *mockRcon) connect() error {
	if m.errOnConnect {
		return errors.New("mock.connect() error")
	}
	return nil
}

func (m *mockRcon) disconnect() error {
	m.disconnectCalled = true
	return nil
}

func (m *mockRcon) queryTps() (string, error) {
	if m.errOnQuery {
		return "", errors.New("mock.queryTps() error")
	}
	return m.responseTps, nil
}

func (m *mockRcon) queryList() (string, error) {
	if m.errOnQuery {
		return "", errors.New("mock.queryList() error")
	}
	return m.responseList, nil
}
