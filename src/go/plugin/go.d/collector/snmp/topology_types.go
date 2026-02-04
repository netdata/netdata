// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import "time"

const topologySchemaVersion = "1.0"

type topologyData struct {
	SchemaVersion string           `json:"schema_version"`
	AgentID       string           `json:"agent_id"`
	CollectedAt   time.Time        `json:"collected_at"`
	Devices       []topologyDevice `json:"devices"`
	Links         []topologyLink   `json:"links"`
	Stats         map[string]any   `json:"stats,omitempty"`
	Metrics       map[string]any   `json:"metrics,omitempty"`
}

type topologyManagementAddress struct {
	Address     string `json:"address"`
	AddressType string `json:"address_type,omitempty"`
	IfSubtype   string `json:"if_subtype,omitempty"`
	IfID        string `json:"if_id,omitempty"`
	OID         string `json:"oid,omitempty"`
	Source      string `json:"source,omitempty"`
}

type topologyDevice struct {
	ChassisID             string                      `json:"chassis_id"`
	ChassisIDType         string                      `json:"chassis_id_type"`
	SysObjectID           string                      `json:"sys_object_id,omitempty"`
	SysName               string                      `json:"sys_name,omitempty"`
	SysDescr              string                      `json:"sys_descr,omitempty"`
	SysLocation           string                      `json:"sys_location,omitempty"`
	ManagementIP          string                      `json:"management_ip,omitempty"`
	ManagementAddresses   []topologyManagementAddress `json:"management_addresses,omitempty"`
	AgentID               string                      `json:"agent_id,omitempty"`
	AgentJobID            string                      `json:"agent_job_id,omitempty"`
	Vendor                string                      `json:"vendor,omitempty"`
	Model                 string                      `json:"model,omitempty"`
	Capabilities          []string                    `json:"capabilities,omitempty"`
	CapabilitiesSupported []string                    `json:"capabilities_supported,omitempty"`
	CapabilitiesEnabled   []string                    `json:"capabilities_enabled,omitempty"`
	Labels                map[string]string           `json:"labels,omitempty"`
	Discovered            bool                        `json:"discovered,omitempty"`
}

type topologyEndpoint struct {
	ChassisID           string                      `json:"chassis_id"`
	ChassisIDType       string                      `json:"chassis_id_type"`
	IfIndex             int                         `json:"if_index,omitempty"`
	IfName              string                      `json:"if_name,omitempty"`
	PortID              string                      `json:"port_id,omitempty"`
	PortIDType          string                      `json:"port_id_type,omitempty"`
	PortDescr           string                      `json:"port_descr,omitempty"`
	SysName             string                      `json:"sys_name,omitempty"`
	ManagementIP        string                      `json:"management_ip,omitempty"`
	ManagementAddresses []topologyManagementAddress `json:"management_addresses,omitempty"`
	AgentID             string                      `json:"agent_id,omitempty"`
	Labels              map[string]string           `json:"labels,omitempty"`
}

type topologyLink struct {
	Protocol      string           `json:"protocol"`
	Src           topologyEndpoint `json:"src"`
	Dst           topologyEndpoint `json:"dst"`
	DiscoveredAt  time.Time        `json:"discovered_at,omitempty"`
	LastSeen      time.Time        `json:"last_seen,omitempty"`
	Bidirectional bool             `json:"bidirectional,omitempty"`
	Validated     bool             `json:"validated,omitempty"`
}
