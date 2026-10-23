// SPDX-License-Identifier: GPL-3.0-or-later

package catofunc

import (
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
)

func topologyFunctionConfig(updateEvery int) funcapi.FunctionConfig {
	return funcapi.FunctionConfig{
		ID:           TopologyMethodID,
		Aliases:      []string{TopologyMethodID},
		Name:         "Topology (Cato Networks)",
		UpdateEvery:  updateEvery,
		Help:         "Cato Networks site, device, PoP, tunnel, and BGP topology data",
		RequireCloud: true,
		ResponseType: topologyv1.ResponseType,
	}.WithPresentation(TopologyPresentation())
}

func TopologyActorTypes() map[string]topologyv1.ActorType {
	types := map[string]topologyv1.ActorType{
		ActorTypeSite: {
			Layer:             "network",
			Identity:          []string{"id"},
			MergeIdentity:     []string{"account_id", "site_id"},
			AggregationScopes: []string{"site", "network"},
			Search: &topologyv1.ActorSearchPolicy{
				Columns: []string{
					"display_name",
					"site_id",
					"description",
					"pop_name",
					"country_code",
					"country_name",
					"region",
					"site_type",
					"connection_type",
				},
			},
			Presentation: &topologyv1.ActorPresentation{
				Label:     "Cato site",
				Role:      "actor",
				Icon:      "network",
				ColorSlot: "green",
				Size: &topologyv1.ActorSizePresentation{
					Mode:  "link_count",
					Scale: "emphasized",
				},
				Layout: &topologyv1.ActorLayoutPresentation{
					Repulsion: "stronger",
				},
				LabelPolicy: &topologyv1.LabelPolicy{
					Columns: []string{"display_name", "site_id"},
				},
				Modal: catoSiteModal(),
			},
		},
		ActorTypeDevice: {
			Layer:             "network",
			Identity:          []string{"id"},
			MergeIdentity:     []string{"account_id", "site_id", "device_id"},
			ParentIdentity:    []string{"site_id"},
			AggregationScopes: []string{"site", "network"},
			Search: &topologyv1.ActorSearchPolicy{
				Columns: []string{
					"display_name",
					"site_id",
					"device_id",
					"socket_serial",
					"socket_version",
					"internal_ip",
					"ha_role",
					"pop_name",
				},
			},
			Presentation: &topologyv1.ActorPresentation{
				Label:     "Cato device",
				Role:      "actor",
				Icon:      "device",
				ColorSlot: "green",
				Size: &topologyv1.ActorSizePresentation{
					Mode:  "link_count",
					Scale: "normal",
				},
				LabelPolicy: &topologyv1.LabelPolicy{
					Columns: []string{"display_name", "device_id"},
				},
				Modal: catoDeviceModal(),
			},
		},
		ActorTypePop: {
			Layer:             "network",
			Identity:          []string{"id"},
			MergeIdentity:     []string{"account_id", "pop_name"},
			AggregationScopes: []string{"pop", "network"},
			Search: &topologyv1.ActorSearchPolicy{
				Columns: []string{"display_name", "pop_name"},
			},
			Presentation: &topologyv1.ActorPresentation{
				Label:     "Cato PoP",
				Role:      "actor",
				Icon:      "cloud",
				ColorSlot: "blue",
				Size: &topologyv1.ActorSizePresentation{
					Mode:  "link_count",
					Scale: "normal",
				},
				LabelPolicy: &topologyv1.LabelPolicy{
					Columns: []string{"display_name", "pop_name"},
				},
			},
		},
		ActorTypeBGPPeer: {
			Layer:             "network",
			Identity:          []string{"id"},
			MergeIdentity:     []string{"remote_ip", "remote_asn"},
			ParentIdentity:    []string{"site_id"},
			AggregationScopes: []string{"network"},
			Search: &topologyv1.ActorSearchPolicy{
				Columns: []string{
					"display_name",
					"site_id",
					"remote_ip",
					"remote_asn",
					"pop_name",
				},
			},
			Presentation: &topologyv1.ActorPresentation{
				Label:     "BGP peer",
				Role:      "endpoint",
				Icon:      "remote-endpoint",
				ColorSlot: "orange",
				Size: &topologyv1.ActorSizePresentation{
					Mode:  "fixed",
					Scale: "compact",
				},
				LabelPolicy: &topologyv1.LabelPolicy{
					Columns: []string{"display_name", "remote_ip", "remote_asn"},
				},
				Modal: catoBGPPeerModal(),
			},
		},
	}
	for id, actorType := range types {
		actorType.Presentation.Border = &topologyv1.BorderPresentation{
			Enabled: new(true),
		}
		actorType.Presentation.LabelPolicy.Fallback = "type_label"
		actorType.Presentation.LabelPolicy.MaxLength = 80
		actorType.Presentation.LabelPolicy.Array = "reject"
		if actorType.Presentation.Modal == nil {
			actorType.Presentation.Modal = &topologyv1.ModalPresentation{}
		}
		modal := actorType.Presentation.Modal
		modal.Enabled = new(true)
		modal.Labels = catoActorLabels(id)
		modal.MiniTopology = &topologyv1.ModalMiniTopologyPresentation{
			Enabled: new(true),
			Depth:   1,
		}
		if id == ActorTypePop {
			section := catoDeviceModal().Sections[0]
			section.ID = "attached_wans"
			section.Label = "Attached WANs"
			section.OwnerFilter.ActorColumn = "pop_actor"
			section.Columns = append([]topologyv1.ModalColumn{catoActorRefColumn("site", "Site", "site_actor"), catoActorRefColumn("device", "Device / owner", "actor")}, section.Columns...)
			modal.Sections = append(modal.Sections, section)
		}
		if id != ActorTypeBGPPeer {
			modal.Sections = append(modal.Sections, catoConnectionsSection())
		}
		if id == ActorTypeDevice || id == ActorTypeBGPPeer {
			actorType.Search.LabelKeys = []string{"site_name", "country_code", "country_name", "region"}
		}
		types[id] = actorType
	}
	return types
}

