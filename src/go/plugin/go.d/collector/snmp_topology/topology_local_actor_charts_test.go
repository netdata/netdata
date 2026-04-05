// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnrichLocalActorChartReferencesAddsStatusAndPortRows(t *testing.T) {
	actor := &topologyActor{
		Attributes: map[string]any{
			"if_statuses": []map[string]any{
				{"if_name": "Gi0/1"},
				{"if_name": "Gi0/2"},
			},
		},
		Tables: map[string][]map[string]any{
			"ports": {
				{"name": "Gi0/1"},
				{"name": "Gi0/3"},
			},
		},
	}

	enrichLocalActorChartReferences(actor, map[string]topologyInterfaceChartRef{
		"Gi0/1": {
			ChartIDSuffix:    "gi0_1",
			AvailableMetrics: []string{"errors", "traffic", "traffic"},
		},
		"gi0/2": {
			AvailableMetrics: []string{"drops"},
		},
	})

	statuses, ok := actor.Attributes["if_statuses"].([]map[string]any)
	require.True(t, ok)
	require.Equal(t, "gi0_1", statuses[0]["chart_id_suffix"])
	require.Equal(t, []string{"errors", "traffic"}, statuses[0]["available_metrics"])
	require.Equal(t, "gi0/2", statuses[1]["chart_id_suffix"])
	require.Equal(t, []string{"drops"}, statuses[1]["available_metrics"])

	require.Equal(t, "gi0_1", actor.Tables["ports"][0]["chart_id_suffix"])
	require.Equal(t, []string{"errors", "traffic"}, actor.Tables["ports"][0]["available_metrics"])
	require.NotContains(t, actor.Tables["ports"][1], "chart_id_suffix")
}

func TestEnrichTopologyInterfaceStatusesWithChartRefsSupportsAnySlices(t *testing.T) {
	statuses := []any{
		map[string]any{"if_name": "Gi0/10"},
		"not-a-map",
	}

	enriched := enrichTopologyInterfaceStatusesWithChartRefs(statuses, map[string]topologyInterfaceChartRef{
		"gi0/10": {
			ChartIDSuffix:    "gi0_10",
			AvailableMetrics: []string{"traffic"},
		},
	}).([]any)

	status, ok := enriched[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "gi0_10", status["chart_id_suffix"])
	require.Equal(t, []string{"traffic"}, status["available_metrics"])
	require.Equal(t, "not-a-map", enriched[1])
}
