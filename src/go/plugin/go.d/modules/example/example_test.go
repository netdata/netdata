// SPDX-License-Identifier: GPL-3.0-or-later

package example

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

func TestExample_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Example{}, dataConfigJSON, dataConfigYAML)
}

func TestNew(t *testing.T) {
	// We want to ensure that module is a reference type, nothing more.

	assert.IsType(t, (*Example)(nil), New())
}

func TestExample_Init(t *testing.T) {
	// 'Init() bool' initializes the module with an appropriate config, so to test it we need:
	// - provide the config.
	// - set module.Config field with the config.
	// - call Init() and compare its return value with the expected value.

	// 'test' map contains different test cases.
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"success on default config": {
			config: New().Config,
		},
		"success when only 'charts' set": {
			config: Config{
				Charts: ConfigCharts{
					Num:  1,
					Dims: 2,
				},
			},
		},
		"success when only 'hidden_charts' set": {
			config: Config{
				HiddenCharts: ConfigCharts{
					Num:  1,
					Dims: 2,
				},
			},
		},
		"success when 'charts' and 'hidden_charts' set": {
			config: Config{
				Charts: ConfigCharts{
					Num:  1,
					Dims: 2,
				},
				HiddenCharts: ConfigCharts{
					Num:  1,
					Dims: 2,
				},
			},
		},
		"fails when 'charts' and 'hidden_charts' set, but 'num' == 0": {
			wantFail: true,
			config: Config{
				Charts: ConfigCharts{
					Num:  0,
					Dims: 2,
				},
				HiddenCharts: ConfigCharts{
					Num:  0,
					Dims: 2,
				},
			},
		},
		"fails when only 'charts' set, 'num' > 0, but 'dimensions' == 0": {
			wantFail: true,
			config: Config{
				Charts: ConfigCharts{
					Num:  1,
					Dims: 0,
				},
			},
		},
		"fails when only 'hidden_charts' set, 'num' > 0, but 'dimensions' == 0": {
			wantFail: true,
			config: Config{
				HiddenCharts: ConfigCharts{
					Num:  1,
					Dims: 0,
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			example := New()
			example.Config = test.config

			if test.wantFail {
				assert.Error(t, example.Init())
			} else {
				assert.NoError(t, example.Init())
			}
		})
	}
}

func TestExample_Check(t *testing.T) {
	// 'Check() bool' reports whether the module is able to collect any data, so to test it we need:
	// - provide the module with a specific config.
	// - initialize the module (call Init()).
	// - call Check() and compare its return value with the expected value.

	// 'test' map contains different test cases.
	tests := map[string]struct {
		prepare  func() *Example
		wantFail bool
	}{
		"success on default":                            {prepare: prepareExampleDefault},
		"success when only 'charts' set":                {prepare: prepareExampleOnlyCharts},
		"success when only 'hidden_charts' set":         {prepare: prepareExampleOnlyHiddenCharts},
		"success when 'charts' and 'hidden_charts' set": {prepare: prepareExampleChartsAndHiddenCharts},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			example := test.prepare()
			require.NoError(t, example.Init())

			if test.wantFail {
				assert.Error(t, example.Check())
			} else {
				assert.NoError(t, example.Check())
			}
		})
	}
}

func TestExample_Charts(t *testing.T) {
	// We want to ensure that initialized module does not return 'nil'.
	// If it is not 'nil' we are ok.

	// 'test' map contains different test cases.
	tests := map[string]struct {
		prepare func(t *testing.T) *Example
		wantNil bool
	}{
		"not initialized collector": {
			wantNil: true,
			prepare: func(t *testing.T) *Example {
				return New()
			},
		},
		"initialized collector": {
			prepare: func(t *testing.T) *Example {
				example := New()
				require.NoError(t, example.Init())
				return example
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			example := test.prepare(t)

			if test.wantNil {
				assert.Nil(t, example.Charts())
			} else {
				assert.NotNil(t, example.Charts())
			}
		})
	}
}

func TestExample_Cleanup(t *testing.T) {
	// Since this module has nothing to clean up,
	// we want just to ensure that Cleanup() not panics.

	assert.NotPanics(t, New().Cleanup)
}

