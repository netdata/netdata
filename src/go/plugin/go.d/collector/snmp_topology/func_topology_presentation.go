// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

func snmpTopologyPresentation() *topology.Presentation {
	deviceSummaryFields := topologyDeviceSummaryFields()
	deviceTables := topologyDeviceTables()
	linkOnlyTables := topologyLinkOnlyTables(deviceTables["links"])
	infoOnlyTabs := topologyInfoOnlyTabs()

	return &topology.Presentation{
		ActorTypes: topologyPresentationActorTypes(
			deviceSummaryFields,
			deviceTables,
			linkOnlyTables,
			infoOnlyTabs,
			topologySegmentSummaryFields(),
			topologyEndpointSummaryFields(),
		),
		LinkTypes:          topologyPresentationLinkTypes(),
		PortFields:         topologyPresentationPortFields(),
		PortTypes:          topologyPresentationPortTypes(),
		Legend:             topologyPresentationLegend(),
		ActorClickBehavior: "highlight_connections",
	}
}

func topologyMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:           topologyMethodID,
		Aliases:      []string{topologyMethodID},
		Name:         "Topology (SNMP)",
		UpdateEvery:  10,
		Help:         "SNMP Layer-2 topology and neighbor discovery data",
		RequireCloud: true,
		ResponseType: "topology",
		AgentWide:    true,
		RequiredParams: []funcapi.ParamConfig{
			topologyNodesIdentityParamConfig(),
			topologyMapTypeParamConfig(),
			topologyInferenceStrategyParamConfig(),
			topologyManagedFocusParamConfig(nil),
			topologyDepthParamConfig(),
		},
	}.WithPresentation(snmpTopologyPresentation())
}
