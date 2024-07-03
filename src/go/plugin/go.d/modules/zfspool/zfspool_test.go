// SPDX-License-Identifier: GPL-3.0-or-later

package zfspool

import (
	"errors"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataZpoolList, _ = os.ReadFile("testdata/zpool-list.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,

		"dataZpoolList": dataZpoolList,
	} {
		require.NotNil(t, data, name)

	}
}

func TestZFSPool_Configuration(t *testing.T) {
	module.TestConfigurationSerialize(t, &ZFSPool{}, dataConfigJSON, dataConfigYAML)
}

func TestZFSPool_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"fails if 'binary_path' is not set": {
			wantFail: true,
			config: Config{
				BinaryPath: "",
			},
		},
		"fails if failed to find binary": {
			wantFail: true,
			config: Config{
				BinaryPath: "zpool!!!",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			zp := New()
			zp.Config = test.config

			if test.wantFail {
				assert.Error(t, zp.Init())
			} else {
				assert.NoError(t, zp.Init())
			}
		})
	}
}

func TestZFSPool_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *ZFSPool
	}{
		"not initialized exec": {
			prepare: func() *ZFSPool {
				return New()
			},
		},
		"after check": {
			prepare: func() *ZFSPool {
				zp := New()
				zp.exec = prepareMockOK()
				_ = zp.Check()
				return zp
			},
		},
		"after collect": {
			prepare: func() *ZFSPool {
				zp := New()
				zp.exec = prepareMockOK()
				_ = zp.Collect()
				return zp
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			zp := test.prepare()

			assert.NotPanics(t, zp.Cleanup)
		})
	}
}

func TestZFSPool_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestZFSPool_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockZpoolCLIExec
		wantFail    bool
	}{
		"success case": {
			prepareMock: prepareMockOK,
			wantFail:    false,
		},
		"error on list call": {
			prepareMock: prepareMockErrOnList,
			wantFail:    true,
		},
		"empty response": {
			prepareMock: prepareMockEmptyResponse,
			wantFail:    true,
		},
		"unexpected response": {
			prepareMock: prepareMockUnexpectedResponse,
			wantFail:    true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			zp := New()
			mock := test.prepareMock()
			zp.exec = mock

			if test.wantFail {
				assert.Error(t, zp.Check())
			} else {
				assert.NoError(t, zp.Check())
			}
		})
	}
}

func TestZFSPool_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockZpoolCLIExec
		wantMetrics map[string]int64
	}{
		"success case": {
			prepareMock: prepareMockOK,
			wantMetrics: map[string]int64{
				"zpool_rpool_alloc":                  9051643576,
				"zpool_rpool_cap":                    42,
				"zpool_rpool_frag":                   33,
				"zpool_rpool_free":                   12240656794,
				"zpool_rpool_health_state_degraded":  0,
				"zpool_rpool_health_state_faulted":   0,
				"zpool_rpool_health_state_offline":   0,
				"zpool_rpool_health_state_online":    1,
				"zpool_rpool_health_state_removed":   0,
				"zpool_rpool_health_state_suspended": 0,
				"zpool_rpool_health_state_unavail":   0,
				"zpool_rpool_size":                   21367462298,
				"zpool_zion_health_state_degraded":   0,
				"zpool_zion_health_state_faulted":    1,
				"zpool_zion_health_state_offline":    0,
				"zpool_zion_health_state_online":     0,
				"zpool_zion_health_state_removed":    0,
				"zpool_zion_health_state_suspended":  0,
				"zpool_zion_health_state_unavail":    0,
			},
		},
		"error on list call": {
			prepareMock: prepareMockErrOnList,
			wantMetrics: nil,
		},
		"empty response": {
			prepareMock: prepareMockEmptyResponse,
			wantMetrics: nil,
		},
		"unexpected response": {
			prepareMock: prepareMockUnexpectedResponse,
			wantMetrics: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			zp := New()
			mock := test.prepareMock()
			zp.exec = mock

			mx := zp.Collect()

			assert.Equal(t, test.wantMetrics, mx)
			if len(test.wantMetrics) > 0 {
				assert.Len(t, *zp.Charts(), len(zpoolChartsTmpl)*len(zp.zpools))
			}
		})
	}
}

func prepareMockOK() *mockZpoolCLIExec {
	return &mockZpoolCLIExec{
		listData: dataZpoolList,
	}
}

func prepareMockErrOnList() *mockZpoolCLIExec {
	return &mockZpoolCLIExec{
		errOnList: true,
	}
}

func prepareMockEmptyResponse() *mockZpoolCLIExec {
	return &mockZpoolCLIExec{}
}

func prepareMockUnexpectedResponse() *mockZpoolCLIExec {
	return &mockZpoolCLIExec{
		listData: []byte(`
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Nulla malesuada erat id magna mattis, eu viverra tellus rhoncus.
Fusce et felis pulvinar, posuere sem non, porttitor eros.
`),
	}
}

type mockZpoolCLIExec struct {
	errOnList bool
	listData  []byte
}

func (m *mockZpoolCLIExec) list() ([]byte, error) {
	if m.errOnList {
		return nil, errors.New("mock.list() error")
	}

	return m.listData, nil
}
