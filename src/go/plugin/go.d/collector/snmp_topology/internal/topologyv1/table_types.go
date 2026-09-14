// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import topologyapi "github.com/netdata/netdata/go/plugins/pkg/topology/v1"

func snmpTopologyV1ActorPortsTableType() topologyapi.TableType {
	return topologyapi.TableType{
		Role:        "actor_detail",
		Owner:       "actor",
		Aggregation: "append",
		Columns:     snmpTopologyV1ActorPortsColumns(),
		Presentation: &topologyapi.TableTypePresentation{
			Label:   "Ports",
			Order:   1,
			Columns: snmpTopologyV1PortModalColumns(),
		},
	}
}

func snmpTopologyV1PortModalColumns() []topologyapi.ModalColumn {
	return []topologyapi.ModalColumn{
		modalDirectColumn("if_index", "Port ID", "if_index", "number"),
		modalDirectColumn("name", "Port", "name", "text"),
		modalDirectColumn("oper_status", "Status", "oper_status", "badge"),
		modalDirectColumn("admin_status", "Admin", "admin_status", "badge"),
		modalDirectColumn("port_type", "Type", "port_type", "badge"),
		modalDirectColumn("link_mode", "Mode", "link_mode", "badge"),
		modalDirectColumn("topology_role", "Role", "topology_role", "badge"),
		modalDirectColumn("vlan_ids", "VLANs", "vlan_ids", "array_count"),
		modalDirectColumn("fdb_mac_count", "FDB", "fdb_mac_count", "number"),
		modalDirectColumn("link_count", "Links", "link_count", "number"),
		modalDirectColumn("neighbor_count", "Neighbors", "neighbor_count", "number"),
		modalActorRefColumnWithVisibility("neighbor_actor", "Neighbor", "neighbor_actor", "expanded"),
		modalDirectColumnWithVisibility("neighbor_port_name", "Neighbor Port", "neighbor_port_name", "text", "expanded"),
		modalDirectColumnWithVisibility("if_name", "ifName", "if_name", "text", "expanded"),
		modalDirectColumnWithVisibility("if_descr", "ifDescr", "if_descr", "text", "expanded"),
		modalDirectColumnWithVisibility("if_alias", "Alias", "if_alias", "text", "expanded"),
		modalDirectColumnWithVisibility("port_id", "Source Port ID", "port_id", "text", "expanded"),
		modalDirectColumnWithVisibility("mac", "MAC", "mac", "text", "expanded"),
		modalDirectColumnWithVisibility("speed", "Speed", "speed", "number", "expanded"),
		modalDirectColumnWithVisibility("stp_state", "STP", "stp_state", "badge", "expanded"),
		modalDirectColumnWithVisibility("duplex", "Duplex", "duplex", "badge", "expanded"),
		modalDirectColumnWithVisibility("link_mode_confidence", "Mode Confidence", "link_mode_confidence", "badge", "expanded"),
		modalDirectColumnWithVisibility("topology_role_confidence", "Role Confidence", "topology_role_confidence", "badge", "expanded"),
		modalDirectColumnWithVisibility("link_mode_sources", "Mode Sources", "link_mode_sources", "array_count", "expanded"),
		modalDirectColumnWithVisibility("topology_role_sources", "Role Sources", "topology_role_sources", "array_count", "expanded"),
		modalDirectColumnWithVisibility("last_change", "Last Change", "last_change", "text", "expanded"),
		modalDirectColumnWithVisibility("neighbors", "Neighbor Data", "neighbors", "debug_json", "debug"),
		modalDirectColumnWithVisibility("vlans", "VLAN Data", "vlans", "debug_json", "debug"),
	}
}

func snmpTopologyV1ActorPortLinksTableType() topologyapi.TableType {
	return topologyapi.TableType{
		Role:        "actor_detail",
		Owner:       "actor",
		Aggregation: "append",
		Columns:     snmpTopologyV1ActorPortLinksColumns(),
		Presentation: &topologyapi.TableTypePresentation{
			Label:   "Port Neighbors",
			Order:   2,
			Columns: snmpTopologyV1PortLinkModalColumns(),
		},
	}
}

func snmpTopologyV1PortLinkModalColumns() []topologyapi.ModalColumn {
	return []topologyapi.ModalColumn{
		modalDirectColumn("if_index", "Port ID", "if_index", "number"),
		modalDirectColumn("port_name", "Port", "port_name", "text"),
		modalActorRefColumn("remote_actor", "Remote Actor", "remote_actor"),
		modalDirectColumn("remote_port_name", "Remote Port", "remote_port_name", "text"),
		modalDirectColumn("type", "Type", "type", "badge"),
		modalDirectColumn("state", "State", "state", "badge"),
		modalDirectColumn("evidence_count", "Evidence", "evidence_count", "number"),
		modalDirectColumnWithVisibility("protocol", "Protocol", "protocol", "badge", "expanded"),
		modalDirectColumnWithVisibility("remote_if_index", "Remote Port ID", "remote_if_index", "number", "expanded"),
		modalDirectColumnWithVisibility("port_id", "Source Port ID", "port_id", "text", "expanded"),
		modalDirectColumnWithVisibility("remote_port_id", "Remote Source Port ID", "remote_port_id", "text", "expanded"),
		modalDirectColumnWithVisibility("confidence", "Confidence", "confidence", "badge", "expanded"),
		modalDirectColumnWithVisibility("inference", "Inference", "inference", "badge", "expanded"),
		modalDirectColumnWithVisibility("attachment_mode", "Attachment", "attachment_mode", "badge", "expanded"),
		modalDirectColumnWithVisibility("discovered_at", "Discovered", "discovered_at", "timestamp", "expanded"),
		modalDirectColumnWithVisibility("last_seen", "Last Seen", "last_seen", "timestamp", "expanded"),
	}
}