func TopologyLinkTypes() map[string]topologyv1.LinkType {
	types := map[string]topologyv1.LinkType{
		LinkTypeOwnership: {
			Orientation: "hierarchical", DirectionRole: "ownership", SemanticRole: "ownership",
			Aggregation: topologyv1.LinkAggregation{
				Direction: "preserve",
				Evidence:  "drop",
			},
			Presentation: &topologyv1.LinkPresentation{
				Label:     "Site membership (not a network connection)",
				ColorSlot: "structural",
				Opacity:   "faded",
				LineStyle: "dotted",
				Width:     "thin",
				Arrow:     "none",
			},
		},
		LinkTypeTunnel: {
			Orientation:   "observed_bidirectional",
			DirectionRole: "observation",
			SemanticRole:  "traffic",
			Aggregation: topologyv1.LinkAggregation{
				Direction: "canonicalize_unordered",
				Evidence:  "drop",
				Metrics: map[string]string{
					"evidence_count":        "sum",
					"bytes_upstream_max":    "max",
					"bytes_downstream_max":  "max",
					"lost_upstream_percent": "avg",
					"rtt_ms":                "avg",
				},
			},
			Presentation: &topologyv1.LinkPresentation{
				Label:     "Observed tunnel: connected",
				ColorSlot: "green",
				Width:     "normal",
				Curve:     "auto",
				Arrow:     "none",
				Hover: &topologyv1.HoverPresentation{
					Fields: []topologyv1.PresentationField{
						{Key: "observation", Label: "Evidence"}, {Key: "interface_name", Label: "Interface"}, {Key: "interface_id", Label: "Interface ID"}, {Key: "state", Label: "Tunnel state"},
					},
				},
			},
		},
		LinkTypeBGP: {
			Orientation:   "observed_bidirectional",
			DirectionRole: "observation",
			SemanticRole:  "control",
			Aggregation: topologyv1.LinkAggregation{
				Direction: "canonicalize_unordered",
				Evidence:  "drop",
				Metrics: map[string]string{
					"evidence_count": "sum",
					"routes":         "sum",
					"routes_limit":   "max",
					"rib_out_routes": "sum",
				},
			},
			Presentation: &topologyv1.LinkPresentation{
				Label:     "BGP session",
				ColorSlot: "orange",
				Width:     "normal",
				Curve:     "straight",
				Arrow:     "none",
			},
		},
	}

	for _, variant := range []struct{ id, label, color, line string }{
		{LinkTypeTunnelDisconnected, "Observed tunnel: disconnected", "danger", "dashed"},
		{LinkTypeTunnelUnknown, "Observed tunnel: state unknown", "muted", "dotted"},
		{LinkTypePopAssociation, "Reported PoP association (tunnel unobserved)", "derived", "dashed"},
	} {
		typ := types[LinkTypeTunnel]
		pres := *typ.Presentation
		pres.Label, pres.ColorSlot, pres.LineStyle = variant.label, variant.color, variant.line
		typ.Presentation = &pres
		if variant.id == LinkTypePopAssociation {
			typ.SemanticRole = "discovery"
			typ.Orientation = "undirected"
		}
		types[variant.id] = typ
	}
	return types
}

