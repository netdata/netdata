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
		"charts use default lifecycle": {
			check: func(t *testing.T, yaml string) {
				spec, err := charttpl.DecodeYAML([]byte(yaml))
				require.NoError(t, err)

				for _, group := range spec.Groups {
					for _, chart := range group.Charts {
						require.Nil(t, chart.Lifecycle, "chart %s must use the default lifecycle", chart.ID)
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
