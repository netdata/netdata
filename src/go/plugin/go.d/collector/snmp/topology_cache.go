// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"sync"
	"time"
)

const (
	metricLldpLocPortEntry      = "lldpLocPortEntry"
	metricLldpLocManAddrEntry   = "lldpLocManAddrEntry"
	metricLldpRemEntry          = "lldpRemEntry"
	metricLldpRemManAddrEntry   = "lldpRemManAddrEntry"
	metricLldpRemManAddrCompat  = "lldpRemManAddrCompatEntry"
	metricCdpCacheEntry         = "cdpCacheEntry"
	metricTopologyIfNameEntry   = "topologyIfNameEntry"
	metricTopologyIfStatusEntry = "topologyIfStatusEntry"
	metricTopologyIfDuplexEntry = "topologyIfDuplexEntry"
	metricTopologyIPIfEntry     = "topologyIpIfIndexEntry"
	metricBridgePortMapEntry    = "dot1dBasePortIfIndexEntry"
	metricFdbEntry              = "dot1dTpFdbEntry"
	metricDot1qFdbEntry         = "dot1qTpFdbEntry"
	metricDot1qVlanEntry        = "dot1qVlanCurrentEntry"
	metricStpPortEntry          = "dot1dStpPortEntry"
	metricVtpVlanEntry          = "vtpVlanEntry"
	metricArpEntry              = "ipNetToPhysicalEntry"
	metricArpLegacyEntry        = "ipNetToMediaEntry"
)

