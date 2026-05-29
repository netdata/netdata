// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

func TestCollector_ChartTemplateYAML(t *testing.T) {
	tests := map[string]struct {
		check func(*testing.T, string)
	}{
		"matches schema": {
			check: func(t *testing.T, yaml string) {
				collecttest.AssertChartTemplateSchema(t, yaml)
			},
		},
		"compiles": {
			check: func(t *testing.T, yaml string) {
				spec, err := charttpl.DecodeYAML([]byte(yaml))
				require.NoError(t, err)
				_, err = chartengine.Compile(spec, 1)
				require.NoError(t, err)
			},
		},
		"dynamic charts declare lifecycle": {
			check: func(t *testing.T, yaml string) {
				spec, err := charttpl.DecodeYAML([]byte(yaml))
				require.NoError(t, err)

				for _, group := range spec.Groups {
					for _, chart := range group.Charts {
						if chart.Instances == nil {
							continue
						}
						require.NotNil(t, chart.Lifecycle, "chart %s must declare lifecycle", chart.ID)
						require.Equal(t, 5, chart.Lifecycle.ExpireAfterCycles, "chart %s lifecycle", chart.ID)
					}
				}
			},
		},
	}

	c := New()
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.check(t, c.ChartTemplateYAML())
		})
	}
}
