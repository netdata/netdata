// SPDX-License-Identifier: GPL-3.0-or-later

package graph

import "time"

type Match struct {
	ChassisIDs         []string `json:"chassis_ids,omitempty"`
	MacAddresses       []string `json:"mac_addresses,omitempty"`
	IPAddresses        []string `json:"ip_addresses,omitempty"`
	Hostnames          []string `json:"hostnames,omitempty"`
	DNSNames           []string `json:"dns_names,omitempty"`
	SysObjectID        string   `json:"sys_object_id,omitempty"`
	SysName            string   `json:"sys_name,omitempty"`
	NetdataNodeID      string   `json:"netdata_node_id,omitempty"`
	NetdataMachineGUID string   `json:"netdata_machine_guid,omitempty"`
	CloudInstanceID    string   `json:"cloud_instance_id,omitempty"`
	CloudAccountID     string   `json:"cloud_account_id,omitempty"`
	ContainerIDs       []string `json:"container_ids,omitempty"`
	PodNames           []string `json:"pod_names,omitempty"`
	NamespaceIDs       []string `json:"namespace_ids,omitempty"`
}

type Actor struct {
	ActorID     string            `json:"actor_id,omitempty"`
	ActorType   string            `json:"actor_type"`
	Layer       string            `json:"layer"`
	Source      string            `json:"source"`
	Match       Match             `json:"match"`
	ParentMatch *Match            `json:"parent_match,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type LinkEndpoint struct {
	Match         Match  `json:"match"`
	IfIndex       int    `json:"if_index,omitempty"`
	IfName        string `json:"if_name,omitempty"`
	IfDescr       string `json:"if_descr,omitempty"`
	IfAlias       string `json:"if_alias,omitempty"`
	PortID        string `json:"port_id,omitempty"`
	PortName      string `json:"port_name,omitempty"`
	BridgePort    string `json:"bridge_port,omitempty"`
	SysName       string `json:"sys_name,omitempty"`
	ManagementIP  string `json:"management_ip,omitempty"`
	DisplayName   string `json:"display_name,omitempty"`
	DisplaySource string `json:"display_source,omitempty"`
	AdminStatus   string `json:"if_admin_status,omitempty"`
	OperStatus    string `json:"if_oper_status,omitempty"`
}

type LinkDisplay struct {
	Name        string `json:"name,omitempty"`
	SrcPortName string `json:"src_port_name,omitempty"`
	DstPortName string `json:"dst_port_name,omitempty"`
}

type LinkL2 struct {
	BridgeDomain   string `json:"bridge_domain,omitempty"`
	Designated     bool   `json:"designated,omitempty"`
	PairID         string `json:"pair_id,omitempty"`
	PairPass       string `json:"pair_pass,omitempty"`
	PairConsistent bool   `json:"pair_consistent,omitempty"`
}

type LinkInference struct {
	Inference      string `json:"inference,omitempty"`
	Confidence     string `json:"confidence,omitempty"`
	AttachmentMode string `json:"attachment_mode,omitempty"`
}

type Link struct {
	Layer        string         `json:"layer"`
	Protocol     string         `json:"protocol"`
	LinkType     string         `json:"link_type"`
	Direction    string         `json:"direction,omitempty"`
	State        string         `json:"state,omitempty"`
	SrcActorID   string         `json:"src_actor_id,omitempty"`
	DstActorID   string         `json:"dst_actor_id,omitempty"`
	Src          LinkEndpoint   `json:"src"`
	Dst          LinkEndpoint   `json:"dst"`
	DiscoveredAt *time.Time     `json:"discovered_at,omitempty"`
	LastSeen     *time.Time     `json:"last_seen,omitempty"`
	Display      *LinkDisplay   `json:"display,omitempty"`
	L2           *LinkL2        `json:"l2,omitempty"`
	Inference    *LinkInference `json:"inference,omitempty"`
}

type Graph struct {
	SchemaVersion string    `json:"schema_version"`
	Source        string    `json:"source,omitempty"`
	Layer         string    `json:"layer,omitempty"`
	AgentID       string    `json:"agent_id"`
	CollectedAt   time.Time `json:"collected_at"`
	View          string    `json:"view,omitempty"`
	Actors        []Actor   `json:"actors,omitempty"`
	Links         []Link    `json:"links,omitempty"`
}
