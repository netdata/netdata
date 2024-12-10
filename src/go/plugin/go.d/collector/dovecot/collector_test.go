// SPDX-License-Identifier: GPL-3.0-or-later

package dovecot

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

	dataExportGlobal, _ = os.ReadFile("testdata/export_global.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":   dataConfigJSON,
		"dataConfigYAML":   dataConfigYAML,
		"dataExportGlobal": dataExportGlobal,
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
				collr.newConn = func(Config) dovecotConn { return prepareMockOk() }
				_ = collr.Check(context.Background())
				return collr
			},
		},
		"after collect": {
			prepare: func() *Collector {
				collr := New()
				collr.newConn = func(Config) dovecotConn { return prepareMockOk() }
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
		prepareMock func() *mockDovecotConn
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
			collr.newConn = func(Config) dovecotConn { return mock }

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
		prepareMock             func() *mockDovecotConn
		wantMetrics             map[string]int64
		disconnectBeforeCleanup bool
		disconnectAfterCleanup  bool
	}{
		"success case": {
			prepareMock:             prepareMockOk,
			disconnectBeforeCleanup: false,
			disconnectAfterCleanup:  true,
			wantMetrics: map[string]int64{
				"auth_cache_hits":        1,
				"auth_cache_misses":      1,
				"auth_db_tempfails":      1,
				"auth_failures":          1,
				"auth_master_successes":  1,
				"auth_successes":         1,
				"disk_input":             1,
				"disk_output":            1,
				"invol_cs":               1,
				"mail_cache_hits":        1,
				"mail_lookup_attr":       1,
				"mail_lookup_path":       1,
				"mail_read_bytes":        1,
				"mail_read_count":        1,
				"maj_faults":             1,
				"min_faults":             1,
				"num_cmds":               1,
				"num_connected_sessions": 1,
				"num_logins":             1,
				"read_bytes":             1,
				"read_count":             1,
				"reset_timestamp":        1723481629,
				"vol_cs":                 1,
				"write_bytes":            1,
				"write_count":            1,
			},
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
			prepareMock:             prepareMockErrOnQueryExportGlobal,
			disconnectBeforeCleanup: true,
			disconnectAfterCleanup:  true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			mock := test.prepareMock()
			collr.newConn = func(Config) dovecotConn { return mock }

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

func prepareMockOk() *mockDovecotConn {
	return &mockDovecotConn{
		exportGlobalResponse: dataExportGlobal,
	}
}

func prepareMockErrOnConnect() *mockDovecotConn {
	return &mockDovecotConn{
		errOnConnect: true,
	}
}

func prepareMockErrOnQueryExportGlobal() *mockDovecotConn {
	return &mockDovecotConn{
		errOnQueryExportGlobal: true,
	}
}

func prepareMockUnexpectedResponse() *mockDovecotConn {
	return &mockDovecotConn{
		exportGlobalResponse: []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit."),
	}
}

func prepareMockEmptyResponse() *mockDovecotConn {
	return &mockDovecotConn{}
}

type mockDovecotConn struct {
	errOnConnect           bool
	errOnQueryExportGlobal bool
	exportGlobalResponse   []byte
	disconnectCalled       bool
}

func (m *mockDovecotConn) connect() error {
	if m.errOnConnect {
		return errors.New("mock.connect() error")
	}
	return nil
}

func (m *mockDovecotConn) disconnect() {
	m.disconnectCalled = true
}

func (m *mockDovecotConn) queryExportGlobal() ([]byte, error) {
	if m.errOnQueryExportGlobal {
		return nil, errors.New("mock.queryExportGlobal() error")
	}
	return m.exportGlobalResponse, nil
}
