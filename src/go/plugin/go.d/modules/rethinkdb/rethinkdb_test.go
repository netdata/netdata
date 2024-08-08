// SPDX-License-Identifier: GPL-3.0-or-later

package rethinkdb

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataStats, _ = os.ReadFile("testdata/v2.4.4/stats.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,

		"dataStats": dataStats,
	} {
		require.NotNil(t, data, name)
	}
}

func TestRethinkdb_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Rethinkdb{}, dataConfigJSON, dataConfigYAML)
}

func TestRethinkdb_Init(t *testing.T) {
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
			rdb := New()
			rdb.Config = test.config

			if test.wantFail {
				assert.Error(t, rdb.Init())
			} else {
				assert.NoError(t, rdb.Init())
			}
		})
	}
}

func TestRethinkdb_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *Rethinkdb
	}{
		"not initialized": {
			prepare: func() *Rethinkdb {
				return New()
			},
		},
		"after check": {
			prepare: func() *Rethinkdb {
				rdb := New()
				rdb.newConn = func(config Config) (rdbConn, error) {
					return &mockRethinkdbConn{dataStats: dataStats}, nil
				}
				_ = rdb.Check()
				return rdb
			},
		},
		"after collect": {
			prepare: func() *Rethinkdb {
				rdb := New()
				rdb.newConn = func(config Config) (rdbConn, error) {
					return &mockRethinkdbConn{dataStats: dataStats}, nil
				}
				_ = rdb.Check()
				return rdb
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			rdb := test.prepare()

			assert.NotPanics(t, rdb.Cleanup)
		})
	}
}

func TestRethinkdb_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() *Rethinkdb
		wantFail bool
	}{
		"success on valid response": {
			wantFail: false,
			prepare:  prepareCaseOk,
		},
		"fails if error on stats": {
			wantFail: true,
			prepare:  prepareCaseErrOnStats,
		},
		"fails if error on connect": {
			wantFail: true,
			prepare:  prepareCaseErrOnConnect,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			rdb := test.prepare()

			if test.wantFail {
				assert.Error(t, rdb.Check())
			} else {
				assert.NoError(t, rdb.Check())
			}

			if m, ok := rdb.rdb.(*mockRethinkdbConn); ok {
				assert.False(t, m.disconnectCalled, "rdb close before cleanup")
				rdb.Cleanup()
				assert.True(t, m.disconnectCalled, "rdb close after cleanup")
			}
		})
	}
}

func TestRethinkdb_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare     func() *Rethinkdb
		wantMetrics map[string]int64
		wantCharts  int
	}{
		"success on valid response": {
			prepare:    prepareCaseOk,
			wantCharts: len(clusterCharts) + len(serverChartsTmpl)*2,
			wantMetrics: map[string]int64{
				"cluster_client_connections":                                               3,
				"cluster_clients_active":                                                   3,
				"cluster_queries_total":                                                    27,
				"cluster_read_docs_total":                                                  3,
				"cluster_servers_stats_request_success":                                    2,
				"cluster_servers_stats_request_timeout":                                    0,
				"cluster_written_docs_total":                                               3,
				"server_b7730db2-4303-4719-aef8-2a3c339c672b_client_connections":           1,
				"server_b7730db2-4303-4719-aef8-2a3c339c672b_clients_active":               1,
				"server_b7730db2-4303-4719-aef8-2a3c339c672b_queries_total":                13,
				"server_b7730db2-4303-4719-aef8-2a3c339c672b_read_docs_total":              1,
				"server_b7730db2-4303-4719-aef8-2a3c339c672b_stats_request_status_success": 1,
				"server_b7730db2-4303-4719-aef8-2a3c339c672b_stats_request_status_timeout": 0,
				"server_b7730db2-4303-4719-aef8-2a3c339c672b_written_docs_total":           1,
				"server_f325e3c3-22d9-4005-b4b2-1f561d384edc_client_connections":           2,
				"server_f325e3c3-22d9-4005-b4b2-1f561d384edc_clients_active":               2,
				"server_f325e3c3-22d9-4005-b4b2-1f561d384edc_queries_total":                14,
				"server_f325e3c3-22d9-4005-b4b2-1f561d384edc_read_docs_total":              2,
				"server_f325e3c3-22d9-4005-b4b2-1f561d384edc_stats_request_status_success": 1,
				"server_f325e3c3-22d9-4005-b4b2-1f561d384edc_stats_request_status_timeout": 0,
				"server_f325e3c3-22d9-4005-b4b2-1f561d384edc_written_docs_total":           2,
			},
		},
		"fails if error on stats": {
			wantCharts: len(clusterCharts),
			prepare:    prepareCaseErrOnStats,
		},
		"fails if error on connect": {
			wantCharts: len(clusterCharts),
			prepare:    prepareCaseErrOnStats,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			rdb := test.prepare()

			require.NoError(t, rdb.Init())

			mx := rdb.Collect()

			require.Equal(t, test.wantMetrics, mx)

			assert.Equal(t, test.wantCharts, len(*rdb.Charts()))

			if len(test.wantMetrics) > 0 {
				module.TestMetricsHasAllChartsDims(t, rdb.Charts(), mx)
			}

			if m, ok := rdb.rdb.(*mockRethinkdbConn); ok {
				assert.False(t, m.disconnectCalled, "rdb close before cleanup")
				rdb.Cleanup()
				assert.True(t, m.disconnectCalled, "rdb close after cleanup")
			}
		})
	}
}

func prepareCaseOk() *Rethinkdb {
	rdb := New()
	rdb.newConn = func(cfg Config) (rdbConn, error) {
		return &mockRethinkdbConn{dataStats: dataStats}, nil
	}
	return rdb
}

func prepareCaseErrOnStats() *Rethinkdb {
	rdb := New()
	rdb.newConn = func(cfg Config) (rdbConn, error) {
		return &mockRethinkdbConn{errOnStats: true}, nil
	}
	return rdb
}

func prepareCaseErrOnConnect() *Rethinkdb {
	rdb := New()
	rdb.newConn = func(cfg Config) (rdbConn, error) {
		return nil, errors.New("mock failed to connect")
	}
	return rdb
}

type mockRethinkdbConn struct {
	dataStats        []byte
	errOnStats       bool
	disconnectCalled bool
}

func (m *mockRethinkdbConn) stats() ([][]byte, error) {
	if m.errOnStats {
		return nil, fmt.Errorf("mock.stats() error")
	}
	return bytes.Split(bytes.TrimSpace(m.dataStats), []byte("\n")), nil
}

func (m *mockRethinkdbConn) close() error {
	m.disconnectCalled = true
	return nil
}
