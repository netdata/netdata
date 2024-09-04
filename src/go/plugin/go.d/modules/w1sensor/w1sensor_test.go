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
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
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
			w1 := New()
			w1.Config = test.config

			if test.wantFail {
				assert.Error(t, w1.Init())
			} else {
				assert.NoError(t, w1.Init())
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
				w1 := prepareCaseOk()
				_ = w1.Check()
				return w1
			},
		},
		"after collect": {
			prepare: func() *W1sensor {
				w1 := prepareCaseOk()
				_ = w1.Collect()
				return w1
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			w1 := test.prepare()

			assert.NotPanics(t, w1.Cleanup)
		})
	}
}

func TestW1sensor_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestW1sensor_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *W1sensor
		wantFail    bool
	}{
		"success case": {
			wantFail:    false,
			prepareMock: prepareCaseOk,
		},
		"no sensors dir": {
			wantFail:    true,
			prepareMock: prepareCaseNoSensorsDir,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			w1 := test.prepareMock()

			if test.wantFail {
				assert.Error(t, w1.Check())
			} else {
				assert.NoError(t, w1.Check())
			}
		})
	}
}

func TestW1Sensors_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *W1sensor
		wantMetrics map[string]int64
		wantCharts  int
	}{
		"success case": {
			prepareMock: prepareCaseOk,
			wantCharts:  4,
			wantMetrics: map[string]int64{
				"w1sensor_28-01204e9d2fa0_temperature": 124,
				"w1sensor_28-01204e9d2fa1_temperature": 299,
				"w1sensor_28-01204e9d2fa2_temperature": 107,
				"w1sensor_28-01204e9d2fa3_temperature": 229,
			},
		},
		"no sensors dir": {
			prepareMock: prepareCaseNoSensorsDir,
			wantMetrics: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			w1 := test.prepareMock()

			mx := w1.Collect()

			assert.Equal(t, test.wantMetrics, mx)

			assert.Equal(t, test.wantCharts, len(*w1.Charts()), "wantCharts")

			module.TestMetricsHasAllChartsDims(t, w1.Charts(), mx)
		})
	}
}

func prepareCaseOk() *W1sensor {
	w1 := New()
	w1.SensorsPath = "testdata/devices"
	return w1
}

func prepareCaseNoSensorsDir() *W1sensor {
	w1 := New()
	w1.SensorsPath = "testdata/devices!"
	return w1
}
