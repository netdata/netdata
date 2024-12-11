// SPDX-License-Identifier: GPL-3.0-or-later

package gearman

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

	dataStatus, _         = os.ReadFile("testdata/status.txt")
	dataPriorityStatus, _ = os.ReadFile("testdata/priority-status.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,

		"dataStatus":         dataStatus,
		"dataPriorityStatus": dataPriorityStatus,
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
				collr.newConn = func(Config) gearmanConn { return prepareMockOk() }
				_ = collr.Check(context.Background())
				return collr
			},
		},
		"after collect": {
			prepare: func() *Collector {
				collr := New()
				collr.newConn = func(Config) gearmanConn { return prepareMockOk() }
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
		prepareMock func() *mockGearmanConn
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
			wantFail:    false,
			prepareMock: prepareMockEmptyResponse,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			mock := test.prepareMock()
			collr.newConn = func(Config) gearmanConn { return mock }

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
		prepareMock             func() *mockGearmanConn
		wantMetrics             map[string]int64
		wantCharts              int
		disconnectBeforeCleanup bool
		disconnectAfterCleanup  bool
	}{
		"success case": {
			prepareMock:             prepareMockOk,
			disconnectBeforeCleanup: false,
			disconnectAfterCleanup:  true,
			wantCharts:              len(summaryCharts) + len(functionStatusChartsTmpl)*4 + len(functionPriorityStatusChartsTmpl)*4,
			wantMetrics: map[string]int64{
				"function_generic_worker1_high_priority_jobs":          10,
				"function_generic_worker1_jobs_queued":                 4,
				"function_generic_worker1_jobs_running":                3,
				"function_generic_worker1_jobs_waiting":                1,
				"function_generic_worker1_low_priority_jobs":           12,
				"function_generic_worker1_normal_priority_jobs":        11,
				"function_generic_worker1_workers_available":           500,
				"function_generic_worker2_high_priority_jobs":          4,
				"function_generic_worker2_jobs_queued":                 78,
				"function_generic_worker2_jobs_running":                78,
				"function_generic_worker2_jobs_waiting":                0,
				"function_generic_worker2_low_priority_jobs":           6,
				"function_generic_worker2_normal_priority_jobs":        5,
				"function_generic_worker2_workers_available":           500,
				"function_generic_worker3_high_priority_jobs":          7,
				"function_generic_worker3_jobs_queued":                 2,
				"function_generic_worker3_jobs_running":                1,
				"function_generic_worker3_jobs_waiting":                1,
				"function_generic_worker3_low_priority_jobs":           9,
				"function_generic_worker3_normal_priority_jobs":        8,
				"function_generic_worker3_workers_available":           760,
				"function_prefix_generic_worker4_high_priority_jobs":   1,
				"function_prefix_generic_worker4_jobs_queued":          78,
				"function_prefix_generic_worker4_jobs_running":         78,
				"function_prefix_generic_worker4_jobs_waiting":         0,
				"function_prefix_generic_worker4_low_priority_jobs":    3,
				"function_prefix_generic_worker4_normal_priority_jobs": 2,
				"function_prefix_generic_worker4_workers_available":    500,
				"total_high_priority_jobs":                             22,
				"total_jobs_queued":                                    162,
				"total_jobs_running":                                   160,
				"total_jobs_waiting":                                   2,
				"total_low_priority_jobs":                              30,
				"total_normal_priority_jobs":                           26,
				"total_workers_avail":                                  0,
				"total_workers_available":                              2260,
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
			wantCharts:              len(summaryCharts),
			wantMetrics: map[string]int64{
				"total_high_priority_jobs":   0,
				"total_jobs_queued":          0,
				"total_jobs_running":         0,
				"total_jobs_waiting":         0,
				"total_low_priority_jobs":    0,
				"total_normal_priority_jobs": 0,
				"total_workers_avail":        0,
			},
		},
		"err on connect": {
			prepareMock:             prepareMockErrOnConnect,
			disconnectBeforeCleanup: false,
			disconnectAfterCleanup:  false,
		},
		"err on query status": {
			prepareMock:             prepareMockErrOnQueryStatus,
			disconnectBeforeCleanup: true,
			disconnectAfterCleanup:  true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			mock := test.prepareMock()
			collr.newConn = func(Config) gearmanConn { return mock }

			mx := collr.Collect(context.Background())

			require.Equal(t, test.wantMetrics, mx, "want metrics")

			if len(test.wantMetrics) > 0 {
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
				assert.Equal(t, test.wantCharts, len(*collr.Charts()), "want charts")
			}

			assert.Equal(t, test.disconnectBeforeCleanup, mock.disconnectCalled, "disconnect before cleanup")
			collr.Cleanup(context.Background())
			assert.Equal(t, test.disconnectAfterCleanup, mock.disconnectCalled, "disconnect after cleanup")
		})
	}
}

func prepareMockOk() *mockGearmanConn {
	return &mockGearmanConn{
		responseStatus:         dataStatus,
		responsePriorityStatus: dataPriorityStatus,
	}
}

func prepareMockErrOnConnect() *mockGearmanConn {
	return &mockGearmanConn{
		errOnConnect: true,
	}
}

func prepareMockErrOnQueryStatus() *mockGearmanConn {
	return &mockGearmanConn{
		errOnQueryStatus: true,
	}
}

func prepareMockUnexpectedResponse() *mockGearmanConn {
	resp := []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit.")
	return &mockGearmanConn{
		responseStatus:         resp,
		responsePriorityStatus: resp,
	}
}

func prepareMockEmptyResponse() *mockGearmanConn {
	return &mockGearmanConn{
		responseStatus:         []byte("."),
		responsePriorityStatus: []byte("."),
	}
}

type mockGearmanConn struct {
	errOnConnect bool

	responseStatus   []byte
	errOnQueryStatus bool

	responsePriorityStatus   []byte
	errOnQueryPriorityStatus bool

	disconnectCalled bool
}

func (m *mockGearmanConn) connect() error {
	if m.errOnConnect {
		return errors.New("mock.connect() error")
	}
	return nil
}

func (m *mockGearmanConn) disconnect() {
	m.disconnectCalled = true
}

func (m *mockGearmanConn) queryStatus() ([]byte, error) {
	if m.errOnQueryStatus {
		return nil, errors.New("mock.queryStatus() error")
	}
	return m.responseStatus, nil
}

func (m *mockGearmanConn) queryPriorityStatus() ([]byte, error) {
	if m.errOnQueryPriorityStatus {
		return nil, errors.New("mock.queryPriorityStatus() error")
	}
	return m.responsePriorityStatus, nil
}