func TestExample_Collect(t *testing.T) {
	// 'Collect() map[string]int64' returns collected data, so to test it we need:
	// - provide the module with a specific config.
	// - initialize the module (call Init()).
	// - call Collect() and compare its return value with the expected value.

	// 'test' map contains different test cases.
	tests := map[string]struct {
		prepare       func() *Example
		wantCollected map[string]int64
	}{
		"default config": {
			prepare: prepareExampleDefault,
			wantCollected: map[string]int64{
				"random_0_random0": 1,
				"random_0_random1": -1,
				"random_0_random2": 1,
				"random_0_random3": -1,
			},
		},
		"only 'charts' set": {
			prepare: prepareExampleOnlyCharts,
			wantCollected: map[string]int64{
				"random_0_random0": 1,
				"random_0_random1": -1,
				"random_0_random2": 1,
				"random_0_random3": -1,
				"random_0_random4": 1,
				"random_1_random0": 1,
				"random_1_random1": -1,
				"random_1_random2": 1,
				"random_1_random3": -1,
				"random_1_random4": 1,
			},
		},
		"only 'hidden_charts' set": {
			prepare: prepareExampleOnlyHiddenCharts,
			wantCollected: map[string]int64{
				"hidden_random_0_random0": 1,
				"hidden_random_0_random1": -1,
				"hidden_random_0_random2": 1,
				"hidden_random_0_random3": -1,
				"hidden_random_0_random4": 1,
				"hidden_random_1_random0": 1,
				"hidden_random_1_random1": -1,
				"hidden_random_1_random2": 1,
				"hidden_random_1_random3": -1,
				"hidden_random_1_random4": 1,
			},
		},
		"'charts' and 'hidden_charts' set": {
			prepare: prepareExampleChartsAndHiddenCharts,
			wantCollected: map[string]int64{
				"hidden_random_0_random0": 1,
				"hidden_random_0_random1": -1,
				"hidden_random_0_random2": 1,
				"hidden_random_0_random3": -1,
				"hidden_random_0_random4": 1,
				"hidden_random_1_random0": 1,
				"hidden_random_1_random1": -1,
				"hidden_random_1_random2": 1,
				"hidden_random_1_random3": -1,
				"hidden_random_1_random4": 1,
				"random_0_random0":        1,
				"random_0_random1":        -1,
				"random_0_random2":        1,
				"random_0_random3":        -1,
				"random_0_random4":        1,
				"random_1_random0":        1,
				"random_1_random1":        -1,
				"random_1_random2":        1,
				"random_1_random3":        -1,
				"random_1_random4":        1,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			example := test.prepare()
			require.NoError(t, example.Init())

			collected := example.Collect()

			assert.Equal(t, test.wantCollected, collected)
			ensureCollectedHasAllChartsDimsVarsIDs(t, example, collected)
		})
	}
}

func ensureCollectedHasAllChartsDimsVarsIDs(t *testing.T, e *Example, collected map[string]int64) {
	for _, chart := range *e.Charts() {
		if chart.Obsolete {
			continue
		}
		for _, dim := range chart.Dims {
			_, ok := collected[dim.ID]
			assert.Truef(t, ok,
				"collected metrics has no data for dim '%s' chart '%s'", dim.ID, chart.ID)
		}
		for _, v := range chart.Vars {
			_, ok := collected[v.ID]
			assert.Truef(t, ok,
				"collected metrics has no data for var '%s' chart '%s'", v.ID, chart.ID)
		}
	}
}

func prepareExampleDefault() *Example {
	return prepareExample(New().Config)
}

func prepareExampleOnlyCharts() *Example {
	return prepareExample(Config{
		Charts: ConfigCharts{
			Num:  2,
			Dims: 5,
		},
	})
}

func prepareExampleOnlyHiddenCharts() *Example {
	return prepareExample(Config{
		HiddenCharts: ConfigCharts{
			Num:  2,
			Dims: 5,
		},
	})
}

func prepareExampleChartsAndHiddenCharts() *Example {
	return prepareExample(Config{
		Charts: ConfigCharts{
			Num:  2,
			Dims: 5,
		},
		HiddenCharts: ConfigCharts{
			Num:  2,
			Dims: 5,
		},
	})
}

func prepareExample(cfg Config) *Example {
	example := New()
	example.Config = cfg
	example.randInt = func() int64 { return 1 }
	return example
}
