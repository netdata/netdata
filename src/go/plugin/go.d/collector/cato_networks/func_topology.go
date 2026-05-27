// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"context"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/topology"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const topologyMethodID = "topology:cato_networks"

type funcTopology struct {
	collector *Collector
}

var _ funcapi.MethodHandler = (*funcTopology)(nil)

func (f *funcTopology) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	return nil, nil
}

func (f *funcTopology) Handle(_ context.Context, method string, _ funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method != topologyMethodID {
		return funcapi.NotFoundResponse(method)
	}
	if f.collector == nil {
		return funcapi.UnavailableResponse("Cato Networks topology data is not available yet")
	}

	data, ok := f.collector.currentTopology()
	if !ok {
		return funcapi.UnavailableResponse("Cato Networks topology data is not available yet")
	}

	return &funcapi.FunctionResponse{
		Status:       200,
		Help:         "Cato Networks site, PoP, tunnel, and BGP topology data",
		ResponseType: "topology",
		Data:         data,
	}
}

func (f *funcTopology) Cleanup(context.Context) {
	// Topology handler stores only a collector reference; the collector owns lifecycle state.
}

func catoMethods() []funcapi.MethodConfig {
	return []funcapi.MethodConfig{catoTopologyMethodConfig()}
}

func catoFunctionHandler(job collectorapi.RuntimeJob) funcapi.MethodHandler {
	c, ok := job.Collector().(*Collector)
	if !ok {
		return nil
	}
	return &funcTopology{collector: c}
}

func catoTopologyMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:           topologyMethodID,
		Aliases:      []string{topologyMethodID},
		Name:         "Topology (Cato Networks)",
		UpdateEvery:  defaultUpdateEvery,
		Help:         "Cato Networks site, PoP, tunnel, and BGP topology data",
		RequireCloud: true,
		ResponseType: "topology",
	}.WithPresentation(catoTopologyPresentation())
}

func catoTopologyPresentation() *topology.Presentation {
	return &topology.Presentation{
		ActorTypes: map[string]topology.PresentationActorType{
			actorTypeSite: {
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
			actorTypePop: {
				Label:     "Cato PoP",
				ColorSlot: "blue",
				Border:    true,
				Role:      "network",
				SummaryFields: []topology.PresentationSummaryField{
					{Key: "name", Label: "Name", Sources: []string{"attributes"}},
				},
			},
			actorTypeBGPPeer: {
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
			linkTypeTunnel: {Label: "Cato tunnel", ColorSlot: "green", Width: 2},
			linkTypeBGP:    {Label: "BGP session", ColorSlot: "orange", Width: 2},
		},
		Legend: topology.PresentationLegend{
			Actors: []topology.PresentationLegendEntry{
				{Type: actorTypeSite, Label: "Cato site"},
				{Type: actorTypePop, Label: "Cato PoP"},
				{Type: actorTypeBGPPeer, Label: "BGP peer"},
			},
			Links: []topology.PresentationLegendEntry{
				{Type: linkTypeTunnel, Label: "Cato tunnel"},
				{Type: linkTypeBGP, Label: "BGP session"},
			},
		},
		ActorClickBehavior: "highlight_connections",
	}
}
