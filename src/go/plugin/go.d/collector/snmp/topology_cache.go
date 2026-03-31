// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
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

func newTopologyCache() *topologyCache {
	return &topologyCache{
		lldpLocPorts:    make(map[string]*lldpLocPort),
		lldpRemotes:     make(map[string]*lldpRemote),
		cdpRemotes:      make(map[string]*cdpRemote),
		ifNamesByIndex:  make(map[string]string),
		ifStatusByIndex: make(map[string]ifStatus),
		ifIndexByIP:     make(map[string]string),
		ifNetmaskByIP:   make(map[string]string),
		bridgePortToIf:  make(map[string]string),
		fdbEntries:      make(map[string]*fdbEntry),
		fdbIDToVlanID:   make(map[string]string),
		vlanIDToName:    make(map[string]string),
		stpPorts:        make(map[string]*stpPortEntry),
		arpEntries:      make(map[string]*arpEntry),
	}
}

func (c *Collector) resetTopologyCache() {
	if c.topologyCache == nil {
		return
	}

	c.topologyCache.mu.Lock()
	defer c.topologyCache.mu.Unlock()

	c.topologyCache.updateTime = time.Now()
	c.topologyCache.lastUpdate = time.Time{}
	c.topologyCache.staleAfter = c.topologyStaleAfter()
	c.topologyCache.agentID = resolveTopologyAgentID(c)
	c.topologyCache.localDevice = buildLocalTopologyDevice(c)

	c.topologyCache.lldpLocPorts = make(map[string]*lldpLocPort)
	c.topologyCache.lldpRemotes = make(map[string]*lldpRemote)
	c.topologyCache.cdpRemotes = make(map[string]*cdpRemote)
	c.topologyCache.ifNamesByIndex = make(map[string]string)
	c.topologyCache.ifStatusByIndex = make(map[string]ifStatus)
	c.topologyCache.ifIndexByIP = make(map[string]string)
	c.topologyCache.ifNetmaskByIP = make(map[string]string)
	c.topologyCache.bridgePortToIf = make(map[string]string)
	c.topologyCache.fdbEntries = make(map[string]*fdbEntry)
	c.topologyCache.fdbIDToVlanID = make(map[string]string)
	c.topologyCache.vlanIDToName = make(map[string]string)
	c.topologyCache.vtpVersion = ""
	c.topologyCache.stpBaseBridgeAddress = ""
	c.topologyCache.stpDesignatedRoot = ""
	c.topologyCache.stpPorts = make(map[string]*stpPortEntry)
	c.topologyCache.arpEntries = make(map[string]*arpEntry)
}

func (c *topologyCache) replaceWith(src *topologyCache) {
	if c == nil || src == nil {
		return
	}

	c.lastUpdate = src.lastUpdate
	c.updateTime = src.updateTime
	c.staleAfter = src.staleAfter
	c.agentID = src.agentID
	c.localDevice = src.localDevice
	c.lldpLocPorts = src.lldpLocPorts
	c.lldpRemotes = src.lldpRemotes
	c.cdpRemotes = src.cdpRemotes
	c.ifNamesByIndex = src.ifNamesByIndex
	c.ifStatusByIndex = src.ifStatusByIndex
	c.ifIndexByIP = src.ifIndexByIP
	c.ifNetmaskByIP = src.ifNetmaskByIP
	c.bridgePortToIf = src.bridgePortToIf
	c.fdbEntries = src.fdbEntries
	c.fdbIDToVlanID = src.fdbIDToVlanID
	c.vlanIDToName = src.vlanIDToName
	c.vtpVersion = src.vtpVersion
	c.stpBaseBridgeAddress = src.stpBaseBridgeAddress
	c.stpDesignatedRoot = src.stpDesignatedRoot
	c.stpPorts = src.stpPorts
	c.arpEntries = src.arpEntries
}

func (c *topologyCache) hasFreshSnapshotAt(now time.Time) bool {
	if c == nil || c.lastUpdate.IsZero() {
		return false
	}
	if c.staleAfter > 0 && now.After(c.lastUpdate.Add(c.staleAfter)) {
		return false
	}
	return true
}