const (
	tagLldpLocChassisID        = "lldp_loc_chassis_id"
	tagLldpLocChassisIDSubtype = "lldp_loc_chassis_id_subtype"
	tagLldpLocSysName          = "lldp_loc_sys_name"
	tagLldpLocSysDesc          = "lldp_loc_sys_desc"
	tagLldpLocSysCapSupported  = "lldp_loc_sys_cap_supported"
	tagLldpLocSysCapEnabled    = "lldp_loc_sys_cap_enabled"

	tagLldpLocMgmtAddrSubtype   = "lldp_loc_mgmt_addr_subtype"
	tagLldpLocMgmtAddr          = "lldp_loc_mgmt_addr"
	tagLldpLocMgmtAddrLen       = "lldp_loc_mgmt_addr_len"
	tagLldpLocMgmtAddrIfSubtype = "lldp_loc_mgmt_addr_if_subtype"
	tagLldpLocMgmtAddrIfID      = "lldp_loc_mgmt_addr_if_id"
	tagLldpLocMgmtAddrOID       = "lldp_loc_mgmt_addr_oid"

	tagLldpLocPortNum       = "lldp_loc_port_num"
	tagLldpLocPortID        = "lldp_loc_port_id"
	tagLldpLocPortIDSubtype = "lldp_loc_port_id_subtype"
	tagLldpLocPortDesc      = "lldp_loc_port_desc"

	tagLldpRemIndex            = "lldp_rem_index"
	tagLldpRemChassisID        = "lldp_rem_chassis_id"
	tagLldpRemChassisIDSubtype = "lldp_rem_chassis_id_subtype"
	tagLldpRemPortID           = "lldp_rem_port_id"
	tagLldpRemPortIDSubtype    = "lldp_rem_port_id_subtype"
	tagLldpRemPortDesc         = "lldp_rem_port_desc"
	tagLldpRemSysName          = "lldp_rem_sys_name"
	tagLldpRemSysDesc          = "lldp_rem_sys_desc"
	tagLldpRemMgmtAddr         = "lldp_rem_mgmt_addr"
	tagLldpRemSysCapSupported  = "lldp_rem_sys_cap_supported"
	tagLldpRemSysCapEnabled    = "lldp_rem_sys_cap_enabled"

	tagLldpRemMgmtAddrSubtype   = "lldp_rem_mgmt_addr_subtype"
	tagLldpRemMgmtAddrLen       = "lldp_rem_mgmt_addr_len"
	tagLldpRemMgmtAddrOctetPref = "lldp_rem_mgmt_addr_octet_"
	tagLldpRemMgmtAddrIfSubtype = "lldp_rem_mgmt_addr_if_subtype"
	tagLldpRemMgmtAddrIfID      = "lldp_rem_mgmt_addr_if_id"
	tagLldpRemMgmtAddrOID       = "lldp_rem_mgmt_addr_oid"

	tagCdpIfIndex               = "cdp_if_index"
	tagCdpIfName                = "cdp_if_name"
	tagCdpDeviceIndex           = "cdp_device_index"
	tagCdpDeviceID              = "cdp_device_id"
	tagCdpAddressType           = "cdp_address_type"
	tagCdpDevicePort            = "cdp_device_port"
	tagCdpVersion               = "cdp_version"
	tagCdpPlatform              = "cdp_platform"
	tagCdpCaps                  = "cdp_capabilities"
	tagCdpAddress               = "cdp_address"
	tagCdpVTPDomain             = "cdp_vtp_mgmt_domain"
	tagCdpNativeVLAN            = "cdp_native_vlan"
	tagCdpDuplex                = "cdp_duplex"
	tagCdpPower                 = "cdp_power_consumption"
	tagCdpMTU                   = "cdp_mtu"
	tagCdpSysName               = "cdp_sys_name"
	tagCdpSysObjectID           = "cdp_sys_object_id"
	tagCdpPrimaryMgmtAddrType   = "cdp_primary_mgmt_addr_type"
	tagCdpPrimaryMgmtAddr       = "cdp_primary_mgmt_addr"
	tagCdpSecondaryMgmtAddrType = "cdp_secondary_mgmt_addr_type"
	tagCdpSecondaryMgmtAddr     = "cdp_secondary_mgmt_addr"
	tagCdpPhysicalLocation      = "cdp_physical_location"
	tagCdpLastChange            = "cdp_last_change"

	tagTopoIfIndex  = "topo_if_index"
	tagTopoIfName   = "topo_if_name"
	tagTopoIfType   = "topo_if_type"
	tagTopoIfAdmin  = "topo_if_admin_status"
	tagTopoIfOper   = "topo_if_oper_status"
	tagTopoIfPhys   = "topo_if_phys_address"
	tagTopoIfDescr  = "topo_if_descr"
	tagTopoIfAlias  = "topo_if_alias"
	tagTopoIfSpeed  = "topo_if_speed"
	tagTopoIfHigh   = "topo_if_high_speed"
	tagTopoIfLast   = "topo_if_last_change"
	tagTopoIfDuplex = "topo_if_duplex"
	tagTopoIPAddr   = "topo_ip_addr"
	tagTopoIPMask   = "topo_ip_netmask"

	tagBridgeBasePort = "bridge_base_port"
	tagBridgeIfIndex  = "bridge_if_index"

	tagFdbMac                  = "fdb_mac"
	tagFdbBridgePort           = "fdb_bridge_port"
	tagFdbStatus               = "fdb_status"
	tagDot1qFdbID              = "dot1q_fdb_id"
	tagDot1qFdbMac             = "dot1q_fdb_mac"
	tagDot1qFdbPort            = "dot1q_fdb_bridge_port"
	tagDot1qFdbStatus          = "dot1q_fdb_status"
	tagDot1qVlanID             = "dot1q_vlan_id"
	tagDot1qVlanID1            = "dot1q_vlan_id_idx1"
	tagDot1qVlanFdbID          = "dot1q_vlan_fdb_id"
	tagBridgeBaseAddress       = "bridge_base_address"
	tagLegacyStpBaseBridgeAddr = "stp_base_bridge_address"
	// Backward-compatibility alias for tests/older in-memory tag references.
	tagStpBaseBridgeAddress    = tagLegacyStpBaseBridgeAddr
	tagStpDesignatedRoot       = "stp_designated_root"
	tagStpPort                 = "stp_port"
	tagStpPortPriority         = "stp_port_priority"
	tagStpPortState            = "stp_port_state"
	tagStpPortEnable           = "stp_port_enable"
	tagStpPortPathCost         = "stp_port_path_cost"
	tagStpPortDesignatedRoot   = "stp_port_designated_root"
	tagStpPortDesignatedCost   = "stp_port_designated_cost"
	tagStpPortDesignatedBridge = "stp_port_designated_bridge"
	tagStpPortDesignatedPort   = "stp_port_designated_port"
	tagVtpVersion              = "vtp_version"
	tagVtpVlanIndex            = "vtp_vlan_index"
	tagVtpVlanState            = "vtp_vlan_state"
	tagVtpVlanType             = "vtp_vlan_type"
	tagVtpVlanName             = "vtp_vlan_name"

	tagArpIfIndex  = "arp_if_index"
	tagArpIfName   = "arp_if_name"
	tagArpIP       = "arp_ip"
	tagArpMac      = "arp_mac"
	tagArpType     = "arp_type"
	tagArpState    = "arp_state"
	tagArpAddrType = "arp_addr_type"

	// Internal collector tags used when ingesting additional VLAN-context snapshots.
	tagTopologyContextVLANID   = "_topology_context_vlan_id"
	tagTopologyContextVLANName = "_topology_context_vlan_name"
)

