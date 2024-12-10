// SPDX-License-Identifier: GPL-3.0-or-later

package memcached

import (
	"context"
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

	dataMemcachedStats, _ = os.ReadFile("testdata/stats.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,

		"dataMemcachedStats": dataMemcachedStats,
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
		"success with default config": {
			wantFail: false,
			config:   New().Config,
		},
		"fails if address not set": {
			wantFail: true,
			config: func() Config {
				conf := New().Config
				conf.Address = ""
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
				collr.newMemcachedConn = func(config Config) memcachedConn { return prepareMockOk() }
				_ = collr.Check(context.Background())
				return collr
			},
		},
		"after collect": {
			prepare: func() *Collector {
				collr := New()
				collr.newMemcachedConn = func(config Config) memcachedConn { return prepareMockOk() }
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
		prepareMock func() *mockMemcachedConn
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
			collr.newMemcachedConn = func(config Config) memcachedConn { return mock }

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
		prepareMock             func() *mockMemcachedConn
		wantMetrics             map[string]int64
		disconnectBeforeCleanup bool
		disconnectAfterCleanup  bool
	}{
		"success case": {
			prepareMock:             prepareMockOk,
			disconnectBeforeCleanup: false,
			disconnectAfterCleanup:  true,
			wantMetrics: map[string]int64{
				"avail":                67108831,
				"bytes":                33,
				"bytes_read":           108662,
				"bytes_written":        9761348,
				"cas_badval":           0,
				"cas_hits":             0,
				"cas_misses":           0,
				"cmd_get":              1,
				"cmd_set":              1,
				"cmd_touch":            0,
				"curr_connections":     3,
				"curr_items":           0,
				"decr_hits":            0,
				"decr_misses":          0,
				"delete_hits":          0,
				"delete_misses":        0,
				"evictions":            0,
				"get_hits":             0,
				"get_misses":           1,
				"incr_hits":            0,
				"incr_misses":          0,
				"limit_maxbytes":       67108864,
				"reclaimed":            1,
				"rejected_connections": 0,
				"total_connections":    39,
				"total_items":          1,
				"touch_hits":           0,
				"touch_misses":         0,
			},
		},
		"error response": {
			prepareMock:             prepareMockErrorResponse,
			disconnectBeforeCleanup: false,
			disconnectAfterCleanup:  true,
		},
		"unexpected response": {
			prepareMock:             prepareMockUnexpectedResponse,
			disconnectBeforeCleanup: false,
			disconnectAfterCleanup:  true,
		},
		"empty response": {
			prepareMock:             prepareMockEmptyResponse,
			disconnectBeforeCleanup: false,
			disconnectAfterCleanup:  true,
		},
		"err on connect": {
			prepareMock:             prepareMockErrOnConnect,
			disconnectBeforeCleanup: false,
			disconnectAfterCleanup:  false,
		},
		"err on query stats": {
			prepareMock:             prepareMockErrOnQueryStats,
			disconnectBeforeCleanup: true,
			disconnectAfterCleanup:  true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			mock := test.prepareMock()
			collr.newMemcachedConn = func(config Config) memcachedConn { return mock }

			mx := collr.Collect(context.Background())

			require.Equal(t, test.wantMetrics, mx)

			if len(test.wantMetrics) > 0 {
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}

			assert.Equal(t, test.disconnectBeforeCleanup, mock.disconnectCalled, "disconnect before cleanup")
			collr.Cleanup(context.Background())
			assert.Equal(t, test.disconnectAfterCleanup, mock.disconnectCalled, "disconnect after cleanup")
		})
	}
}

func prepareMockOk() *mockMemcachedConn {
	return &mockMemcachedConn{
		statsResponse: dataMemcachedStats,
	}
}

func prepareMockErrorResponse() *mockMemcachedConn {
	return &mockMemcachedConn{
		statsResponse: []byte("ERROR"),
	}
}

func prepareMockErrOnConnect() *mockMemcachedConn {
	return &mockMemcachedConn{
		errOnConnect: true,
	}
}

func prepareMockErrOnQueryStats() *mockMemcachedConn {
	return &mockMemcachedConn{
		errOnQueryStats: true,
	}
}

func prepareMockUnexpectedResponse() *mockMemcachedConn {
	return &mockMemcachedConn{
		statsResponse: []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit."),
	}
}

func prepareMockEmptyResponse() *mockMemcachedConn {
	return &mockMemcachedConn{}
}

type mockMemcachedConn struct {
	errOnConnect     bool
	errOnQueryStats  bool
	statsResponse    []byte
	disconnectCalled bool
}

func (m *mockMemcachedConn) connect() error {
	if m.errOnConnect {
		return errors.New("mock.connect() error")
	}
	return nil
}

func (m *mockMemcachedConn) disconnect() {
	m.disconnectCalled = true
}

func (m *mockMemcachedConn) queryStats() ([]byte, error) {
	if m.errOnQueryStats {
		return nil, errors.New("mock.queryStats() error")
	}
	return m.statsResponse, nil
}
