// SPDX-License-Identifier: GPL-3.0-or-later

package catofunc

import (
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

func topologyMethodConfig(updateEvery int) funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:           TopologyMethodID,
		Aliases:      []string{TopologyMethodID},
		Name:         "Topology (Cato Networks)",
		UpdateEvery:  updateEvery,
		Help:         "Cato Networks site, PoP, tunnel, and BGP topology data",
		RequireCloud: true,
		ResponseType: "topology",
	}.WithPresentation(catoTopologyPresentation())
}

func catoTopologyPresentation() *topology.Presentation {
	return &topology.Presentation{
		ActorTypes: map[string]topology.PresentationActorType{
			ActorTypeSite: {
				Label:     "Site",
				ColorSlot: "green",
				Border:    true,
				Role:      "node",
				SummaryFields: []topology.PresentationSummaryField{
					{Key: "name", Label: "Name", Sources: []string{"attributes"}},
					{Key: "connectivity_status", Label: "Connectivity", Sources: []string{"attributes"}},
					{Key: "operational_status", Label: "Operational status", Sources: []string{"attributes"}},
					{Key: "pop_name", Label: "PoP", Sources: []string{"attributes"}},
					{Key: "host_count", Label: "Hosts", Sources: []string{"attributes"}},
				},
				Tables: map[string]topology.PresentationTable{
					"interfaces": {
						Label:  "Interfaces",
						Source: "tables.interfaces",
						Order:  10,
						Columns: []topology.PresentationTableColumn{
							{Key: "name", Label: "Name"},
							{Key: "type", Label: "Type"},
							{Key: "connected", Label: "Connected", Type: "boolean"},
							{Key: "pop_name", Label: "PoP"},
							{Key: "tunnel_remote_ip", Label: "Remote IP"},
							{Key: "tunnel_uptime", Label: "Uptime", Type: "duration"},
							{Key: "upstream_bandwidth", Label: "Upstream bandwidth"},
							{Key: "downstream_bandwidth", Label: "Downstream bandwidth"},
						},
					},
					"devices": {
						Label:  "Devices",
						Source: "tables.devices",
						Order:  20,
						Columns: []topology.PresentationTableColumn{
							{Key: "name", Label: "Name"},
							{Key: "type", Label: "Type"},
							{Key: "connected", Label: "Connected", Type: "boolean"},
							{Key: "ha_role", Label: "HA role"},
							{Key: "socket_serial", Label: "Serial"},
							{Key: "socket_version", Label: "Version"},
							{Key: "internal_ip", Label: "Internal IP"},
						},
					},
				},
				ModalTabs: []topology.PresentationModalTab{
					{ID: "interfaces", Label: "Interfaces", Type: "table"},
					{ID: "devices", Label: "Devices", Type: "table"},
				},
			},
			ActorTypePop: {
				Label:     "Cato PoP",
				ColorSlot: "blue",
				Border:    true,
				Role:      "network",
				SummaryFields: []topology.PresentationSummaryField{
					{Key: "name", Label: "Name", Sources: []string{"attributes"}},
				},
			},
			ActorTypeBGPPeer: {
				Label:     "BGP peer",
				ColorSlot: "orange",
				Border:    true,
				Role:      "network",
				SummaryFields: []topology.PresentationSummaryField{
					{Key: "remote_ip", Label: "Remote IP", Sources: []string{"attributes"}},
					{Key: "remote_asn", Label: "Remote ASN", Sources: []string{"attributes"}},
					{Key: "bgp_session", Label: "Session", Sources: []string{"attributes"}},
				},
			},
		},
		LinkTypes: map[string]topology.PresentationLinkType{
			LinkTypeTunnel: {Label: "Cato tunnel", ColorSlot: "green", Width: 2},
			LinkTypeBGP:    {Label: "BGP session", ColorSlot: "orange", Width: 2},
		},
		Legend: topology.PresentationLegend{
			Actors: []topology.PresentationLegendEntry{
				{Type: ActorTypeSite, Label: "Cato site"},
				{Type: ActorTypePop, Label: "Cato PoP"},
				{Type: ActorTypeBGPPeer, Label: "BGP peer"},
			},
			Links: []topology.PresentationLegendEntry{
				{Type: LinkTypeTunnel, Label: "Cato tunnel"},
				{Type: LinkTypeBGP, Label: "BGP session"},
			},
		},
		ActorClickBehavior: "highlight_connections",
	}
}
