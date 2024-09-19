// SPDX-License-Identifier: GPL-3.0-or-later

package testrandom

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

func TestTestRandom_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &TestRandom{}, dataConfigJSON, dataConfigYAML)
}

func TestNew(t *testing.T) {
	assert.IsType(t, (*TestRandom)(nil), New())
}

func TestTestRandom_Init(t *testing.T) {
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
			tr := New()
			tr.Config = test.config

			if test.wantFail {
				assert.Error(t, tr.Init())
			} else {
				assert.NoError(t, tr.Init())
			}
		})
	}
}

func TestTestRandom_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() *TestRandom
		wantFail bool
	}{
		"success on default":                            {prepare: prepareTRDefault},
		"success when only 'charts' set":                {prepare: prepareTROnlyCharts},
		"success when only 'hidden_charts' set":         {prepare: prepareTROnlyHiddenCharts},
		"success when 'charts' and 'hidden_charts' set": {prepare: prepareTRChartsAndHiddenCharts},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			tr := test.prepare()
			require.NoError(t, tr.Init())

			if test.wantFail {
				assert.Error(t, tr.Check())
			} else {
				assert.NoError(t, tr.Check())
			}
		})
	}
}

func TestTestRandom_Charts(t *testing.T) {
	tests := map[string]struct {
		prepare func(t *testing.T) *TestRandom
		wantNil bool
	}{
		"not initialized collector": {
			wantNil: true,
			prepare: func(t *testing.T) *TestRandom {
				return New()
			},
		},
		"initialized collector": {
			prepare: func(t *testing.T) *TestRandom {
				tr := New()
				require.NoError(t, tr.Init())
				return tr
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			tr := test.prepare(t)

			if test.wantNil {
				assert.Nil(t, tr.Charts())
			} else {
				assert.NotNil(t, tr.Charts())
			}
		})
	}
}

func TestTestRandom_Cleanup(t *testing.T) {
	assert.NotPanics(t, New().Cleanup)
}

func TestTestRandom_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func() *TestRandom
		wantCollected map[string]int64
	}{
		"default config": {
			prepare: prepareTRDefault,
			wantCollected: map[string]int64{
				"random_0_random0": 1,
				"random_0_random1": -1,
				"random_0_random2": 1,
				"random_0_random3": -1,
			},
		},
		"only 'charts' set": {
			prepare: prepareTROnlyCharts,
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
			prepare: prepareTROnlyHiddenCharts,
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
			prepare: prepareTRChartsAndHiddenCharts,
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
			tr := test.prepare()
			require.NoError(t, tr.Init())

			mx := tr.Collect()

			assert.Equal(t, test.wantCollected, mx)
			module.TestMetricsHasAllChartsDims(t, tr.Charts(), mx)
		})
	}
}

func prepareTRDefault() *TestRandom {
	return prepareTR(New().Config)
}

func prepareTROnlyCharts() *TestRandom {
	return prepareTR(Config{
		Charts: ConfigCharts{
			Num:  2,
			Dims: 5,
		},
	})
}

func prepareTROnlyHiddenCharts() *TestRandom {
	return prepareTR(Config{
		HiddenCharts: ConfigCharts{
			Num:  2,
			Dims: 5,
		},
	})
}

func prepareTRChartsAndHiddenCharts() *TestRandom {
	return prepareTR(Config{
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

func prepareTR(cfg Config) *TestRandom {
	tr := New()
	tr.Config = cfg
	tr.randInt = func() int64 { return 1 }
	return tr
}
