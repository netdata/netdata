// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/stretchr/testify/require"
)

func TestEnrichLocalActorChartReferencesAddsTypedPortDetails(t *testing.T) {
	actor := &topologyActor{
		Detail: topologyActorDetail{
			L2: topologyengine.ProjectionActorDetail{
				Device: topologyengine.ProjectionDeviceActorDetail{
					Ports: []topologyengine.ProjectionPortDetail{
						{Name: "Gi0/1"},
						{IfName: "Gi0/2"},
						{Name: "Gi0/3"},
					},
				},
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

	tests := map[string]struct {
		port        topologyengine.ProjectionPortDetail
		wantSuffix  string
		wantMetrics []string
	}{
		"name-match":    {port: actor.Detail.L2.Device.Ports[0], wantSuffix: "gi0_1", wantMetrics: []string{"errors", "traffic"}},
		"if-name-match": {port: actor.Detail.L2.Device.Ports[1], wantSuffix: "gi0/2", wantMetrics: []string{"drops"}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.wantSuffix, tc.port.ChartIDSuffix)
			require.Equal(t, tc.wantMetrics, tc.port.AvailableMetrics)
		})
	}

	require.Empty(t, actor.Detail.L2.Device.Ports[2].ChartIDSuffix)
	require.Empty(t, actor.Detail.L2.Device.Ports[2].AvailableMetrics)
}
