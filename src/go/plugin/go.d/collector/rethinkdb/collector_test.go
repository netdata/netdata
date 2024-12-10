// SPDX-License-Identifier: GPL-3.0-or-later

package rethinkdb

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
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
				collr.newConn = func(config Config) (rdbConn, error) {
					return &mockRethinkdbConn{dataStats: dataStats}, nil
				}
				_ = collr.Check(context.Background())
				return collr
			},
		},
		"after collect": {
			prepare: func() *Collector {
				collr := New()
				collr.newConn = func(config Config) (rdbConn, error) {
					return &mockRethinkdbConn{dataStats: dataStats}, nil
				}
				_ = collr.Check(context.Background())
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

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() *Collector
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
			collr := test.prepare()

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}

			if m, ok := collr.rdb.(*mockRethinkdbConn); ok {
				assert.False(t, m.disconnectCalled, "rdb close before cleanup")
				collr.Cleanup(context.Background())
				assert.True(t, m.disconnectCalled, "rdb close after cleanup")
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare     func() *Collector
		wantMetrics map[string]int64
		wantCharts  int
		skipChart   func(chart *module.Chart, dim *module.Dim) bool
	}{
		"success on valid response": {
			prepare:    prepareCaseOk,
			wantCharts: len(clusterCharts) + len(serverChartsTmpl)*3,
			skipChart: func(chart *module.Chart, dim *module.Dim) bool {
				return strings.HasPrefix(chart.ID, "server_0f74c641-af5f-48d6-a005-35b8983c576a") &&
					!strings.Contains(chart.ID, "stats_request_status")
			},
			wantMetrics: map[string]int64{
				"cluster_client_connections":            3,
				"cluster_clients_active":                3,
				"cluster_queries_total":                 27,
				"cluster_read_docs_total":               3,
				"cluster_servers_stats_request_success": 2,
				"cluster_servers_stats_request_timeout": 1,
				"cluster_written_docs_total":            3,
				"server_0f74c641-af5f-48d6-a005-35b8983c576a_stats_request_status_success": 0,
				"server_0f74c641-af5f-48d6-a005-35b8983c576a_stats_request_status_timeout": 1,
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
			collr := test.prepare()

			require.NoError(t, collr.Init(context.Background()))

			mx := collr.Collect(context.Background())

			require.Equal(t, test.wantMetrics, mx)

			assert.Equal(t, test.wantCharts, len(*collr.Charts()))

			if len(test.wantMetrics) > 0 {
				module.TestMetricsHasAllChartsDimsSkip(t, collr.Charts(), mx, test.skipChart)
			}

			if m, ok := collr.rdb.(*mockRethinkdbConn); ok {
				assert.False(t, m.disconnectCalled, "rdb close before cleanup")
				collr.Cleanup(context.Background())
				assert.True(t, m.disconnectCalled, "rdb close after cleanup")
			}
		})
	}
}

func prepareCaseOk() *Collector {
	collr := New()
	collr.newConn = func(cfg Config) (rdbConn, error) {
		return &mockRethinkdbConn{dataStats: dataStats}, nil
	}
	return collr
}

func prepareCaseErrOnStats() *Collector {
	collr := New()
	collr.newConn = func(cfg Config) (rdbConn, error) {
		return &mockRethinkdbConn{errOnStats: true}, nil
	}
	return collr
}

func prepareCaseErrOnConnect() *Collector {
	collr := New()
	collr.newConn = func(cfg Config) (rdbConn, error) {
		return nil, errors.New("mock failed to connect")
	}
	return collr
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
