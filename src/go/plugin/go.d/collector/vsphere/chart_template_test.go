// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

func TestCollector_ChartTemplateYAML(t *testing.T) {
	collecttest.AssertChartTemplateSchema(t, chartTemplateYAML)

	spec, err := charttpl.DecodeYAML([]byte(chartTemplateYAML))
	require.NoError(t, err)
	assertUniqueChartPriorities(t, spec)

	_, err = chartengine.Compile(spec, 1)
	require.NoError(t, err)
}

func assertUniqueChartPriorities(t *testing.T, spec *charttpl.Spec) {
	t.Helper()

	seen := make(map[int]string)
	for _, group := range spec.Groups {
		for _, chart := range group.Charts {
			if other, ok := seen[chart.Priority]; ok {
				require.Failf(t, "duplicate chart priority", "priority %d is used by %s and %s", chart.Priority, other, chart.Context)
			}
			seen[chart.Priority] = chart.Context
		}
	}
}