func snmpTopologyV1ActorLabelsTableType() topologyapi.TableType {
	return topologyapi.TableType{
		Role:        "actor_inventory",
		Owner:       "actor",
		Aggregation: "set",
		Columns: []topologyapi.Column{
			topologyapi.NewColumn("actor", "actor_ref", topologyapi.WithRole("reference")),
			topologyapi.NewColumn("key", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithRole("attribute")),
			topologyapi.NewColumn("value", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithRole("attribute")),
			topologyapi.NewColumn("source", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable(), topologyapi.WithRole("attribute")),
			topologyapi.NewColumn("kind", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable(), topologyapi.WithRole("attribute")),
			topologyapi.NewColumn("value_index", "uint", topologyapi.WithNullable(), topologyapi.WithRole("attribute")),
		},
		Presentation: &topologyapi.TableTypePresentation{
			Label: "Labels",
			Order: 0,
			Columns: []topologyapi.ModalColumn{
				modalDirectColumn("key", "Label", "key", "text"),
				modalDirectColumn("value", "Value", "value", "text"),
				modalDirectColumn("source", "Source", "source", "badge"),
				modalDirectColumn("kind", "Kind", "kind", "badge"),
			},
		},
	}
}

func snmpTopologyV1OSPFNeighborsTableType() topologyapi.TableType {
	return topologyapi.TableType{
		Role:        "actor_detail",
		Owner:       "actor",
		Aggregation: "append",
		Columns:     snmpTopologyV1OSPFNeighborsColumns(),
		Presentation: &topologyapi.TableTypePresentation{
			Label:   "OSPF Neighbors",
			Order:   4,
			Columns: snmpTopologyV1OSPFNeighborModalColumns(),
		},
	}
}

func snmpTopologyV1BGPPeersTableType() topologyapi.TableType {
	return topologyapi.TableType{
		Role:        "actor_detail",
		Owner:       "actor",
		Aggregation: "append",
		Columns:     snmpTopologyV1BGPPeersColumns(),
		Presentation: &topologyapi.TableTypePresentation{
			Label:   "BGP Peers",
			Order:   5,
			Columns: snmpTopologyV1BGPPeerModalColumns(),
		},
	}
}

func snmpTopologyV1BGPPeerModalColumns() []topologyapi.ModalColumn {
	return []topologyapi.ModalColumn{
		modalDirectColumn("neighbor_ip", "Neighbor IP", "neighbor_ip", "text"),
		modalDirectColumn("remote_as", "Remote AS", "remote_as", "text"),
		modalDirectColumn("state", "State", "state", "badge"),
		modalActorRefColumn("remote_actor", "Remote Actor", "remote_actor"),
		modalDirectColumnWithVisibility("routing_instance", "Routing Instance", "routing_instance", "text", "expanded"),
		modalDirectColumnWithVisibility("admin_status", "Admin", "admin_status", "badge", "expanded"),
		modalDirectColumnWithVisibility("local_ip", "Local IP", "local_ip", "text", "expanded"),
		modalDirectColumnWithVisibility("local_as", "Local AS", "local_as", "text", "expanded"),
		modalDirectColumnWithVisibility("local_identifier", "Local Identifier", "local_identifier", "text", "expanded"),
		modalDirectColumnWithVisibility("peer_identifier", "Peer Identifier", "peer_identifier", "text", "expanded"),
		modalDirectColumnWithVisibility("peer_type", "Peer Type", "peer_type", "badge", "expanded"),
		modalDirectColumnWithVisibility("bgp_version", "BGP Version", "bgp_version", "text", "expanded"),
		modalDirectColumnWithVisibility("established_uptime", "Established Uptime", "established_uptime", "duration", "expanded"),
		modalDirectColumnWithVisibility("last_received_update_age", "Last Update Age", "last_received_update_age", "duration", "expanded"),
		modalDirectColumnWithVisibility("description", "Description", "description", "text", "expanded"),
		modalDirectColumnWithVisibility("source", "Source", "source", "badge", "expanded"),
	}
}