func TopologyPresentation() *topologyv1.Presentation {
	return &topologyv1.Presentation{
		ProfileVersion: "cato-networks.v1",
		Selection: &topologyv1.SelectionPresentation{
			ActorClick: &topologyv1.ActorClickPresentation{
				Mode: "highlight_connections",
			},
		},
		Legend: &topologyv1.PresentationLegend{
			Actors: []topologyv1.LegendEntry{
				{Type: ActorTypeSite, Label: "Cato site"},
				{Type: ActorTypeDevice, Label: "Cato device"},
				{Type: ActorTypePop, Label: "Cato PoP"},
				{Type: ActorTypeBGPPeer, Label: "BGP peer"},
			},
			Links: []topologyv1.LegendEntry{
				{Type: LinkTypeOwnership, Label: "Site membership (not connectivity)"},
				{Type: LinkTypeTunnel, Label: "Observed WAN tunnel: connected"},
				{Type: LinkTypeTunnelDisconnected, Label: "Observed WAN tunnel: disconnected"},
				{Type: LinkTypeTunnelUnknown, Label: "Observed WAN tunnel: state unknown"},
				{Type: LinkTypePopAssociation, Label: "Reported PoP association: tunnel unobserved"},
				{Type: LinkTypeBGP, Label: "BGP adjacency (inspect session state)"},
			},
		},
	}
}

func catoSiteModal() *topologyv1.ModalPresentation {
	modal := catoDeviceModal()
	modal.Sections[0].Label = "Site WANs (all devices)"
	modal.Sections[0].OwnerFilter.ActorColumn = "site_actor"
	modal.Sections[0].Columns = append([]topologyv1.ModalColumn{catoActorRefColumn("device", "Device / owner", "actor")}, modal.Sections[0].Columns...)
	modal.Sections = append(modal.Sections, topologyv1.ModalSection{
		ID:    "devices",
		Label: "Site devices",
		Order: 2,
		Source: topologyv1.ModalSource{
			Kind: "links",
		},
		OwnerFilter: &topologyv1.ModalOwnerFilter{
			Mode:        "actor_column",
			ActorColumn: "src_actor",
		},
		RowFilters: []topologyv1.ModalRowFilter{{Column: "observation", Op: "eq", Value: "Site membership"}},
		Columns:    []topologyv1.ModalColumn{catoActorRefColumn("device", "Device", "dst_actor"), catoDeviceLabelColumn("ha_role", "HA role"), catoDeviceLabelColumn("connected", "Device connected"), catoDeviceLabelColumn("socket_serial", "Serial"), catoDeviceLabelColumn("socket_version", "Version")},
		EmptyLabel: "No devices reported for this site.",
	})
	return modal
}

func catoDeviceModal() *topologyv1.ModalPresentation {
	return &topologyv1.ModalPresentation{
		MiniTopology: &topologyv1.ModalMiniTopologyPresentation{
			Depth: 1,
		},
		Sections: []topologyv1.ModalSection{
			{
				ID:         ActorTableInterfaces,
				Label:      "WAN interfaces",
				Order:      1,
				EmptyLabel: "No interface observations available. This does not establish that no WAN is configured.",
				Source: topologyv1.ModalSource{
					Kind:  "actor_table",
					Table: ActorTableInterfaces,
				},
				OwnerFilter: &topologyv1.ModalOwnerFilter{
					Mode:        "actor_column",
					ActorColumn: "actor",
				},
				Columns: []topologyv1.ModalColumn{
					modalDirectColumn("name", "Name", "name", "text"),

					modalDirectColumn("tunnel_state", "Tunnel state", "tunnel_state", "badge"),
					modalDirectColumn("physical_link_state", "Physical link", "physical_link_state", "badge"),
					modalDirectColumn("pop_name", "PoP", "pop_name", "text"),
					modalExpandedColumn("tunnel_remote_ip", "Remote IP", "tunnel_remote_ip", "text"),
					modalExpandedColumn("tunnel_uptime", "Uptime", "tunnel_uptime", "duration"),
					modalExpandedColumn("upstream_bandwidth", "Configured upstream limit (Mbps)", "upstream_bandwidth", "number"),
					modalExpandedColumn("downstream_bandwidth", "Configured downstream limit (Mbps)", "downstream_bandwidth", "number"),
					modalDirectColumn("rtt_ms", "RTT (ms)", "rtt_ms", "number"),
					modalDirectColumn("lost_upstream_percent", "Upstream loss (%)", "lost_upstream_percent", "number"),
					modalDirectColumn("lost_downstream_percent", "Downstream loss (%)", "lost_downstream_percent", "number"),
					modalExpandedColumn("jitter_upstream_ms", "Upstream jitter (ms)", "jitter_upstream_ms", "number"),
					modalExpandedColumn("jitter_downstream_ms", "Downstream jitter (ms)", "jitter_downstream_ms", "number"),
					modalExpandedColumn("last_mile_latency_ms", "Last-mile latency (ms)", "last_mile_latency_ms", "number"),
					modalExpandedColumn("last_mile_loss_percent", "Last-mile loss (%)", "last_mile_loss_percent", "number"),
				},
				Sort: &topologyv1.ModalSort{
					Column:    "name",
					Direction: "asc",
				},
			},
		},
	}
}