func (c *Collector) updateTopologyProfileTags(pms []*ddsnmp.ProfileMetrics) {
	if c.topologyCache == nil {
		return
	}

	c.topologyCache.mu.Lock()
	defer c.topologyCache.mu.Unlock()

	for _, pm := range pms {
		if len(pm.Tags) == 0 {
			continue
		}

		if v := pm.Tags[tagLldpLocChassisID]; v != "" && strings.TrimSpace(c.topologyCache.localDevice.ChassisID) == "" {
			c.topologyCache.localDevice.ChassisID = v
		}
		if v := pm.Tags[tagLldpLocChassisIDSubtype]; v != "" && strings.TrimSpace(c.topologyCache.localDevice.ChassisIDType) == "" {
			c.topologyCache.localDevice.ChassisIDType = normalizeLLDPSubtype(v, lldpChassisIDSubtypeMap)
		}
		if v := pm.Tags[tagLldpLocSysName]; v != "" && strings.TrimSpace(c.topologyCache.localDevice.SysName) == "" {
			c.topologyCache.localDevice.SysName = v
		}
		if v := pm.Tags[tagLldpLocSysDesc]; v != "" && strings.TrimSpace(c.topologyCache.localDevice.SysDescr) == "" {
			c.topologyCache.localDevice.SysDescr = v
		}
		if v := pm.Tags[tagLldpLocSysCapSupported]; v != "" {
			c.topologyCache.localDevice.Labels = ensureLabels(c.topologyCache.localDevice.Labels)
			c.topologyCache.localDevice.Labels[tagLldpLocSysCapSupported] = v
			caps := decodeLLDPCapabilities(v)
			if len(caps) > 0 {
				c.topologyCache.localDevice.CapabilitiesSupported = caps
			}
		}
		if v := pm.Tags[tagLldpLocSysCapEnabled]; v != "" {
			c.topologyCache.localDevice.Labels = ensureLabels(c.topologyCache.localDevice.Labels)
			c.topologyCache.localDevice.Labels[tagLldpLocSysCapEnabled] = v
			caps := decodeLLDPCapabilities(v)
			if len(caps) > 0 {
				c.topologyCache.localDevice.CapabilitiesEnabled = caps
				if len(c.topologyCache.localDevice.Capabilities) == 0 {
					c.topologyCache.localDevice.Capabilities = caps
				}
			}
		}
		c.topologyCache.updateLocalBridgeIdentityFromTags(pm.Tags)
		if v := stpBridgeAddressToMAC(pm.Tags[tagStpDesignatedRoot]); v != "" {
			c.topologyCache.stpDesignatedRoot = v
		}
		if v := strings.TrimSpace(pm.Tags[tagVtpVersion]); v != "" {
			c.topologyCache.vtpVersion = v
		}
	}
}

func (c *topologyCache) applyAuthoritativeBridgeIdentity(mac string) {
	mac = normalizeMAC(mac)
	if mac == "" || mac == "00:00:00:00:00:00" {
		return
	}
	c.stpBaseBridgeAddress = mac
	// SNMP bridge identity is authoritative for managed device identity.
	c.localDevice.ChassisID = mac
	c.localDevice.ChassisIDType = "macAddress"
}

func (c *topologyCache) updateLocalBridgeIdentityFromTags(tags map[string]string) {
	if c == nil || len(tags) == 0 {
		return
	}
	if v := stpBridgeAddressToMAC(firstNonEmpty(tags[tagBridgeBaseAddress], tags[tagLegacyStpBaseBridgeAddr])); v != "" {
		c.applyAuthoritativeBridgeIdentity(v)
	}
}

func (c *Collector) updateTopologyCacheEntry(m ddsnmp.Metric) {
	if c.topologyCache == nil {
		return
	}

	c.topologyCache.mu.Lock()
	defer c.topologyCache.mu.Unlock()

	switch m.Name {
	case metricLldpLocPortEntry:
		c.topologyCache.updateLldpLocPort(m.Tags)
	case metricLldpLocManAddrEntry:
		c.topologyCache.updateLldpLocManAddr(m.Tags)
	case metricLldpRemEntry:
		c.topologyCache.updateLldpRemote(m.Tags)
	case metricLldpRemManAddrEntry, metricLldpRemManAddrCompat:
		c.topologyCache.updateLldpRemManAddr(m.Tags)
	case metricCdpCacheEntry:
		c.topologyCache.updateCdpRemote(m.Tags)
	case metricTopologyIfNameEntry:
		c.topologyCache.updateIfNameByIndex(m.Tags)
	case metricTopologyIfStatusEntry:
		c.topologyCache.updateIfNameByIndex(m.Tags)
	case metricTopologyIfDuplexEntry:
		c.topologyCache.updateIfNameByIndex(m.Tags)
	case metricTopologyIPIfEntry:
		c.topologyCache.updateIfIndexByIP(m.Tags)
	case metricBridgePortMapEntry:
		c.topologyCache.updateBridgePortMap(m.Tags)
	case metricFdbEntry, metricDot1qFdbEntry:
		c.topologyCache.updateFdbEntry(m.Tags)
	case metricDot1qVlanEntry:
		c.topologyCache.updateDot1qVlanMap(m.Tags)
	case metricStpPortEntry:
		c.topologyCache.updateStpPortEntry(m.Tags)
	case metricVtpVlanEntry:
		c.topologyCache.updateVtpVlanEntry(m.Tags)
	case metricArpEntry, metricArpLegacyEntry:
		c.topologyCache.updateArpEntry(m.Tags)
	}
}

func (c *Collector) finalizeTopologyCache() {
	if c.topologyCache == nil {
		return
	}

	c.topologyCache.mu.Lock()
	defer c.topologyCache.mu.Unlock()

	c.topologyCache.lastUpdate = c.topologyCache.updateTime
}

