// SPDX-License-Identifier: GPL-3.0-or-later

package apcupsd

import (
	"errors"
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

	dataStatus, _         = os.ReadFile("testdata/status.txt")
	dataStatusCommlost, _ = os.ReadFile("testdata/status_commlost.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":     dataConfigJSON,
		"dataConfigYAML":     dataConfigYAML,
		"dataStatus":         dataStatus,
		"dataStatusCommlost": dataStatusCommlost,
	} {
		require.NotNil(t, data, name)
	}
}

func TestApcupsd_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Apcupsd{}, dataConfigJSON, dataConfigYAML)
}

func TestApcupsd_Cleanup(t *testing.T) {
	apc := New()

	require.NotPanics(t, apc.Cleanup)

	mock := prepareMockOk()
	apc.newConn = func(Config) apcupsdConn { return mock }

	require.NoError(t, apc.Init())
	_ = apc.Collect()
	require.NotPanics(t, apc.Cleanup)
	assert.True(t, mock.calledDisconnect)
}

func TestApcupsd_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"success on default config": {
			wantFail: false,
			config:   New().Config,
		},
		"fails when 'address' option not set": {
			wantFail: true,
			config:   Config{Address: ""},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			apc := New()
			apc.Config = test.config

			if test.wantFail {
				assert.Error(t, apc.Init())
			} else {
				assert.NoError(t, apc.Init())
			}
		})
	}
}

func TestApcupsd_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockApcupsdConn
		wantFail    bool
	}{
		"case ok": {
			wantFail:    false,
			prepareMock: prepareMockOk,
		},
		"case commlost": {
			wantFail:    false,
			prepareMock: prepareMockOkCommlost,
		},
		"error on connect()": {
			wantFail:    true,
			prepareMock: prepareMockErrOnConnect,
		},
		"error on status()": {
			wantFail:    true,
			prepareMock: prepareMockErrOnStatus,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			apc := New()
			apc.newConn = func(Config) apcupsdConn { return test.prepareMock() }

			require.NoError(t, apc.Init())

			if test.wantFail {
				assert.Error(t, apc.Check())
			} else {
				assert.NoError(t, apc.Check())
			}
		})
	}
}

func TestApcupsd_Charts(t *testing.T) {
	apc := New()
	require.NoError(t, apc.Init())
	assert.NotNil(t, apc.Charts())
}

func TestApcupsd_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMock        func() *mockApcupsdConn
		wantCollected      map[string]int64
		wantCharts         int
		wantConnDisconnect bool
	}{
		"case ok": {
			prepareMock: prepareMockOk,
			wantCollected: map[string]int64{
				"battery_charge":                    10000,
				"battery_seconds_since_replacement": 86400,
				"battery_voltage":                   2790,
				"battery_voltage_nominal":           2400,
				"input_frequency":                   5000,
				"input_voltage":                     23530,
				"input_voltage_max":                 23920,
				"input_voltage_min":                 23400,
				"itemp":                             3279,
				"load":                              5500,
				"load_percent":                      930,
				"output_voltage":                    23660,
				"output_voltage_nominal":            23000,
				"selftest_BT":                       0,
				"selftest_IP":                       0,
				"selftest_NG":                       0,
				"selftest_NO":                       1,
				"selftest_OK":                       0,
				"selftest_UNK":                      0,
				"selftest_WN":                       0,
				"status_BOOST":                      0,
				"status_CAL":                        0,
				"status_COMMLOST":                   0,
				"status_LOWBATT":                    0,
				"status_NOBATT":                     0,
				"status_ONBATT":                     0,
				"status_ONLINE":                     1,
				"status_OVERLOAD":                   0,
				"status_REPLACEBATT":                0,
				"status_SHUTTING_DOWN":              0,
				"status_SLAVE":                      0,
				"status_SLAVEDOWN":                  0,
				"status_TRIM":                       0,
				"timeleft":                          780000,
			},
			wantConnDisconnect: false,
		},
		"case commlost": {
			prepareMock: prepareMockOkCommlost,
			wantCollected: map[string]int64{
				"status_BOOST":         0,
				"status_CAL":           0,
				"status_COMMLOST":      1,
				"status_LOWBATT":       0,
				"status_NOBATT":        0,
				"status_ONBATT":        0,
				"status_ONLINE":        0,
				"status_OVERLOAD":      0,
				"status_REPLACEBATT":   0,
				"status_SHUTTING_DOWN": 0,
				"status_SLAVE":         0,
				"status_SLAVEDOWN":     0,
				"status_TRIM":          0,
			},
			wantConnDisconnect: false,
		},
		"error on connect()": {
			prepareMock:        prepareMockErrOnConnect,
			wantCollected:      nil,
			wantConnDisconnect: false,
		},
		"error on status()": {
			prepareMock:        prepareMockErrOnStatus,
			wantCollected:      nil,
			wantConnDisconnect: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			apc := New()
			require.NoError(t, apc.Init())

			mock := test.prepareMock()
			apc.newConn = func(Config) apcupsdConn { return mock }

			mx := apc.Collect()

			if _, ok := mx["battery_seconds_since_replacement"]; ok {
				mx["battery_seconds_since_replacement"] = 86400
			}

			assert.Equal(t, test.wantCollected, mx)

			if len(test.wantCollected) > 0 {
				if strings.Contains(name, "commlost") {
					module.TestMetricsHasAllChartsDimsSkip(t, apc.Charts(), mx, func(chart *module.Chart, _ *module.Dim) bool {
						return chart.ID != statusChart.ID
					})
				} else {
					module.TestMetricsHasAllChartsDims(t, apc.Charts(), mx)
				}
			}

			assert.Equalf(t, test.wantConnDisconnect, mock.calledDisconnect, "calledDisconnect")
		})
	}
}

func prepareMockOk() *mockApcupsdConn {
	return &mockApcupsdConn{
		dataStatus: dataStatus,
	}
}

func prepareMockOkCommlost() *mockApcupsdConn {
	return &mockApcupsdConn{
		dataStatus: dataStatusCommlost,
	}
}

func prepareMockErrOnConnect() *mockApcupsdConn {
	return &mockApcupsdConn{errOnConnect: true}
}

func prepareMockErrOnStatus() *mockApcupsdConn {
	return &mockApcupsdConn{errOnStatus: true}
}

type mockApcupsdConn struct {
	errOnConnect     bool
	errOnStatus      bool
	calledDisconnect bool

	dataStatus []byte
}

func (m *mockApcupsdConn) connect() error {
	if m.errOnConnect {
		return errors.New("mock error on connect()")
	}
	return nil
}

func (m *mockApcupsdConn) disconnect() error {
	m.calledDisconnect = true
	return nil
}

func (m *mockApcupsdConn) status() ([]byte, error) {
	if m.errOnStatus {
		return nil, errors.New("mock error on status()")
	}

	return m.dataStatus, nil
}
