// SPDX-License-Identifier: GPL-3.0-or-later

package w1sensor

import (
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataW1Slave, _ = os.ReadFile("testdata/w1_slave")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
		"dataW1Slave":    dataW1Slave,
	} {
		require.NotNil(t, data, name)
	}
}

func TestW1sensor_Configuration(t *testing.T) {
	module.TestConfigurationSerialize(t, &W1sensor{}, dataConfigJSON, dataConfigYAML)
}

func TestW1sensor_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"fails if 'sensors_path' is not set": {
			wantFail: true,
			config: Config{
				SensorsPath: "",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			pf := New()
			pf.Config = test.config

			if test.wantFail {
				assert.Error(t, pf.Init())
			} else {
				assert.NoError(t, pf.Init())
			}
		})
	}
}

func TestAP_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *W1sensor
	}{
		"not initialized exec": {
			prepare: func() *W1sensor {
				return New()
			},
		},
		"after check": {
			prepare: func() *W1sensor {
				w := New()
				w.SensorsPath = prepareMockOk().path
				_ = w.Check()
				return w
			},
		},
		"after collect": {
			prepare: func() *W1sensor {
				w := New()
				w.SensorsPath = prepareMockOk().path
				_ = w.Collect()
				return w
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			pf := test.prepare()

			assert.NotPanics(t, pf.Cleanup)
		})
	}
}

func TestW1sensor_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestW1sensor_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockSensor
		wantFail    bool
	}{
		"success case": {
			wantFail:    false,
			prepareMock: prepareMockOk,
		},
		"no sensors": {
			wantFail:    true,
			prepareMock: prepareMockNoSensors,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			w := New()
			w.SensorsPath = string(test.prepareMock().path)

			if test.wantFail {
				assert.Error(t, w.Check())
			} else {
				assert.NoError(t, w.Check())
			}
		})
	}
}

func TestW1Sensors_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockSensor
		wantMetrics map[string]int64
		wantCharts  int
	}{
		"success case": {
			prepareMock: prepareMockOk,
			wantCharts:  4,
			wantMetrics: map[string]int64{
				"w1sensor_temp_28-01204e9d2fa0": 12435,
				"w1sensor_temp_28-01204e9d2fa1": 29960,
				"w1sensor_temp_28-01204e9d2fa2": 10762,
				"w1sensor_temp_28-01204e9d2fa3": 22926,
			},
		},
		"no sensors": {
			prepareMock: prepareMockNoSensors,
			wantMetrics: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			w := New()
			w.SensorsPath = test.prepareMock().path

			mx := w.Collect()

			assert.Equal(t, test.wantMetrics, mx)

			assert.Equal(t, test.wantCharts, len(*w.Charts()), "wantCharts")

			module.TestMetricsHasAllChartsDims(t, w.Charts(), mx)
		})
	}
}

func prepareMockOk() *mockSensor {
	return &mockSensor{
		path: "/output",
	}
}

func prepareMockNoSensors() *mockSensor {
	return &mockSensor{
		path: "",
	}
}

type mockSensor struct {
	path string
}
