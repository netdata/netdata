// SPDX-License-Identifier: GPL-3.0-or-later

package catofunc

import (
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
)

func topologyMethodConfig(updateEvery int) funcapi.MethodConfig {
	return funcapi.MethodConfig{
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
	return map[string]topologyv1.ActorType{
		ActorTypeSite: {
			Layer:             "network",
			Identity:          []string{"id"},
			MergeIdentity:     []string{"account_id", "site_id"},
			AggregationScopes: []string{"site", "network"},
			Search:            &topologyv1.ActorSearchPolicy{Columns: []string{"display_name", "site_id", "pop_name", "country_name", "region"}},
			Presentation: &topologyv1.ActorPresentation{
				Label:     "Cato site",
				Role:      "actor",
				Icon:      "network",
				ColorSlot: "green",
				Border:    &topologyv1.BorderPresentation{Enabled: new(true)},
				Size:      &topologyv1.ActorSizePresentation{Mode: "link_count", Scale: "emphasized"},
				Layout:    &topologyv1.ActorLayoutPresentation{Repulsion: "stronger"},
				LabelPolicy: &topologyv1.LabelPolicy{
					Columns:   []string{"display_name", "site_id"},
					Fallback:  "type_label",
					MaxLength: 80,
					Array:     "reject",
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
			Search:            &topologyv1.ActorSearchPolicy{Columns: []string{"display_name", "device_id", "socket_serial"}},
			Presentation: &topologyv1.ActorPresentation{
				Label:     "Cato device",
				Role:      "actor",
				Icon:      "device",
				ColorSlot: "green",
				Border:    &topologyv1.BorderPresentation{Enabled: new(true)},
				Size:      &topologyv1.ActorSizePresentation{Mode: "link_count", Scale: "normal"},
				LabelPolicy: &topologyv1.LabelPolicy{
					Columns:   []string{"display_name", "device_id"},
					Fallback:  "type_label",
					MaxLength: 80,
					Array:     "reject",
				},
				Modal: catoDeviceModal(),
			},
		},
		ActorTypePop: {
			Layer:             "network",
			Identity:          []string{"id"},
			MergeIdentity:     []string{"account_id", "pop_name"},
			AggregationScopes: []string{"pop", "network"},
			Search:            &topologyv1.ActorSearchPolicy{Columns: []string{"display_name", "pop_name"}},
			Presentation: &topologyv1.ActorPresentation{
				Label:     "Cato PoP",
				Role:      "actor",
				Icon:      "cloud",
				ColorSlot: "blue",
				Border:    &topologyv1.BorderPresentation{Enabled: new(true)},
				Size:      &topologyv1.ActorSizePresentation{Mode: "link_count", Scale: "normal"},
				LabelPolicy: &topologyv1.LabelPolicy{
					Columns:   []string{"display_name", "pop_name"},
					Fallback:  "type_label",
					MaxLength: 80,
					Array:     "reject",
				},
			},
		},
		ActorTypeBGPPeer: {
			Layer:             "network",
			Identity:          []string{"id"},
			MergeIdentity:     []string{"remote_ip", "remote_asn"},
			ParentIdentity:    []string{"site_id"},
			AggregationScopes: []string{"network"},
			Search:            &topologyv1.ActorSearchPolicy{Columns: []string{"display_name", "remote_ip", "remote_asn"}},
			Presentation: &topologyv1.ActorPresentation{
				Label:     "BGP peer",
				Role:      "endpoint",
				Icon:      "remote-endpoint",
				ColorSlot: "orange",
				Border:    &topologyv1.BorderPresentation{Enabled: new(true)},
				Size:      &topologyv1.ActorSizePresentation{Mode: "fixed", Scale: "compact"},
				LabelPolicy: &topologyv1.LabelPolicy{
					Columns:   []string{"display_name", "remote_ip", "remote_asn"},
					Fallback:  "type_label",
					MaxLength: 80,
					Array:     "reject",
				},
				Modal: catoBGPPeerModal(),
			},
		},
	}
}

func TopologyLinkTypes() map[string]topologyv1.LinkType {
	return map[string]topologyv1.LinkType{
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
				Label:     "Cato tunnel",
				ColorSlot: "green",
				Width:     "normal",
				Curve:     "straight",
				Arrow:     "none",
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
}

func TopologyPresentation() *topologyv1.Presentation {
	return &topologyv1.Presentation{
		ProfileVersion: "cato-networks.v1",
		Selection: &topologyv1.SelectionPresentation{
			ActorClick: &topologyv1.ActorClickPresentation{Mode: "highlight_connections"},
		},
		Legend: &topologyv1.PresentationLegend{
			Actors: []topologyv1.LegendEntry{
				{Type: ActorTypeSite, Label: "Cato site"},
				{Type: ActorTypeDevice, Label: "Cato device"},
				{Type: ActorTypePop, Label: "Cato PoP"},
				{Type: ActorTypeBGPPeer, Label: "BGP peer"},
			},
			Links: []topologyv1.LegendEntry{
				{Type: LinkTypeTunnel, Label: "Cato tunnel"},
				{Type: LinkTypeBGP, Label: "BGP session"},
			},
		},
	}
}

func catoSiteModal() *topologyv1.ModalPresentation {
	return &topologyv1.ModalPresentation{
		MiniTopology: &topologyv1.ModalMiniTopologyPresentation{Depth: 1},
	}
}

func catoDeviceModal() *topologyv1.ModalPresentation {
	return &topologyv1.ModalPresentation{
		MiniTopology: &topologyv1.ModalMiniTopologyPresentation{Depth: 1},
		Sections: []topologyv1.ModalSection{
			{
				ID:    ActorTableInterfaces,
				Label: "Interfaces",
				Order: 1,
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
					modalDirectColumn("type", "Type", "type", "badge"),
					modalDirectColumn("connected", "Connected", "connected", "badge"),
					modalDirectColumn("pop_name", "PoP", "pop_name", "text"),
					modalDirectColumn("tunnel_remote_ip", "Remote IP", "tunnel_remote_ip", "text"),
					modalDirectColumn("tunnel_uptime", "Uptime", "tunnel_uptime", "duration"),
					modalDirectColumn("upstream_bandwidth", "Upstream bandwidth", "upstream_bandwidth", "number"),
					modalDirectColumn("downstream_bandwidth", "Downstream bandwidth", "downstream_bandwidth", "number"),
				},
				Sort: &topologyv1.ModalSort{Column: "name", Direction: "asc"},
			},
		},
	}
}

func catoBGPPeerModal() *topologyv1.ModalPresentation {
	return &topologyv1.ModalPresentation{
		MiniTopology: &topologyv1.ModalMiniTopologyPresentation{Depth: 1},
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