func (c *Collector) syncTopologyChartReferences() {
	if c == nil || c.topologyCache == nil {
		return
	}

	deviceCharts := c.collectLocalDeviceCharts()
	interfaceCharts := c.collectLocalInterfaceCharts()

	c.topologyCache.mu.Lock()
	defer c.topologyCache.mu.Unlock()

	local := c.topologyCache.localDevice
	if local.ChartIDPrefix == "" {
		local.ChartIDPrefix = topologyProfileChartIDPrefix
	}
	if local.ChartContextPrefix == "" {
		local.ChartContextPrefix = topologyProfileChartContextPrefix
	}
	if c.vnode != nil && strings.TrimSpace(c.vnode.GUID) != "" {
		local.NetdataHostID = strings.TrimSpace(c.vnode.GUID)
	}
	local.DeviceCharts = deviceCharts
	local.InterfaceCharts = interfaceCharts
	c.topologyCache.localDevice = local
}

func (c *Collector) collectLocalDeviceCharts() map[string]string {
	if c == nil || c.charts == nil {
		return nil
	}

	out := make(map[string]string)
	staticCharts := map[string]string{
		"ping_rtt":         "ping_rtt",
		"ping_rtt_stddev":  "ping_rtt_stddev",
		"topology_devices": "topology_devices",
		"topology_links":   "topology_links",
	}
	for semantic, chartID := range staticCharts {
		if c.chartExists(chartID) {
			out[semantic] = chartID
		}
	}
	for metricName := range c.seenScalarMetrics {
		metricName = strings.TrimSpace(metricName)
		if metricName == "" {
			continue
		}
		chartID := topologyProfileChartIDPrefix + cleanMetricName.Replace(metricName)
		if c.chartExists(chartID) {
			out[metricName] = chartID
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (c *Collector) updateTopologyScalarMetric(m ddsnmp.Metric) {
	if c == nil || c.topologyCache == nil {
		return
	}
	if !isTopologySysUptimeMetric(m.Name) || m.Value <= 0 {
		return
	}

	c.topologyCache.mu.Lock()
	defer c.topologyCache.mu.Unlock()

	local := c.topologyCache.localDevice
	local.SysUptime = m.Value
	local.Labels = ensureLabels(local.Labels)
	setTopologyMetadataLabelIfMissing(local.Labels, "sys_uptime", strconv.FormatInt(m.Value, 10))
	c.topologyCache.localDevice = local
}

func isTopologySysUptimeMetric(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "sysuptime", "systemuptime":
		return true
	default:
		return false
	}
}

func (c *Collector) collectLocalInterfaceCharts() map[string]topologyInterfaceChartRef {
	if c == nil || c.ifaceCache == nil {
		return nil
	}

	c.ifaceCache.mu.RLock()
	defer c.ifaceCache.mu.RUnlock()

	out := make(map[string]topologyInterfaceChartRef)
	for _, entry := range c.ifaceCache.interfaces {
		if entry == nil {
			continue
		}
		suffix := strings.TrimSpace(entry.name)
		if suffix == "" {
			continue
		}

		availableMetrics := make([]string, 0, len(entry.availableMetrics))
		for metricName := range entry.availableMetrics {
			metricName = strings.TrimSpace(metricName)
			if metricName == "" {
				continue
			}
			chartID := topologyProfileChartIDPrefix + cleanMetricName.Replace(metricName+"_"+suffix)
			if c.chartExists(chartID) {
				availableMetrics = append(availableMetrics, metricName)
			}
		}
		sort.Strings(availableMetrics)
		if len(availableMetrics) == 0 {
			continue
		}

		out[suffix] = topologyInterfaceChartRef{
			ChartIDSuffix:    suffix,
			AvailableMetrics: deduplicateSortedStrings(availableMetrics),
		}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func (c *Collector) chartExists(chartID string) bool {
	if c == nil || c.charts == nil {
		return false
	}
	chart := c.charts.Get(strings.TrimSpace(chartID))
	return chart != nil && !chart.Obsolete
}

func isTopologyMetric(name string) bool {
	switch name {
	case metricLldpLocPortEntry, metricLldpLocManAddrEntry, metricLldpRemEntry, metricLldpRemManAddrEntry, metricLldpRemManAddrCompat, metricCdpCacheEntry,
		metricTopologyIfNameEntry, metricTopologyIfStatusEntry, metricTopologyIfDuplexEntry, metricTopologyIPIfEntry, metricBridgePortMapEntry, metricFdbEntry, metricDot1qFdbEntry, metricDot1qVlanEntry, metricStpPortEntry, metricVtpVlanEntry, metricArpEntry, metricArpLegacyEntry:
		return true
	default:
		return false
	}
}

func resolveTopologyAgentID(c *Collector) string {
	if c.vnode != nil && c.vnode.GUID != "" {
		return c.vnode.GUID
	}
	if c.Hostname != "" {
		return c.Hostname
	}
	return ""
}
