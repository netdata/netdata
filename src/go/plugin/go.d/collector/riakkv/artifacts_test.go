// SPDX-License-Identifier: GPL-3.0-or-later

package riakkv

import (
	"cmp"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
)

func TestArtifacts(t *testing.T) {
	read := func(path string) []byte { data, err := os.ReadFile(path); require.NoError(t, err); return data }
	metadata := read("metadata.yaml")
	health := read("../../../../../health/health.d/riakkv.conf")

	module, err := collecttest.DecodeMetadataModule(metadata, "")
	require.NoError(t, err)
	require.NoError(t, collecttest.CheckMetadataMetricsMatchCharts(module.Metrics, chartsByContext(t), nil))
	collecttest.AssertHealthAlertsMatchMetadata(t, health, metadata)
}

// chartsByContext returns the static charts keyed by context. Every chart needs its own ID and context: the Agent
// keeps one chart per ID, so a second chart with the same ID replaces the first one's context.
func chartsByContext(t *testing.T) map[string]charttpl.Chart {
	ids := make(map[string]bool)
	out := make(map[string]charttpl.Chart)
	for _, chart := range charts {
		require.Falsef(t, ids[chart.ID], "chart ID %q is used twice", chart.ID)
		ids[chart.ID] = true
		_, ok := out[chart.Ctx]
		require.Falsef(t, ok, "context %q is used twice", chart.Ctx)

		tpl := charttpl.Chart{Title: chart.Title, Units: chart.Units, Type: cmp.Or(chart.Type.String(), "line")}
		for _, dim := range chart.Dims {
			tpl.Dimensions = append(tpl.Dimensions, charttpl.Dimension{Name: cmp.Or(dim.Name, dim.ID)})
		}
		out[chart.Ctx] = tpl
	}
	return out
}