func catoBGPPeerModal() *topologyv1.ModalPresentation {
	return &topologyv1.ModalPresentation{
		MiniTopology: &topologyv1.ModalMiniTopologyPresentation{
			Depth: 1,
		},
		Sections: []topologyv1.ModalSection{
			{
				ID:    "links",
				Label: "Links",
				Order: 1,
				Source: topologyv1.ModalSource{
					Kind: "links",
				},
				OwnerFilter: &topologyv1.ModalOwnerFilter{
					Mode:           "incident_link",
					SrcActorColumn: "src_actor",
					DstActorColumn: "dst_actor",
				},
				Columns: []topologyv1.ModalColumn{
					{
						ID:    "remote",
						Label: "Remote Actor",
						Projection: topologyv1.ModalProjection{
							Kind:           "opposite_actor",
							SrcActorColumn: "src_actor",
							DstActorColumn: "dst_actor",
						},
						Cell: "actor_link",
					},
					modalDirectColumn("local_ip", "Local IP", "local_ip", "text"),
					modalDirectColumn("local_asn", "Local ASN", "local_asn", "text"),
					modalDirectColumn("incoming_connection_state", "Incoming connection", "incoming_connection_state", "badge"),
					modalDirectColumn("outgoing_connection_state", "Outgoing connection", "outgoing_connection_state", "badge"),
					modalDirectColumn("protocol", "Protocol", "protocol", "badge"),
					modalDirectColumn("state", "State", "state", "badge"),
					modalDirectColumn("routes", "Routes", "routes", "number"),
					modalDirectColumn("routes_limit", "Routes limit", "routes_limit", "number"),
					modalDirectColumn("routes_limit_exceeded", "Limit exceeded", "routes_limit_exceeded", "badge"),
					modalDirectColumn("rib_out_routes", "RIB out", "rib_out_routes", "number"),
				},
			},
		},
	}
}

func modalDirectColumn(id, label, sourceColumn, cell string) topologyv1.ModalColumn {
	return topologyv1.ModalColumn{
		ID:    id,
		Label: label,
		Projection: topologyv1.ModalProjection{
			Kind:   "direct",
			Column: sourceColumn,
		},
		Cell: cell,
	}
}