const (
	topologyProfileChartIDPrefix      = "snmp_device_prof_"
	topologyProfileChartContextPrefix = "snmp.device_prof_"
	topologyHighSpeedMultiplier       = int64(1_000_000)
)

var lldpChassisIDSubtypeMap = map[string]string{
	"1": "chassisComponent",
	"2": "interfaceAlias",
	"3": "portComponent",
	"4": "macAddress",
	"5": "networkAddress",
	"6": "interfaceName",
	"7": "local",
}

var lldpPortIDSubtypeMap = map[string]string{
	"1": "interfaceAlias",
	"2": "portComponent",
	"3": "macAddress",
	"4": "networkAddress",
	"5": "interfaceName",
	"6": "agentCircuitId",
	"7": "local",
}

var (
	topologyMetadataAliasSysDescr = []string{
		"description", "sys_descr", "sys_description",
	}
	topologyMetadataAliasSysContact = []string{
		"contact", "sys_contact",
	}
	topologyMetadataAliasSysLocation = []string{
		"location", "sys_location",
	}
	topologyMetadataAliasVendor = []string{
		"vendor", "manufacturer",
	}
	topologyMetadataAliasModel = []string{
		"model", "device_model",
	}
	topologyMetadataAliasSysUptime = []string{
		"sys_uptime", "sysuptime", "uptime",
	}
	topologyMetadataAliasSerial = []string{
		"serial_number", "serial", "serial_num", "serial_no", "serialnumber",
	}
	topologyMetadataAliasFirmware = []string{
		"firmware_version", "firmware", "firmware_rev", "firmware_revision",
	}
	topologyMetadataAliasSoftware = []string{
		"software_version", "software", "software_rev", "software_revision",
		"sw_version", "sw_rev", "version", "os_version",
	}
	topologyMetadataAliasHardware = []string{
		"hardware_version", "hardware", "hardware_rev", "hw_version", "hw_rev",
	}
)

type topologyCache struct {
	mu         sync.RWMutex
	lastUpdate time.Time
	updateTime time.Time
	staleAfter time.Duration

	agentID     string
	localDevice topologyDevice

	lldpLocPorts map[string]*lldpLocPort
	lldpRemotes  map[string]*lldpRemote
	cdpRemotes   map[string]*cdpRemote

	ifNamesByIndex       map[string]string
	ifStatusByIndex      map[string]ifStatus
	ifIndexByIP          map[string]string
	ifNetmaskByIP        map[string]string
	bridgePortToIf       map[string]string
	fdbEntries           map[string]*fdbEntry
	fdbIDToVlanID        map[string]string
	vlanIDToName         map[string]string
	vtpVersion           string
	stpBaseBridgeAddress string
	stpDesignatedRoot    string
	stpPorts             map[string]*stpPortEntry
	arpEntries           map[string]*arpEntry
}

type ifStatus struct {
	admin      string
	oper       string
	ifType     string
	ifDescr    string
	ifAlias    string
	mac        string
	speedBps   int64
	lastChange int64
	duplex     string
}

type lldpLocPort struct {
	portNum       string
	portID        string
	portIDSubtype string
	portDesc      string
}

type lldpRemote struct {
	localPortNum       string
	remIndex           string
	chassisID          string
	chassisIDSubtype   string
	portID             string
	portIDSubtype      string
	portDesc           string
	sysName            string
	sysDesc            string
	sysCapSupported    string
	sysCapEnabled      string
	managementAddr     string
	managementAddrType string
	managementAddrs    []topologyManagementAddress
}

type cdpRemote struct {
	ifIndex               string
	ifName                string
	deviceIndex           string
	deviceID              string
	devicePort            string
	platform              string
	capabilities          string
	addressType           string
	address               string
	version               string
	vtpMgmtDomain         string
	nativeVLAN            string
	duplex                string
	powerConsumption      string
	mtu                   string
	sysName               string
	sysObjectID           string
	primaryMgmtAddrType   string
	primaryMgmtAddr       string
	secondaryMgmtAddrType string
	secondaryMgmtAddr     string
	physicalLocation      string
	lastChange            string
	managementAddrs       []topologyManagementAddress
}

type fdbEntry struct {
	mac        string
	bridgePort string
	status     string
	fdbID      string
	vlanID     string
	vlanName   string
}

type stpPortEntry struct {
	port             string
	vlanID           string
	vlanName         string
	priority         string
	state            string
	enable           string
	pathCost         string
	designatedRoot   string
	designatedCost   string
	designatedBridge string
	designatedPort   string
}

type topologyVLANContext struct {
	vlanID   string
	vlanName string
}

type arpEntry struct {
	ifIndex  string
	ifName   string
	ip       string
	mac      string
	addrType string
	state    string
}
