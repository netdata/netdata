// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"

func snmpTopologyV1ActorPortsTableType() topologyv1.TableType {
	return topologyv1.TableType{
		Role:        "actor_detail",
		Owner:       "actor",
		Aggregation: "append",
		Columns:     snmpTopologyV1ActorPortsColumns(),
		Presentation: &topologyv1.TableTypePresentation{
			Label:   "Ports",
			Order:   1,
			Columns: snmpTopologyV1PortModalColumns(),
		},
	}
}

func snmpTopologyV1PortModalColumns() []topologyv1.ModalColumn {
	return []topologyv1.ModalColumn{
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
		modalDirectColumnWithVisibility("neighbors", "Neighbor Data", "neighbors", "debug_json", "debug"),
		modalDirectColumnWithVisibility("vlans", "VLAN Data", "vlans", "debug_json", "debug"),
		modalDirectColumnWithVisibility("extra", "Extra", "extra", "debug_json", "debug"),
	}
}

func snmpTopologyV1ActorPortLinksTableType() topologyv1.TableType {
	return topologyv1.TableType{
		Role:        "actor_detail",
		Owner:       "actor",
		Aggregation: "append",
		Columns:     snmpTopologyV1ActorPortLinksColumns(),
		Presentation: &topologyv1.TableTypePresentation{
			Label:   "Port Neighbors",
			Order:   2,
			Columns: snmpTopologyV1PortLinkModalColumns(),
		},
	}
}

func snmpTopologyV1PortLinkModalColumns() []topologyv1.ModalColumn {
	return []topologyv1.ModalColumn{
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

func snmpTopologyV1ActorLabelsTableType() topologyv1.TableType {
	return topologyv1.TableType{
		Role:        "actor_inventory",
		Owner:       "actor",
		Aggregation: "set",
		Columns: []topologyv1.Column{
			topologyv1.NewColumn("actor", "actor_ref", topologyv1.WithRole("reference")),
			topologyv1.NewColumn("key", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("attribute")),
			topologyv1.NewColumn("value", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("attribute")),
			topologyv1.NewColumn("source", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
			topologyv1.NewColumn("kind", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
			topologyv1.NewColumn("value_index", "uint", topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		},
		Presentation: &topologyv1.TableTypePresentation{
			Label: "Labels",
			Order: 0,
			Columns: []topologyv1.ModalColumn{
				modalDirectColumn("key", "Label", "key", "text"),
				modalDirectColumn("value", "Value", "value", "text"),
				modalDirectColumn("source", "Source", "source", "badge"),
				modalDirectColumn("kind", "Kind", "kind", "badge"),
			},
		},
	}
}

func snmpTopologyV1OSPFNeighborsTableType() topologyv1.TableType {
	return topologyv1.TableType{
		Role:        "actor_detail",
		Owner:       "actor",
		Aggregation: "append",
		Columns:     snmpTopologyV1OSPFNeighborsColumns(),
		Presentation: &topologyv1.TableTypePresentation{
			Label:   "OSPF Neighbors",
			Order:   4,
			Columns: snmpTopologyV1OSPFNeighborModalColumns(),
		},
	}
}

func snmpTopologyV1OSPFNeighborModalColumns() []topologyv1.ModalColumn {
	return []topologyv1.ModalColumn{
		modalDirectColumn("neighbor_router_id", "Neighbor Router ID", "neighbor_router_id", "text"),
		modalDirectColumn("neighbor_ip", "Neighbor IP", "neighbor_ip", "text"),
		modalDirectColumn("state", "State", "state", "badge"),
		modalActorRefColumnWithVisibility("remote_actor", "Remote Actor", "remote_actor", "expanded"),
		modalDirectColumnWithVisibility("local_router_id", "Local Router ID", "local_router_id", "text", "expanded"),
		modalDirectColumnWithVisibility("local_ip", "Local IP", "local_ip", "text", "expanded"),
		modalDirectColumnWithVisibility("subnet", "Subnet", "subnet", "text", "expanded"),
		modalDirectColumnWithVisibility("addressless_index", "Addressless Index", "addressless_index", "text", "expanded"),
		modalDirectColumnWithVisibility("source", "Source", "source", "badge", "expanded"),
	}
}

func snmpTopologyV1ActorPortsColumns() []topologyv1.Column {
	return []topologyv1.Column{
		topologyv1.NewColumn("actor", "actor_ref", topologyv1.WithRole("reference")),
		topologyv1.NewColumn("if_index", "uint", topologyv1.WithNullable()),
		topologyv1.NewColumn("port_id", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("name", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("if_name", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("if_descr", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("if_alias", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("mac", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("speed", "uint", topologyv1.WithNullable()),
		topologyv1.NewColumn("topology_role", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("oper_status", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("admin_status", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("port_type", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("link_mode", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("stp_state", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("vlan_ids", "array", topologyv1.WithNullable()),
		topologyv1.NewColumn("fdb_mac_count", "uint", topologyv1.WithNullable()),
		topologyv1.NewColumn("link_count", "uint", topologyv1.WithNullable()),
		topologyv1.NewColumn("neighbor_count", "uint", topologyv1.WithNullable()),
		topologyv1.NewColumn("neighbor_actor", "actor_ref", topologyv1.WithNullable(), topologyv1.WithRole("reference")),
		topologyv1.NewColumn("neighbor_port_name", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("neighbors", "json", topologyv1.WithNullable()),
		topologyv1.NewColumn("vlans", "json", topologyv1.WithNullable()),
		topologyv1.NewColumn("extra", "json", topologyv1.WithNullable()),
	}
}

func snmpTopologyV1ActorPortLinksColumns() []topologyv1.Column {
	return []topologyv1.Column{
		topologyv1.NewColumn("actor", "actor_ref", topologyv1.WithRole("reference")),
		topologyv1.NewColumn("link", "link_ref", topologyv1.WithRole("reference")),
		topologyv1.NewColumn("remote_actor", "actor_ref", topologyv1.WithRole("reference")),
		topologyv1.NewColumn("if_index", "uint", topologyv1.WithNullable()),
		topologyv1.NewColumn("port_id", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("port_name", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("remote_if_index", "uint", topologyv1.WithNullable()),
		topologyv1.NewColumn("remote_port_id", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("remote_port_name", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("type", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("group_key")),
		topologyv1.NewColumn("protocol", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("group_key")),
		topologyv1.NewColumn("state", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("evidence_count", "uint", topologyv1.WithAggregation("sum")),
		topologyv1.NewColumn("confidence", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("inference", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("attachment_mode", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("discovered_at", "timestamp", topologyv1.WithNullable(), topologyv1.WithRole("timestamp")),
		topologyv1.NewColumn("last_seen", "timestamp", topologyv1.WithNullable(), topologyv1.WithRole("timestamp")),
	}
}

func snmpTopologyV1OSPFNeighborsColumns() []topologyv1.Column {
	return []topologyv1.Column{
		topologyv1.NewColumn("actor", "actor_ref", topologyv1.WithRole("reference")),
		topologyv1.NewColumn("remote_actor", "actor_ref", topologyv1.WithNullable(), topologyv1.WithRole("reference")),
		topologyv1.NewColumn("local_router_id", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("neighbor_router_id", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("neighbor_ip", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("state", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("local_ip", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("subnet", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("addressless_index", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("source", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
	}
}
