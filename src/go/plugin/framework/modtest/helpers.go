// SPDX-License-Identifier: GPL-3.0-or-later

package modtest

import (
	"encoding/json"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/module"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

type configurationProvider interface {
	Configuration() any
}

func TestConfigurationSerialize(t *testing.T, mod configurationProvider, cfgJSON, cfgYAML []byte) {
	t.Helper()
	tests := map[string]struct {
		config    []byte
		unmarshal func(in []byte, out any) error
		marshal   func(in any) ([]byte, error)
	}{
		"json": {config: cfgJSON, marshal: json.Marshal, unmarshal: json.Unmarshal},
		"yaml": {config: cfgYAML, marshal: yaml.Marshal, unmarshal: yaml.Unmarshal},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, test.unmarshal(test.config, mod), "unmarshal test->mod")
			bs, err := test.marshal(mod.Configuration())
			require.NoError(t, err, "marshal mod config")

			var want map[string]any
			var got map[string]any

			require.NoError(t, test.unmarshal(test.config, &want), "unmarshal test->map")
			require.NoError(t, test.unmarshal(bs, &got), "unmarshal mod->map")
			require.NotNil(t, want, "want map")
			require.NotNil(t, got, "got map")
			assert.Equal(t, want, got)
		})
	}
}

func TestMetricsHasAllChartsDims(t *testing.T, charts *module.Charts, mx map[string]int64) {
	TestMetricsHasAllChartsDimsSkip(t, charts, mx, nil)
}

func TestMetricsHasAllChartsDimsSkip(t *testing.T, charts *module.Charts, mx map[string]int64, skip func(chart *module.Chart, dim *module.Dim) bool) {
	for _, chart := range *charts {
		if chart.Obsolete {
			continue
		}
		for _, dim := range chart.Dims {
			if skip != nil && skip(chart, dim) {
				continue
			}
			_, ok := mx[dim.ID]
			assert.Truef(t, ok, "missing data for dimension '%s' in chart '%s'", dim.ID, chart.ID)
		}
		for _, v := range chart.Vars {
			_, ok := mx[v.ID]
			assert.Truef(t, ok, "missing data for variable '%s' in chart '%s'", v.ID, chart.ID)
		}
	}
}