func catoActorLabels(actorType string) *topologyv1.ModalLabelsPresentation {
	fields := []topologyv1.ModalLabelIdentificationField{{Key: "account_id", Label: "Account", MaxValues: 1}}
	switch actorType {
	case ActorTypeSite:
		fields = append(fields,
			topologyv1.ModalLabelIdentificationField{
				Key:       "snapshot_collection",
				Label:     "Inventory collection",
				MaxValues: 1,
			},
			topologyv1.ModalLabelIdentificationField{
				Key:       "metrics_collection",
				Label:     "Metrics collection",
				MaxValues: 1,
			},
			topologyv1.ModalLabelIdentificationField{
				Key:       "bgp_collection",
				Label:     "BGP collection",
				MaxValues: 1,
			},
			topologyv1.ModalLabelIdentificationField{
				Key:       "description",
				Label:     "Description",
				MaxValues: 1,
			},
			topologyv1.ModalLabelIdentificationField{
				Key:       "country_name",
				Label:     "Country",
				MaxValues: 1,
			},
			topologyv1.ModalLabelIdentificationField{
				Key:       "connection_type",
				Label:     "Connection",
				MaxValues: 1,
			},
			topologyv1.ModalLabelIdentificationField{
				Key:       "connectivity_status",
				Label:     "Connectivity",
				MaxValues: 1,
			})
	case ActorTypeDevice:
		fields = append(fields,
			topologyv1.ModalLabelIdentificationField{
				Key:       "source_name",
				Label:     "Source name",
				MaxValues: 1,
			},
			topologyv1.ModalLabelIdentificationField{
				Key:       "site_name",
				Label:     "Site",
				MaxValues: 1,
			},
			topologyv1.ModalLabelIdentificationField{
				Key:       "ha_role",
				Label:     "HA role",
				MaxValues: 1,
			},
			topologyv1.ModalLabelIdentificationField{
				Key:       "socket_serial",
				Label:     "Serial",
				MaxValues: 1,
			},
			topologyv1.ModalLabelIdentificationField{
				Key:       "socket_version",
				Label:     "Version",
				MaxValues: 1,
			},
			topologyv1.ModalLabelIdentificationField{
				Key:       "connected",
				Label:     "Connected",
				MaxValues: 1,
			})
	case ActorTypePop:
		fields = append(fields, topologyv1.ModalLabelIdentificationField{
			Key:       "pop_name",
			Label:     "PoP",
			MaxValues: 1,
		})
	case ActorTypeBGPPeer:
		fields = append(fields,
			topologyv1.ModalLabelIdentificationField{
				Key:       "site_name",
				Label:     "Site",
				MaxValues: 1,
			},
			topologyv1.ModalLabelIdentificationField{
				Key:       "remote_ip",
				Label:     "Remote IP",
				MaxValues: 1,
			},
			topologyv1.ModalLabelIdentificationField{
				Key:       "remote_asn",
				Label:     "Remote ASN",
				MaxValues: 1,
			})
	}
	return &topologyv1.ModalLabelsPresentation{
		Enabled:      new(true),
		Table:        ActorTableLabels,
		ActorColumn:  "actor",
		KeyColumn:    "key",
		ValueColumn:  "value",
		SourceColumn: "source",
		KindColumn:   "kind",
		Identification: &topologyv1.ModalLabelIdentificationPresentation{
			Enabled: new(true),
			Fields:  fields,
		},
	}
}

func catoConnectionsSection() topologyv1.ModalSection {
	return topologyv1.ModalSection{
		ID:    "connections",
		Label: "Connections",
		Order: 3,
		Source: topologyv1.ModalSource{
			Kind: "links",
		},
		OwnerFilter: &topologyv1.ModalOwnerFilter{
			Mode:           "incident_link",
			SrcActorColumn: "src_actor",
			DstActorColumn: "dst_actor",
		},
		Columns: []topologyv1.ModalColumn{
			{ID: "remote", Label: "Remote actor", Cell: "actor_link", Projection: topologyv1.ModalProjection{
				Kind:           "opposite_actor",
				SrcActorColumn: "src_actor",
				DstActorColumn: "dst_actor",
			}},
			catoActorRefColumn("site", "Site", "site_actor"),
			modalDirectColumn("observation", "Relationship evidence", "observation", "text"),
			modalDirectColumn("interface_name", "Interface", "interface_name", "text"),
			modalDirectColumn("interface_id", "Interface ID", "interface_id", "text"),
			modalDirectColumn("state", "State", "state", "badge"),
			modalDirectColumn("rtt_ms", "RTT (ms)", "rtt_ms", "number"),
			modalDirectColumn("lost_upstream_percent", "Upstream loss (%)", "lost_upstream_percent", "number"),
		},
	}
}

func catoActorRefColumn(id, label, column string) topologyv1.ModalColumn {
	return topologyv1.ModalColumn{
		ID:    id,
		Label: label,
		Cell:  "actor_link",
		Projection: topologyv1.ModalProjection{
			Kind:        "actor_ref_label",
			ActorColumn: column,
		},
	}
}
func catoDeviceLabelColumn(key, label string) topologyv1.ModalColumn {
	return topologyv1.ModalColumn{
		ID:    key,
		Label: label,
		Cell:  "text",
		Projection: topologyv1.ModalProjection{
			Kind:        "label_lookup",
			ActorColumn: "dst_actor",
			LabelKey:    key,
			Fallback:    "Unknown",
		},
	}
}

func modalExpandedColumn(id, label, source, cell string) topologyv1.ModalColumn {
	col := modalDirectColumn(id, label, source, cell)
	col.Visibility = "expanded"
	return col
}