func snmpTopologyV1OSPFNeighborModalColumns() []topologyapi.ModalColumn {
	return []topologyapi.ModalColumn{
		modalDirectColumn("neighbor_router_id", "Neighbor Router ID", "neighbor_router_id", "text"),
		modalDirectColumn("neighbor_ip", "Neighbor IP", "neighbor_ip", "text"),
		modalDirectColumn("state", "State", "state", "badge"),
		modalActorRefColumn("remote_actor", "Remote Actor", "remote_actor"),
		modalDirectColumnWithVisibility("local_router_id", "Local Router ID", "local_router_id", "text", "expanded"),
		modalDirectColumnWithVisibility("local_ip", "Local IP", "local_ip", "text", "expanded"),
		modalDirectColumnWithVisibility("subnet", "Subnet", "subnet", "text", "expanded"),
		modalDirectColumnWithVisibility("addressless_index", "Addressless Index", "addressless_index", "text", "expanded"),
		modalDirectColumnWithVisibility("source", "Source", "source", "badge", "expanded"),
	}
}

func snmpTopologyV1ActorPortsColumns() []topologyapi.Column {
	return []topologyapi.Column{
		topologyapi.NewColumn("actor", "actor_ref", topologyapi.WithRole("reference")),
		topologyapi.NewColumn("if_index", "uint", topologyapi.WithNullable()),
		topologyapi.NewColumn("port_id", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("name", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("if_name", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("if_descr", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("if_alias", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("mac", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("speed", "uint", topologyapi.WithNullable()),
		topologyapi.NewColumn("topology_role", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("oper_status", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("admin_status", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("port_type", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("link_mode", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("stp_state", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("vlan_ids", "array", topologyapi.WithNullable()),
		topologyapi.NewColumn("fdb_mac_count", "uint", topologyapi.WithNullable()),
		topologyapi.NewColumn("link_count", "uint", topologyapi.WithNullable()),
		topologyapi.NewColumn("neighbor_count", "uint", topologyapi.WithNullable()),
		topologyapi.NewColumn("neighbor_actor", "actor_ref", topologyapi.WithNullable(), topologyapi.WithRole("reference")),
		topologyapi.NewColumn("neighbor_port_name", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("neighbors", "json", topologyapi.WithNullable()),
		topologyapi.NewColumn("vlans", "json", topologyapi.WithNullable()),
		topologyapi.NewColumn("duplex", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("link_mode_confidence", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("topology_role_confidence", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("link_mode_sources", "array", topologyapi.WithNullable()),
		topologyapi.NewColumn("topology_role_sources", "array", topologyapi.WithNullable()),
		topologyapi.NewColumn("last_change", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
	}
}

func snmpTopologyV1ActorPortLinksColumns() []topologyapi.Column {
	return []topologyapi.Column{
		topologyapi.NewColumn("actor", "actor_ref", topologyapi.WithRole("reference")),
		topologyapi.NewColumn("link", "link_ref", topologyapi.WithRole("reference")),
		topologyapi.NewColumn("remote_actor", "actor_ref", topologyapi.WithRole("reference")),
		topologyapi.NewColumn("if_index", "uint", topologyapi.WithNullable()),
		topologyapi.NewColumn("port_id", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("port_name", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("remote_if_index", "uint", topologyapi.WithNullable()),
		topologyapi.NewColumn("remote_port_id", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("remote_port_name", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("type", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithRole("group_key")),
		topologyapi.NewColumn("protocol", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithRole("group_key")),
		topologyapi.NewColumn("state", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("evidence_count", "uint", topologyapi.WithAggregation("sum")),
		topologyapi.NewColumn("confidence", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("inference", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("attachment_mode", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("discovered_at", "timestamp", topologyapi.WithNullable(), topologyapi.WithRole("timestamp")),
		topologyapi.NewColumn("last_seen", "timestamp", topologyapi.WithNullable(), topologyapi.WithRole("timestamp")),
	}
}

func snmpTopologyV1OSPFNeighborsColumns() []topologyapi.Column {
	return []topologyapi.Column{
		topologyapi.NewColumn("actor", "actor_ref", topologyapi.WithRole("reference")),
		topologyapi.NewColumn("remote_actor", "actor_ref", topologyapi.WithNullable(), topologyapi.WithRole("reference")),
		topologyapi.NewColumn("local_router_id", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("neighbor_router_id", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("neighbor_ip", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("state", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("local_ip", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("subnet", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("addressless_index", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("source", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
	}
}

func snmpTopologyV1BGPPeersColumns() []topologyapi.Column {
	return []topologyapi.Column{
		topologyapi.NewColumn("actor", "actor_ref", topologyapi.WithRole("reference")),
		topologyapi.NewColumn("remote_actor", "actor_ref", topologyapi.WithNullable(), topologyapi.WithRole("reference")),
		topologyapi.NewColumn("routing_instance", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("neighbor_ip", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("remote_as", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("state", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("admin_status", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("local_ip", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("local_as", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("local_identifier", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("peer_identifier", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("peer_type", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("bgp_version", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("description", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
		topologyapi.NewColumn("established_uptime", "uint", topologyapi.WithNullable()),
		topologyapi.NewColumn("last_received_update_age", "uint", topologyapi.WithNullable()),
		topologyapi.NewColumn("source", "string_ref", topologyapi.WithNullable(), topologyapi.WithDictionary("strings")),
	}
}
