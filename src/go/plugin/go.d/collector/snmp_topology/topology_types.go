// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "github.com/netdata/netdata/go/plugins/pkg/topology"

const topologySchemaVersion = "2.0"

type topologyData = topology.Data
type topologyActor = topology.Actor
type topologyMatch = topology.Match
type topologyLink = topology.Link
type topologyLinkEndpoint = topology.LinkEndpoint
type topologyFlow = topology.Flow
type topologyIPPolicy = topology.IPPolicy

type topologyManagementAddress struct {
	Address     string `json:"address"`
	AddressType string `json:"address_type,omitempty"`
	IfSubtype   string `json:"if_subtype,omitempty"`
	IfID        string `json:"if_id,omitempty"`
	OID         string `json:"oid,omitempty"`
	Source      string `json:"source,omitempty"`
}

type topologyInterfaceChartRef struct {
	ChartIDSuffix    string   `json:"chart_id_suffix,omitempty"`
	AvailableMetrics []string `json:"available_metrics,omitempty"`
}

type topologyDevice struct {
	ChassisID             string                               `json:"chassis_id"`
	ChassisIDType         string                               `json:"chassis_id_type"`
	SysObjectID           string                               `json:"sys_object_id,omitempty"`
	SysName               string                               `json:"sys_name,omitempty"`
	SysDescr              string                               `json:"sys_descr,omitempty"`
	SysContact            string                               `json:"sys_contact,omitempty"`
	SysLocation           string                               `json:"sys_location,omitempty"`
	SysUptime             int64                                `json:"sys_uptime,omitempty"`
	SerialNumber          string                               `json:"serial_number,omitempty"`
	SoftwareVersion       string                               `json:"software_version,omitempty"`
	FirmwareVersion       string                               `json:"firmware_version,omitempty"`
	HardwareVersion       string                               `json:"hardware_version,omitempty"`
	ManagementIP          string                               `json:"management_ip,omitempty"`
	ManagementAddresses   []topologyManagementAddress          `json:"management_addresses,omitempty"`
	AgentID               string                               `json:"agent_id,omitempty"`
	AgentJobID            string                               `json:"agent_job_id,omitempty"`
	NetdataHostID         string                               `json:"netdata_host_id,omitempty"`
	ChartIDPrefix         string                               `json:"chart_id_prefix,omitempty"`
	ChartContextPrefix    string                               `json:"chart_context_prefix,omitempty"`
	DeviceCharts          map[string]string                    `json:"device_charts,omitempty"`
	InterfaceCharts       map[string]topologyInterfaceChartRef `json:"interface_charts,omitempty"`
	Vendor                string                               `json:"vendor,omitempty"`
	Model                 string                               `json:"model,omitempty"`
	Capabilities          []string                             `json:"capabilities,omitempty"`
	CapabilitiesSupported []string                             `json:"capabilities_supported,omitempty"`
	CapabilitiesEnabled   []string                             `json:"capabilities_enabled,omitempty"`
	Labels                map[string]string                    `json:"labels,omitempty"`
	Discovered            bool                                 `json:"discovered,omitempty"`
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
