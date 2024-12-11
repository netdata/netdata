// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package w1sensor

import (
	"context"
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

func TestCollector_Configuration(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
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

func TestAP_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *Collector
	}{
		"not initialized exec": {
			prepare: func() *Collector {
				return New()
			},
		},
		"after check": {
			prepare: func() *Collector {
				collr := prepareCaseOk()
				_ = collr.Check(context.Background())
				return collr
			},
		},
		"after collect": {
			prepare: func() *Collector {
				collr := prepareCaseOk()
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
		prepareMock func() *Collector
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
			collr := test.prepareMock()

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestW1Sensors_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *Collector
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
			collr := test.prepareMock()

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantMetrics, mx)

			assert.Equal(t, test.wantCharts, len(*collr.Charts()), "wantCharts")

			module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
		})
	}
}

func prepareCaseOk() *Collector {
	collr := New()
	collr.SensorsPath = "testdata/devices"
	return collr
}

func prepareCaseNoSensorsDir() *Collector {
	collr := New()
	collr.SensorsPath = "testdata/devices!"
	return collr
}
