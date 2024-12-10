// SPDX-License-Identifier: GPL-3.0-or-later

package testrandom

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

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestNew(t *testing.T) {
	assert.IsType(t, (*Collector)(nil), New())
}

func TestCollector_Init(t *testing.T) {
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

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() *Collector
		wantFail bool
	}{
		"success on default":                            {prepare: prepareTRDefault},
		"success when only 'charts' set":                {prepare: prepareTROnlyCharts},
		"success when only 'hidden_charts' set":         {prepare: prepareTROnlyHiddenCharts},
		"success when 'charts' and 'hidden_charts' set": {prepare: prepareTRChartsAndHiddenCharts},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()
			require.NoError(t, collr.Init(context.Background()))

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	tests := map[string]struct {
		prepare func(t *testing.T) *Collector
		wantNil bool
	}{
		"not initialized collector": {
			wantNil: true,
			prepare: func(t *testing.T) *Collector {
				return New()
			},
		},
		"initialized collector": {
			prepare: func(t *testing.T) *Collector {
				collr := New()
				require.NoError(t, collr.Init(context.Background()))
				return collr
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare(t)

			if test.wantNil {
				assert.Nil(t, collr.Charts())
			} else {
				assert.NotNil(t, collr.Charts())
			}
		})
	}
}

func TestCollector_Cleanup(t *testing.T) {
	assert.NotPanics(t, func() { New().Cleanup(context.Background()) })
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func() *Collector
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
			collr := test.prepare()
			require.NoError(t, collr.Init(context.Background()))

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantCollected, mx)
			module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
		})
	}
}

func prepareTRDefault() *Collector {
	return prepareTR(New().Config)
}

func prepareTROnlyCharts() *Collector {
	return prepareTR(Config{
		Charts: ConfigCharts{
			Num:  2,
			Dims: 5,
		},
	})
}

func prepareTROnlyHiddenCharts() *Collector {
	return prepareTR(Config{
		HiddenCharts: ConfigCharts{
			Num:  2,
			Dims: 5,
		},
	})
}

func prepareTRChartsAndHiddenCharts() *Collector {
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

func prepareTR(cfg Config) *Collector {
	collr := New()
	collr.Config = cfg
	collr.randInt = func() int64 { return 1 }
	return collr
}
