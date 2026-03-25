// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"encoding/hex"
	"fmt"
	"math"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"
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

func buildLocalTopologyDevice(c *Collector) topologyDevice {
	device := topologyDevice{
		ManagementIP:       c.Hostname,
		ChartIDPrefix:      topologyProfileChartIDPrefix,
		ChartContextPrefix: topologyProfileChartContextPrefix,
	}

	if c.vnode != nil {
		device.AgentID = c.vnode.GUID
		device.NetdataHostID = c.vnode.GUID
		device.Labels = cloneTopologyLabels(c.vnode.Labels)
	}

	if c.sysInfo != nil {
		device.SysObjectID = c.sysInfo.SysObjectID
		device.SysName = c.sysInfo.Name
		device.SysDescr = c.sysInfo.Descr
		device.SysContact = c.sysInfo.Contact
		device.SysLocation = c.sysInfo.Location

		if c.sysInfo.Vendor != "" {
			device.Vendor = c.sysInfo.Vendor
		} else if c.sysInfo.Organization != "" {
			device.Vendor = c.sysInfo.Organization
		}
		device.Model = c.sysInfo.Model
	}

	if value := topologyMetadataValue(device.Labels, topologyMetadataAliasSysDescr); value != "" && device.SysDescr == "" {
		device.SysDescr = value
	}
	if value := topologyMetadataValue(device.Labels, topologyMetadataAliasSysContact); value != "" && device.SysContact == "" {
		device.SysContact = value
	}
	if value := topologyMetadataValue(device.Labels, topologyMetadataAliasSysLocation); value != "" && device.SysLocation == "" {
		device.SysLocation = value
	}
	if value := topologyMetadataValue(device.Labels, topologyMetadataAliasVendor); value != "" && device.Vendor == "" {
		device.Vendor = value
	}
	if value := topologyMetadataValue(device.Labels, topologyMetadataAliasModel); value != "" && device.Model == "" {
		device.Model = value
	}

	if value := topologyMetadataValue(device.Labels, topologyMetadataAliasSysUptime); value != "" {
		if uptime := parsePositiveInt64(value); uptime > 0 {
			device.SysUptime = uptime
		}
	}
	if value := topologyMetadataValue(device.Labels, topologyMetadataAliasSerial); value != "" {
		device.SerialNumber = value
		setTopologyMetadataLabelIfMissing(device.Labels, "serial_number", value)
	}
	if value := topologyMetadataValue(device.Labels, topologyMetadataAliasSoftware); value != "" {
		device.SoftwareVersion = value
		setTopologyMetadataLabelIfMissing(device.Labels, "software_version", value)
	}
	if value := topologyMetadataValue(device.Labels, topologyMetadataAliasFirmware); value != "" {
		device.FirmwareVersion = value
		setTopologyMetadataLabelIfMissing(device.Labels, "firmware_version", value)
	}
	if value := topologyMetadataValue(device.Labels, topologyMetadataAliasHardware); value != "" {
		device.HardwareVersion = value
		setTopologyMetadataLabelIfMissing(device.Labels, "hardware_version", value)
	}

	return device
}

func (c *topologyCache) updateLldpLocPort(tags map[string]string) {
	portNum := tags[tagLldpLocPortNum]
	if portNum == "" {
		return
	}

	entry := c.lldpLocPorts[portNum]
	if entry == nil {
		entry = &lldpLocPort{portNum: portNum}
		c.lldpLocPorts[portNum] = entry
	}

	if v := tags[tagLldpLocPortID]; v != "" {
		entry.portID = v
	}
	if v := tags[tagLldpLocPortIDSubtype]; v != "" {
		entry.portIDSubtype = normalizeLLDPSubtype(v, lldpPortIDSubtypeMap)
	}
	if v := tags[tagLldpLocPortDesc]; v != "" {
		entry.portDesc = v
	}
}

func (c *topologyCache) updateLldpLocManAddr(tags map[string]string) {
	addrHex := tags[tagLldpLocMgmtAddr]
	if addrHex == "" {
		return
	}

	addr, addrType := normalizeManagementAddress(addrHex, tags[tagLldpLocMgmtAddrSubtype])
	if addr == "" {
		return
	}

	mgmt := topologyManagementAddress{
		Address:     addr,
		AddressType: addrType,
		IfSubtype:   tags[tagLldpLocMgmtAddrIfSubtype],
		IfID:        tags[tagLldpLocMgmtAddrIfID],
		OID:         tags[tagLldpLocMgmtAddrOID],
		Source:      "lldp_local",
	}

	c.localDevice.ManagementAddresses = appendManagementAddress(c.localDevice.ManagementAddresses, mgmt)
}

func (c *topologyCache) updateLldpRemote(tags map[string]string) {
	localPort := tags[tagLldpLocPortNum]
	if localPort == "" {
		return
	}

	remIndex := tags[tagLldpRemIndex]
	key := localPort + ":" + remIndex

	entry := c.lldpRemotes[key]
	if entry == nil {
		entry = &lldpRemote{
			localPortNum: localPort,
			remIndex:     remIndex,
		}
		c.lldpRemotes[key] = entry
	}

	if v := tags[tagLldpRemChassisID]; v != "" {
		entry.chassisID = v
	}
	if v := tags[tagLldpRemChassisIDSubtype]; v != "" {
		entry.chassisIDSubtype = normalizeLLDPSubtype(v, lldpChassisIDSubtypeMap)
	}
	if v := tags[tagLldpRemPortID]; v != "" {
		entry.portID = v
	}
	if v := tags[tagLldpRemPortIDSubtype]; v != "" {
		entry.portIDSubtype = normalizeLLDPSubtype(v, lldpPortIDSubtypeMap)
	}
	if v := tags[tagLldpRemPortDesc]; v != "" {
		entry.portDesc = v
	}
	if v := tags[tagLldpRemSysName]; v != "" {
		entry.sysName = v
	}
	if v := tags[tagLldpRemSysDesc]; v != "" {
		entry.sysDesc = v
	}
	if v := tags[tagLldpRemSysCapSupported]; v != "" {
		entry.sysCapSupported = v
	}
	if v := tags[tagLldpRemSysCapEnabled]; v != "" {
		entry.sysCapEnabled = v
	}
	if v := tags[tagLldpRemMgmtAddr]; v != "" {
		entry.managementAddr = v
		addr, addrType := normalizeManagementAddress(v, tags[tagLldpRemMgmtAddrSubtype])
		if addr != "" {
			entry.managementAddrs = appendManagementAddress(entry.managementAddrs, topologyManagementAddress{
				Address:     addr,
				AddressType: addrType,
				Source:      "lldp_remote",
			})
		}
	}
}

func (c *topologyCache) updateLldpRemManAddr(tags map[string]string) {
	localPort := tags[tagLldpLocPortNum]
	if localPort == "" {
		return
	}

	remIndex := tags[tagLldpRemIndex]
	if remIndex == "" {
		return
	}

	key := localPort + ":" + remIndex
	entry := c.lldpRemotes[key]
	if entry == nil {
		entry = &lldpRemote{
			localPortNum: localPort,
			remIndex:     remIndex,
		}
		c.lldpRemotes[key] = entry
	}

	addrHex := tags[tagLldpRemMgmtAddr]
	if strings.TrimSpace(addrHex) == "" {
		addrHex = reconstructLldpRemMgmtAddrHex(tags)
	}
	addr, addrType := normalizeManagementAddress(addrHex, tags[tagLldpRemMgmtAddrSubtype])
	if addr == "" {
		return
	}

	mgmt := topologyManagementAddress{
		Address:     addr,
		AddressType: addrType,
		IfSubtype:   tags[tagLldpRemMgmtAddrIfSubtype],
		IfID:        tags[tagLldpRemMgmtAddrIfID],
		OID:         tags[tagLldpRemMgmtAddrOID],
		Source:      "lldp_remote",
	}
	entry.managementAddrs = appendManagementAddress(entry.managementAddrs, mgmt)
}

func (c *topologyCache) updateCdpRemote(tags map[string]string) {
	ifIndex := tags[tagCdpIfIndex]
	if ifIndex == "" {
		return
	}

	deviceIndex := tags[tagCdpDeviceIndex]
	key := ifIndex + ":" + deviceIndex

	entry := c.cdpRemotes[key]
	if entry == nil {
		entry = &cdpRemote{
			ifIndex:     ifIndex,
			deviceIndex: deviceIndex,
		}
		c.cdpRemotes[key] = entry
	}

	if v := tags[tagCdpIfName]; v != "" {
		entry.ifName = v
	}
	if v := tags[tagCdpDeviceID]; v != "" {
		entry.deviceID = v
	}
	if v := tags[tagCdpAddressType]; v != "" {
		entry.addressType = v
	}
	if v := tags[tagCdpDevicePort]; v != "" {
		entry.devicePort = v
	}
	if v := tags[tagCdpVersion]; v != "" {
		entry.version = v
	}
	if v := tags[tagCdpPlatform]; v != "" {
		entry.platform = v
	}
	if v := tags[tagCdpCaps]; v != "" {
		entry.capabilities = v
	}
	if v := tags[tagCdpAddress]; v != "" {
		entry.address = v
	}
	if v := tags[tagCdpVTPDomain]; v != "" {
		entry.vtpMgmtDomain = v
	}
	if v := tags[tagCdpNativeVLAN]; v != "" {
		entry.nativeVLAN = v
	}
	if v := tags[tagCdpDuplex]; v != "" {
		entry.duplex = v
	}
	if v := tags[tagCdpPower]; v != "" {
		entry.powerConsumption = v
	}
	if v := tags[tagCdpMTU]; v != "" {
		entry.mtu = v
	}
	if v := tags[tagCdpSysName]; v != "" {
		entry.sysName = v
	}
	if v := tags[tagCdpSysObjectID]; v != "" {
		entry.sysObjectID = v
	}
	if v := tags[tagCdpPrimaryMgmtAddrType]; v != "" {
		entry.primaryMgmtAddrType = v
	}
	if v := tags[tagCdpPrimaryMgmtAddr]; v != "" {
		entry.primaryMgmtAddr = v
	}
	if v := tags[tagCdpSecondaryMgmtAddrType]; v != "" {
		entry.secondaryMgmtAddrType = v
	}
	if v := tags[tagCdpSecondaryMgmtAddr]; v != "" {
		entry.secondaryMgmtAddr = v
	}
	if v := tags[tagCdpPhysicalLocation]; v != "" {
		entry.physicalLocation = v
	}
	if v := tags[tagCdpLastChange]; v != "" {
		entry.lastChange = v
	}

	entry.managementAddrs = appendCdpManagementAddresses(entry, entry.managementAddrs)
}

func (c *topologyCache) updateIfNameByIndex(tags map[string]string) {
	ifIndex := strings.TrimSpace(tags[tagTopoIfIndex])
	if ifIndex == "" {
		return
	}

	ifName := strings.TrimSpace(tags[tagTopoIfName])
	if ifName != "" {
		c.ifNamesByIndex[ifIndex] = ifName
	}

	status := c.ifStatusByIndex[ifIndex]
	if ifType := normalizeInterfaceType(tags[tagTopoIfType]); ifType != "" {
		status.ifType = ifType
	}
	if admin := normalizeInterfaceAdminStatus(tags[tagTopoIfAdmin]); admin != "" {
		status.admin = admin
	}
	if oper := normalizeInterfaceOperStatus(tags[tagTopoIfOper]); oper != "" {
		status.oper = oper
	}
	if ifDescr := strings.TrimSpace(tags[tagTopoIfDescr]); ifDescr != "" {
		status.ifDescr = ifDescr
	}
	if ifAlias := strings.TrimSpace(tags[tagTopoIfAlias]); ifAlias != "" {
		status.ifAlias = ifAlias
	}
	if mac := normalizeMAC(tags[tagTopoIfPhys]); mac != "" && mac != "00:00:00:00:00:00" {
		status.mac = mac
	}
	if ifHighSpeed := parsePositiveInt64(tags[tagTopoIfHigh]); ifHighSpeed > 0 {
		if ifHighSpeed > math.MaxInt64/topologyHighSpeedMultiplier {
			status.speedBps = math.MaxInt64
		} else {
			status.speedBps = ifHighSpeed * topologyHighSpeedMultiplier
		}
	} else if ifSpeed := parsePositiveInt64(tags[tagTopoIfSpeed]); ifSpeed > 0 {
		status.speedBps = ifSpeed
	}
	if lastChange := parsePositiveInt64(tags[tagTopoIfLast]); lastChange > 0 {
		status.lastChange = lastChange
	}
	if duplex := normalizeInterfaceDuplex(tags[tagTopoIfDuplex]); duplex != "" {
		status.duplex = duplex
	}
	if status.ifType != "" ||
		status.admin != "" ||
		status.oper != "" ||
		status.ifDescr != "" ||
		status.ifAlias != "" ||
		status.mac != "" ||
		status.speedBps > 0 ||
		status.lastChange > 0 ||
		status.duplex != "" {
		c.ifStatusByIndex[ifIndex] = status
	}
}

func (c *topologyCache) updateIfIndexByIP(tags map[string]string) {
	ifIndex := strings.TrimSpace(tags[tagTopoIfIndex])
	if ifIndex == "" {
		return
	}

	ip := normalizeIPAddress(tags[tagTopoIPAddr])
	if ip == "" {
		return
	}

	c.ifIndexByIP[ip] = ifIndex
	c.localDevice.ManagementAddresses = appendManagementAddress(c.localDevice.ManagementAddresses, topologyManagementAddress{
		Address:     ip,
		AddressType: managementAddressTypeFromIP(ip),
		Source:      "ip_mib",
	})
	if mask := normalizeIPAddress(tags[tagTopoIPMask]); mask != "" {
		c.ifNetmaskByIP[ip] = mask
	}
}

func (c *topologyCache) updateBridgePortMap(tags map[string]string) {
	c.updateLocalBridgeIdentityFromTags(tags)

	basePort := strings.TrimSpace(tags[tagBridgeBasePort])
	if basePort == "" {
		return
	}

	ifIndex := strings.TrimSpace(tags[tagBridgeIfIndex])
	if ifIndex == "" {
		return
	}

	c.bridgePortToIf[basePort] = ifIndex
}

func (c *topologyCache) updateFdbEntry(tags map[string]string) {
	c.updateLocalBridgeIdentityFromTags(tags)

	mac := normalizeMAC(firstNonEmpty(tags[tagFdbMac], tags[tagDot1qFdbMac]))
	if mac == "" {
		return
	}

	bridgePort := strings.TrimSpace(firstNonEmpty(tags[tagFdbBridgePort], tags[tagDot1qFdbPort]))
	if bridgePort == "" || bridgePort == "0" {
		return
	}

	fdbID := strings.TrimSpace(tags[tagDot1qFdbID])
	contextVLANID := strings.TrimSpace(tags[tagTopologyContextVLANID])
	contextVLANName := strings.TrimSpace(tags[tagTopologyContextVLANName])
	key := strings.Join([]string{mac, bridgePort, strings.ToLower(fdbID), strings.ToLower(contextVLANID)}, "|")
	entry := c.fdbEntries[key]
	if entry == nil {
		entry = &fdbEntry{
			mac:        mac,
			bridgePort: bridgePort,
			fdbID:      fdbID,
		}
		c.fdbEntries[key] = entry
	}

	if v := strings.TrimSpace(firstNonEmpty(tags[tagFdbStatus], tags[tagDot1qFdbStatus])); v != "" {
		entry.status = v
	}
	if entry.vlanID == "" && contextVLANID != "" {
		entry.vlanID = contextVLANID
	}
	if entry.vlanName == "" && contextVLANName != "" {
		entry.vlanName = contextVLANName
	}
	if entry.fdbID == "" && fdbID != "" {
		entry.fdbID = fdbID
	}
	if entry.vlanID == "" && entry.fdbID != "" {
		if vlanID := strings.TrimSpace(c.fdbIDToVlanID[entry.fdbID]); vlanID != "" {
			entry.vlanID = vlanID
		}
	}
	if entry.vlanName == "" && entry.vlanID != "" {
		if vlanName := strings.TrimSpace(c.vlanIDToName[entry.vlanID]); vlanName != "" {
			entry.vlanName = vlanName
		}
	}
}

func (c *topologyCache) updateDot1qVlanMap(tags map[string]string) {
	fdbID := strings.TrimSpace(tags[tagDot1qVlanFdbID])
	if fdbID == "" {
		return
	}

	vlanID := strings.TrimSpace(tags[tagDot1qVlanID])
	if vlanID == "" {
		vlanID = strings.TrimSpace(tags[tagDot1qVlanID1])
	}
	if vlanID == "" {
		return
	}

	c.fdbIDToVlanID[fdbID] = vlanID
	for _, entry := range c.fdbEntries {
		if entry == nil || strings.TrimSpace(entry.fdbID) != fdbID {
			continue
		}
		if strings.TrimSpace(entry.vlanID) == "" {
			entry.vlanID = vlanID
		}
		if strings.TrimSpace(entry.vlanName) == "" {
			entry.vlanName = strings.TrimSpace(c.vlanIDToName[vlanID])
		}
	}
}

func (c *topologyCache) updateVtpVlanEntry(tags map[string]string) {
	vlanID := strings.TrimSpace(tags[tagVtpVlanIndex])
	vlanName := strings.TrimSpace(tags[tagVtpVlanName])
	if vlanID == "" || vlanName == "" {
		return
	}

	vlanType := strings.TrimSpace(tags[tagVtpVlanType])
	vlanState := strings.ToLower(strings.TrimSpace(tags[tagVtpVlanState]))
	if vlanState != "" && vlanState != "1" && vlanState != "operational" {
		return
	}
	if vlanType != "" && vlanType != "1" && strings.ToLower(vlanType) != "ethernet" {
		// Keep parity with Enlinkd collector behavior: keep Ethernet VLANs only.
		return
	}

	c.vlanIDToName[vlanID] = vlanName
	for _, entry := range c.fdbEntries {
		if entry == nil || strings.TrimSpace(entry.vlanID) != vlanID {
			continue
		}
		if strings.TrimSpace(entry.vlanName) == "" {
			entry.vlanName = vlanName
		}
	}
}

func (c *topologyCache) vtpVLANContexts() []topologyVLANContext {
	c.mu.RLock()
	defer c.mu.RUnlock()

	contexts := make([]topologyVLANContext, 0, len(c.vlanIDToName))
	for vlanID, vlanName := range c.vlanIDToName {
		id := strings.TrimSpace(vlanID)
		if id == "" {
			continue
		}
		if _, err := strconv.Atoi(id); err != nil {
			continue
		}
		contexts = append(contexts, topologyVLANContext{
			vlanID:   id,
			vlanName: strings.TrimSpace(vlanName),
		})
	}

	sort.Slice(contexts, func(i, j int) bool {
		left, leftErr := strconv.Atoi(contexts[i].vlanID)
		right, rightErr := strconv.Atoi(contexts[j].vlanID)
		if leftErr == nil && rightErr == nil && left != right {
			return left < right
		}
		if contexts[i].vlanID != contexts[j].vlanID {
			return contexts[i].vlanID < contexts[j].vlanID
		}
		return contexts[i].vlanName < contexts[j].vlanName
	})

	return contexts
}

func (c *topologyCache) updateStpPortEntry(tags map[string]string) {
	port := strings.TrimSpace(tags[tagStpPort])
	if port == "" {
		return
	}

	contextVLANID := strings.TrimSpace(tags[tagTopologyContextVLANID])
	stpPortKey := port
	if contextVLANID != "" {
		stpPortKey = port + "|vlan:" + strings.ToLower(contextVLANID)
	}
	entry := c.stpPorts[stpPortKey]
	if entry == nil {
		entry = &stpPortEntry{port: port}
		c.stpPorts[stpPortKey] = entry
	}
	if contextVLANID != "" {
		entry.vlanID = contextVLANID
	}
	if v := strings.TrimSpace(tags[tagTopologyContextVLANName]); v != "" {
		entry.vlanName = v
	}
	if v := strings.TrimSpace(tags[tagStpPortPriority]); v != "" {
		entry.priority = v
	}
	if v := strings.TrimSpace(tags[tagStpPortState]); v != "" {
		entry.state = v
	}
	if v := strings.TrimSpace(tags[tagStpPortEnable]); v != "" {
		entry.enable = v
	}
	if v := strings.TrimSpace(tags[tagStpPortPathCost]); v != "" {
		entry.pathCost = v
	}
	if v := stpBridgeAddressToMAC(tags[tagStpPortDesignatedRoot]); v != "" {
		entry.designatedRoot = v
	}
	if v := strings.TrimSpace(tags[tagStpPortDesignatedCost]); v != "" {
		entry.designatedCost = v
	}
	if v := stpBridgeAddressToMAC(tags[tagStpPortDesignatedBridge]); v != "" {
		entry.designatedBridge = v
	}
	if v := stpDesignatedPortString(tags[tagStpPortDesignatedPort]); v != "" {
		entry.designatedPort = v
	}
}

func (c *topologyCache) updateArpEntry(tags map[string]string) {
	ip := normalizeIPAddress(tags[tagArpIP])
	mac := normalizeMAC(tags[tagArpMac])
	if ip == "" && mac == "" {
		return
	}

	ifIndex := strings.TrimSpace(tags[tagArpIfIndex])
	ifName := strings.TrimSpace(tags[tagArpIfName])
	if ifName == "" && ifIndex != "" {
		ifName = c.ifNamesByIndex[ifIndex]
	}

	if ifIndex != "" && ifName != "" {
		c.ifNamesByIndex[ifIndex] = ifName
	}

	key := strings.Join([]string{ifIndex, ip, mac}, "|")
	entry := c.arpEntries[key]
	if entry == nil {
		entry = &arpEntry{
			ifIndex: ifIndex,
			ifName:  ifName,
			ip:      ip,
			mac:     mac,
		}
		c.arpEntries[key] = entry
	}

	if v := strings.TrimSpace(tags[tagArpState]); v != "" {
		entry.state = v
	}
	if v := strings.TrimSpace(tags[tagArpType]); v != "" && entry.state == "" {
		entry.state = v
	}
	if v := strings.TrimSpace(tags[tagArpAddrType]); v != "" {
		entry.addrType = v
	}
}

func (c *topologyCache) snapshot() (topologyData, bool) {
	if c.lastUpdate.IsZero() {
		return topologyData{}, false
	}

	local := c.localDevice
	local = normalizeTopologyDevice(local)

	observations, localDeviceID := c.buildEngineObservations(local)
	if len(observations) == 0 {
		return topologyData{}, false
	}

	result, err := topologyengine.BuildL2ResultFromObservations(observations, topologyengine.DiscoverOptions{
		EnableLLDP:   true,
		EnableCDP:    true,
		EnableBridge: true,
		EnableARP:    true,
	})
	if err != nil {
		return topologyData{}, false
	}

	data := topologyengine.ToTopologyData(result, topologyengine.TopologyDataOptions{
		SchemaVersion:  topologySchemaVersion,
		Source:         "snmp",
		Layer:          "2",
		View:           "summary",
		AgentID:        c.agentID,
		LocalDeviceID:  localDeviceID,
		CollectedAt:    c.lastUpdate,
		ResolveDNSName: resolveTopologyReverseDNSName,
	})

	augmentLocalActorFromCache(&data, local)
	return data, true
}

func (c *topologyCache) buildEngineObservations(local topologyDevice) ([]topologyengine.L2Observation, string) {
	localObservation := c.buildEngineObservation(local)
	localObservation.DeviceID = strings.TrimSpace(localObservation.DeviceID)
	if localObservation.DeviceID == "" {
		return nil, ""
	}

	localManagementIP := normalizeIPAddress(local.ManagementIP)
	if localManagementIP == "" {
		localManagementIP = pickManagementIP(local.ManagementAddresses)
	}
	localSysName := strings.TrimSpace(local.SysName)
	localGlobalID := strings.TrimSpace(localObservation.Hostname)
	if localGlobalID == "" {
		localGlobalID = localObservation.DeviceID
	}

	resolver := newTopologyObservationIdentityResolver(localObservation)
	remoteObservations := make(map[string]*topologyengine.L2Observation)
	remoteOrder := make([]string, 0, len(c.lldpRemotes)+len(c.cdpRemotes))
	remoteManagementByID := make(map[string]string)
	remoteChassisByID := make(map[string]string)

	updateRemoteIdentity := func(deviceID, managementIP, chassisID string) {
		deviceID = strings.TrimSpace(deviceID)
		if deviceID == "" {
			return
		}
		if managementIP = canonicalObservationIP(managementIP); managementIP != "" {
			if _, ok := remoteManagementByID[deviceID]; !ok {
				remoteManagementByID[deviceID] = managementIP
			}
		}
		if chassisID = strings.TrimSpace(chassisID); chassisID != "" {
			if _, ok := remoteChassisByID[deviceID]; !ok {
				remoteChassisByID[deviceID] = chassisID
			}
		}
	}

	selectHostname := func(current, candidate, deviceID string) string {
		current = strings.TrimSpace(current)
		candidate = strings.TrimSpace(candidate)
		deviceID = strings.TrimSpace(deviceID)
		if candidate == "" {
			if current != "" {
				return current
			}
			return deviceID
		}
		if current == "" || current == deviceID {
			return candidate
		}
		return current
	}

	ensureRemoteObservation := func(protocol, deviceID, hostname, managementIP, chassisID string) *topologyengine.L2Observation {
		deviceID = strings.TrimSpace(deviceID)
		if deviceID == "" {
			return nil
		}

		key := strings.Join([]string{
			protocol,
			deviceID,
		}, "|")

		entry := remoteObservations[key]
		if entry == nil {
			entry = &topologyengine.L2Observation{
				DeviceID: deviceID,
				Inferred: true,
			}
			remoteObservations[key] = entry
			remoteOrder = append(remoteOrder, key)
		}
		entry.Hostname = selectHostname(entry.Hostname, hostname, deviceID)

		updateRemoteIdentity(deviceID, managementIP, chassisID)
		if entry.ManagementIP == "" {
			entry.ManagementIP = remoteManagementByID[deviceID]
		}
		if entry.ChassisID == "" {
			entry.ChassisID = remoteChassisByID[deviceID]
		}
		resolver.register(deviceID, []string{entry.Hostname}, entry.ChassisID, entry.ManagementIP)
		return entry
	}

	lldpKeys := make([]string, 0, len(c.lldpRemotes))
	for key := range c.lldpRemotes {
		lldpKeys = append(lldpKeys, key)
	}
	sort.Strings(lldpKeys)
	for _, key := range lldpKeys {
		remote := c.lldpRemotes[key]
		if remote == nil {
			continue
		}

		remoteSysName := strings.TrimSpace(remote.sysName)
		remoteChassisID := strings.TrimSpace(remote.chassisID)
		remoteManagementIP := normalizeIPAddress(remote.managementAddr)
		if remoteManagementIP == "" {
			remoteManagementIP = pickManagementIP(remote.managementAddrs)
		}

		remoteDeviceID := resolver.resolve(
			[]string{remoteSysName},
			remoteChassisID,
			strings.TrimSpace(remote.chassisIDSubtype),
			remoteManagementIP,
		)
		if remoteDeviceID == "" || remoteDeviceID == localObservation.DeviceID {
			continue
		}
		updateRemoteIdentity(remoteDeviceID, remoteManagementIP, remoteChassisID)

		remoteObservation := ensureRemoteObservation(
			"lldp",
			remoteDeviceID,
			firstNonEmpty(remoteSysName, remoteDeviceID),
			remoteManagementIP,
			remoteChassisID,
		)
		if remoteObservation == nil {
			continue
		}

		localPort := c.lldpLocPorts[remote.localPortNum]
		localPortID := ""
		localPortIDSubtype := ""
		localPortDesc := ""
		if localPort != nil {
			localPortID = strings.TrimSpace(localPort.portID)
			localPortIDSubtype = strings.TrimSpace(localPort.portIDSubtype)
			localPortDesc = strings.TrimSpace(localPort.portDesc)
		}

		if strings.TrimSpace(remote.portID) == "" &&
			strings.TrimSpace(remote.portDesc) == "" &&
			localPortID == "" &&
			localPortDesc == "" {
			continue
		}

		remoteObservation.LLDPRemotes = append(remoteObservation.LLDPRemotes, topologyengine.LLDPRemoteObservation{
			LocalPortNum:       strings.TrimSpace(remote.remIndex),
			RemoteIndex:        strings.TrimSpace(remote.localPortNum),
			LocalPortID:        strings.TrimSpace(remote.portID),
			LocalPortIDSubtype: strings.TrimSpace(remote.portIDSubtype),
			LocalPortDesc:      strings.TrimSpace(remote.portDesc),
			ChassisID:          strings.TrimSpace(local.ChassisID),
			SysName:            localSysName,
			PortID:             localPortID,
			PortIDSubtype:      localPortIDSubtype,
			PortDesc:           localPortDesc,
			ManagementIP:       localManagementIP,
		})
	}

	cdpKeys := make([]string, 0, len(c.cdpRemotes))
	for key := range c.cdpRemotes {
		cdpKeys = append(cdpKeys, key)
	}
	sort.Strings(cdpKeys)
	for _, key := range cdpKeys {
		remote := c.cdpRemotes[key]
		if remote == nil {
			continue
		}

		remoteDeviceToken := strings.TrimSpace(remote.deviceID)
		remoteSysName := strings.TrimSpace(remote.sysName)
		remoteManagementIP := normalizeIPAddress(remote.address)
		if remoteManagementIP == "" {
			remoteManagementIP = pickManagementIP(remote.managementAddrs)
		}
		if remoteManagementIP == "" && remoteDeviceToken == "" && remoteSysName == "" {
			continue
		}

		remoteDeviceID := resolver.resolve(
			[]string{remoteDeviceToken, remoteSysName},
			"",
			"",
			remoteManagementIP,
		)
		if remoteDeviceID == "" || remoteDeviceID == localObservation.DeviceID {
			continue
		}
		updateRemoteIdentity(remoteDeviceID, remoteManagementIP, "")

		remoteIfName := strings.TrimSpace(remote.devicePort)
		localIfName := strings.TrimSpace(remote.ifName)
		if localIfName == "" && strings.TrimSpace(remote.ifIndex) != "" {
			localIfName = strings.TrimSpace(c.ifNamesByIndex[remote.ifIndex])
		}
		if remoteIfName == "" || localIfName == "" {
			continue
		}

		remoteObservation := ensureRemoteObservation(
			"cdp",
			remoteDeviceID,
			firstNonEmpty(remoteDeviceToken, remoteSysName, remoteDeviceID),
			remoteManagementIP,
			"",
		)
		if remoteObservation == nil {
			continue
		}

		remoteObservation.CDPRemotes = append(remoteObservation.CDPRemotes, topologyengine.CDPRemoteObservation{
			LocalIfName: remoteIfName,
			DeviceID:    localGlobalID,
			SysName:     localSysName,
			DevicePort:  localIfName,
			Address:     localManagementIP,
		})
	}

	observations := make([]topologyengine.L2Observation, 0, 1+len(remoteObservations))
	observations = append(observations, localObservation)
	sort.Strings(remoteOrder)
	for _, key := range remoteOrder {
		entry := remoteObservations[key]
		if entry == nil {
			continue
		}
		if entry.ManagementIP == "" {
			entry.ManagementIP = remoteManagementByID[entry.DeviceID]
		}
		if entry.ChassisID == "" {
			entry.ChassisID = remoteChassisByID[entry.DeviceID]
		}
		if len(entry.LLDPRemotes) == 0 && len(entry.CDPRemotes) == 0 {
			continue
		}
		if entry.Hostname == "" {
			entry.Hostname = entry.DeviceID
		}
		observations = append(observations, *entry)
	}

	return observations, localObservation.DeviceID
}

type topologyObservationIdentityResolver struct {
	hostToID    map[string]string
	chassisToID map[string]string
	macToID     map[string]string
	ipToID      map[string]string
	fallbackSeq int
}

func newTopologyObservationIdentityResolver(local topologyengine.L2Observation) *topologyObservationIdentityResolver {
	resolver := &topologyObservationIdentityResolver{
		hostToID:    make(map[string]string),
		chassisToID: make(map[string]string),
		macToID:     make(map[string]string),
		ipToID:      make(map[string]string),
	}
	resolver.register(local.DeviceID, []string{local.Hostname}, local.ChassisID, local.ManagementIP)
	return resolver
}

func (r *topologyObservationIdentityResolver) resolve(hostAliases []string, chassisID, chassisType, managementIP string) string {
	if mac := canonicalObservationMAC(chassisID); mac != "" {
		if id := r.macToID[mac]; id != "" {
			return id
		}

		candidate := normalizeTopologyDevice(topologyDevice{
			ChassisID:     mac,
			ChassisIDType: "macAddress",
			SysName:       firstNonEmpty(hostAliases...),
			ManagementIP:  normalizeIPAddress(managementIP),
		})
		id := strings.TrimSpace(ensureTopologyObservationDeviceID(candidate, ""))
		if id == "" || id == "local-device" {
			r.fallbackSeq++
			id = fmt.Sprintf("remote-device-%d", r.fallbackSeq)
		}
		r.register(id, hostAliases, mac, managementIP)
		return id
	}

	for _, host := range hostAliases {
		if id := r.hostToID[canonicalObservationHost(host)]; id != "" {
			return id
		}
	}
	if id := r.chassisToID[canonicalObservationChassis(chassisID)]; id != "" {
		return id
	}
	if id := r.ipToID[canonicalObservationIP(managementIP)]; id != "" {
		return id
	}

	candidate := normalizeTopologyDevice(topologyDevice{
		ChassisID:     strings.TrimSpace(chassisID),
		ChassisIDType: strings.TrimSpace(chassisType),
		SysName:       firstNonEmpty(hostAliases...),
		ManagementIP:  normalizeIPAddress(managementIP),
	})
	id := strings.TrimSpace(ensureTopologyObservationDeviceID(candidate, ""))
	if id == "" || id == "local-device" {
		r.fallbackSeq++
		id = fmt.Sprintf("remote-device-%d", r.fallbackSeq)
	}
	r.register(id, hostAliases, chassisID, managementIP)
	return id
}

func (r *topologyObservationIdentityResolver) register(id string, hostAliases []string, chassisID, managementIP string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	for _, host := range hostAliases {
		if key := canonicalObservationHost(host); key != "" {
			if _, exists := r.hostToID[key]; !exists {
				r.hostToID[key] = id
			}
		}
	}
	if key := canonicalObservationChassis(chassisID); key != "" {
		if _, exists := r.chassisToID[key]; !exists {
			r.chassisToID[key] = id
		}
	}
	if mac := canonicalObservationMAC(chassisID); mac != "" {
		if _, exists := r.macToID[mac]; !exists {
			r.macToID[mac] = id
		}
	}
	if key := canonicalObservationIP(managementIP); key != "" {
		if _, exists := r.ipToID[key]; !exists {
			r.ipToID[key] = id
		}
	}
}

func canonicalObservationHost(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func canonicalObservationChassis(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if mac := normalizeMAC(value); mac != "" {
		return mac
	}
	return strings.ToLower(value)
}

func canonicalObservationMAC(value string) string {
	if mac := normalizeMAC(value); mac != "" {
		return mac
	}
	return ""
}

func canonicalObservationIP(value string) string {
	value = normalizeIPAddress(value)
	if value != "" {
		return strings.ToLower(value)
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func (c *topologyCache) buildEngineObservation(local topologyDevice) topologyengine.L2Observation {
	localManagementIP := normalizeIPAddress(local.ManagementIP)
	if localManagementIP == "" {
		localManagementIP = pickManagementIP(local.ManagementAddresses)
	}
	baseBridgeAddress := strings.TrimSpace(c.stpBaseBridgeAddress)
	if baseBridgeAddress == "" {
		// Some devices do not expose LLDP local chassis identity or STP base tags
		// through profile-level tags. Derive stable bridge MAC from explicit FDB self rows.
		baseBridgeAddress = c.deriveLocalBridgeMACFromFDBSelfEntries()
	}
	if baseBridgeAddress == "" {
		// Last-resort fallback for managed devices without bridge-base/STP/FDB-self:
		// infer a stable chassis MAC from IF-MIB physical interface addresses.
		baseBridgeAddress = c.deriveLocalBridgeMACFromInterfacePhysAddress(localManagementIP)
	}
	if baseBridgeAddress != "" && normalizeMAC(local.ChassisID) == "" {
		local.ChassisID = baseBridgeAddress
		local.ChassisIDType = "macAddress"
	}

	observation := topologyengine.L2Observation{
		DeviceID:          ensureTopologyObservationDeviceID(local, baseBridgeAddress),
		Hostname:          strings.TrimSpace(local.SysName),
		ManagementIP:      localManagementIP,
		SysObjectID:       strings.TrimSpace(local.SysObjectID),
		ChassisID:         strings.TrimSpace(local.ChassisID),
		BaseBridgeAddress: baseBridgeAddress,
	}
	if observation.BaseBridgeAddress == "" {
		observation.BaseBridgeAddress = stpBridgeAddressToMAC(observation.ChassisID)
	}

	ifaceKeys := make(map[string]struct{}, len(c.ifNamesByIndex)+len(c.ifStatusByIndex))
	for key := range c.ifNamesByIndex {
		ifaceKeys[key] = struct{}{}
	}
	for key := range c.ifStatusByIndex {
		ifaceKeys[key] = struct{}{}
	}
	ifaceKeyList := make([]string, 0, len(ifaceKeys))
	for key := range ifaceKeys {
		ifaceKeyList = append(ifaceKeyList, key)
	}
	sort.Strings(ifaceKeyList)
	for _, ifIndex := range ifaceKeyList {
		idx := parseIndex(ifIndex)
		if idx <= 0 {
			continue
		}
		ifName := strings.TrimSpace(c.ifNamesByIndex[ifIndex])
		if ifName == "" {
			// `ifIndex` is the minimum required identity for interface inventory.
			ifName = ifIndex
		}
		status := c.ifStatusByIndex[ifIndex]
		ifDescr := strings.TrimSpace(status.ifDescr)
		if ifDescr == "" {
			ifDescr = ifName
		}
		observation.Interfaces = append(observation.Interfaces, topologyengine.ObservedInterface{
			IfIndex:       idx,
			IfName:        ifName,
			IfDescr:       ifDescr,
			IfAlias:       strings.TrimSpace(status.ifAlias),
			MAC:           strings.TrimSpace(status.mac),
			SpeedBps:      status.speedBps,
			LastChange:    status.lastChange,
			Duplex:        strings.TrimSpace(status.duplex),
			InterfaceType: strings.TrimSpace(status.ifType),
			AdminStatus:   strings.TrimSpace(status.admin),
			OperStatus:    strings.TrimSpace(status.oper),
		})
	}

	bridgePortKeys := make([]string, 0, len(c.bridgePortToIf))
	for key := range c.bridgePortToIf {
		bridgePortKeys = append(bridgePortKeys, key)
	}
	sort.Strings(bridgePortKeys)
	for _, basePort := range bridgePortKeys {
		ifIndex := parseIndex(c.bridgePortToIf[basePort])
		if ifIndex <= 0 {
			continue
		}
		observation.BridgePorts = append(observation.BridgePorts, topologyengine.BridgePortObservation{
			BasePort: strings.TrimSpace(basePort),
			IfIndex:  ifIndex,
		})
	}

	fdbKeys := make([]string, 0, len(c.fdbEntries))
	for key := range c.fdbEntries {
		fdbKeys = append(fdbKeys, key)
	}
	sort.Strings(fdbKeys)
	for _, key := range fdbKeys {
		entry := c.fdbEntries[key]
		if entry == nil || strings.TrimSpace(entry.mac) == "" {
			continue
		}
		ifIndex := parseIndex(c.bridgePortToIf[strings.TrimSpace(entry.bridgePort)])
		vlanID := strings.TrimSpace(entry.vlanID)
		if vlanID == "" && strings.TrimSpace(entry.fdbID) != "" {
			vlanID = strings.TrimSpace(c.fdbIDToVlanID[strings.TrimSpace(entry.fdbID)])
		}
		observation.FDBEntries = append(observation.FDBEntries, topologyengine.FDBObservation{
			MAC:        strings.TrimSpace(entry.mac),
			BridgePort: strings.TrimSpace(entry.bridgePort),
			IfIndex:    ifIndex,
			Status:     strings.TrimSpace(entry.status),
			VLANID:     vlanID,
			VLANName:   strings.TrimSpace(entry.vlanName),
		})
	}

	stpKeys := make([]string, 0, len(c.stpPorts))
	for key := range c.stpPorts {
		stpKeys = append(stpKeys, key)
	}
	sort.Strings(stpKeys)
	for _, key := range stpKeys {
		entry := c.stpPorts[key]
		if entry == nil {
			continue
		}
		port := strings.TrimSpace(entry.port)
		if port == "" {
			continue
		}
		ifIndex := parseIndex(c.bridgePortToIf[port])
		ifName := ""
		if ifIndex > 0 {
			ifName = strings.TrimSpace(c.ifNamesByIndex[strconv.Itoa(ifIndex)])
		}
		observation.STPPorts = append(observation.STPPorts, topologyengine.STPPortObservation{
			Port:             port,
			IfIndex:          ifIndex,
			IfName:           ifName,
			VLANID:           strings.TrimSpace(entry.vlanID),
			VLANName:         strings.TrimSpace(entry.vlanName),
			State:            strings.TrimSpace(entry.state),
			Enable:           strings.TrimSpace(entry.enable),
			PathCost:         strings.TrimSpace(entry.pathCost),
			DesignatedRoot:   strings.TrimSpace(entry.designatedRoot),
			DesignatedBridge: strings.TrimSpace(entry.designatedBridge),
			DesignatedPort:   strings.TrimSpace(entry.designatedPort),
		})
	}

	arpKeys := make([]string, 0, len(c.arpEntries))
	for key := range c.arpEntries {
		arpKeys = append(arpKeys, key)
	}
	sort.Strings(arpKeys)
	for _, key := range arpKeys {
		entry := c.arpEntries[key]
		if entry == nil {
			continue
		}
		ifName := strings.TrimSpace(entry.ifName)
		if ifName == "" && strings.TrimSpace(entry.ifIndex) != "" {
			ifName = strings.TrimSpace(c.ifNamesByIndex[entry.ifIndex])
		}
		observation.ARPNDEntries = append(observation.ARPNDEntries, topologyengine.ARPNDObservation{
			Protocol: "arp",
			IfIndex:  parseIndex(entry.ifIndex),
			IfName:   ifName,
			IP:       strings.TrimSpace(entry.ip),
			MAC:      strings.TrimSpace(entry.mac),
			State:    strings.TrimSpace(entry.state),
			AddrType: strings.TrimSpace(entry.addrType),
		})
	}

	lldpKeys := make([]string, 0, len(c.lldpRemotes))
	for key := range c.lldpRemotes {
		lldpKeys = append(lldpKeys, key)
	}
	sort.Strings(lldpKeys)
	for _, key := range lldpKeys {
		remote := c.lldpRemotes[key]
		if remote == nil {
			continue
		}

		managementIP := normalizeIPAddress(remote.managementAddr)
		if managementIP == "" {
			managementIP = pickManagementIP(remote.managementAddrs)
		}

		localPort := c.lldpLocPorts[remote.localPortNum]
		localPortID := ""
		localPortIDSubtype := ""
		localPortDesc := ""
		if localPort != nil {
			localPortID = strings.TrimSpace(localPort.portID)
			localPortIDSubtype = strings.TrimSpace(localPort.portIDSubtype)
			localPortDesc = strings.TrimSpace(localPort.portDesc)
		}

		observation.LLDPRemotes = append(observation.LLDPRemotes, topologyengine.LLDPRemoteObservation{
			LocalPortNum:       strings.TrimSpace(remote.localPortNum),
			RemoteIndex:        strings.TrimSpace(remote.remIndex),
			LocalPortID:        localPortID,
			LocalPortIDSubtype: localPortIDSubtype,
			LocalPortDesc:      localPortDesc,
			ChassisID:          strings.TrimSpace(remote.chassisID),
			SysName:            strings.TrimSpace(remote.sysName),
			PortID:             strings.TrimSpace(remote.portID),
			PortIDSubtype:      strings.TrimSpace(remote.portIDSubtype),
			PortDesc:           strings.TrimSpace(remote.portDesc),
			ManagementIP:       managementIP,
		})
	}

	cdpKeys := make([]string, 0, len(c.cdpRemotes))
	for key := range c.cdpRemotes {
		cdpKeys = append(cdpKeys, key)
	}
	sort.Strings(cdpKeys)
	for _, key := range cdpKeys {
		remote := c.cdpRemotes[key]
		if remote == nil {
			continue
		}

		deviceID := strings.TrimSpace(remote.deviceID)
		sysName := strings.TrimSpace(remote.sysName)
		if deviceID == "" {
			deviceID = sysName
		}
		if deviceID == "" {
			continue
		}

		ifName := strings.TrimSpace(remote.ifName)
		if ifName == "" && strings.TrimSpace(remote.ifIndex) != "" {
			ifName = strings.TrimSpace(c.ifNamesByIndex[remote.ifIndex])
		}

		address := strings.TrimSpace(remote.address)
		if address == "" {
			address = pickManagementIP(remote.managementAddrs)
		}

		observation.CDPRemotes = append(observation.CDPRemotes, topologyengine.CDPRemoteObservation{
			LocalIfIndex: parseIndex(remote.ifIndex),
			LocalIfName:  ifName,
			DeviceIndex:  strings.TrimSpace(remote.deviceIndex),
			DeviceID:     deviceID,
			SysName:      sysName,
			DevicePort:   strings.TrimSpace(remote.devicePort),
			Address:      address,
		})
	}

	return observation
}

func (c *topologyCache) deriveLocalBridgeMACFromFDBSelfEntries() string {
	if len(c.fdbEntries) == 0 {
		return ""
	}
	keys := make([]string, 0, len(c.fdbEntries))
	for key := range c.fdbEntries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		entry := c.fdbEntries[key]
		if entry == nil || !isFDBSelfStatus(entry.status) {
			continue
		}
		mac := normalizeMAC(entry.mac)
		if mac == "" || mac == "00:00:00:00:00:00" {
			continue
		}
		return mac
	}
	return ""
}

func (c *topologyCache) deriveLocalBridgeMACFromInterfacePhysAddress(localManagementIP string) string {
	if len(c.ifStatusByIndex) == 0 {
		return ""
	}

	localManagementIP = normalizeIPAddress(localManagementIP)
	if localManagementIP != "" {
		ifIndex := strings.TrimSpace(c.ifIndexByIP[localManagementIP])
		if ifIndex != "" {
			if status, ok := c.ifStatusByIndex[ifIndex]; ok {
				if mac := normalizeMAC(status.mac); mac != "" && mac != "00:00:00:00:00:00" {
					return mac
				}
			}
		}
	}

	keys := make([]string, 0, len(c.ifStatusByIndex))
	for key := range c.ifStatusByIndex {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left := parseIndex(keys[i])
		right := parseIndex(keys[j])
		if left > 0 && right > 0 && left != right {
			return left < right
		}
		if left > 0 && right <= 0 {
			return true
		}
		if left <= 0 && right > 0 {
			return false
		}
		return keys[i] < keys[j]
	})
	for _, key := range keys {
		mac := normalizeMAC(c.ifStatusByIndex[key].mac)
		if mac == "" || mac == "00:00:00:00:00:00" {
			continue
		}
		return mac
	}
	return ""
}

func isFDBSelfStatus(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "4", "self", "dot1d_tp_fdb_status_self", "dot1dtpfdbstatusself", "dot1q_tp_fdb_status_self", "dot1qtpfdbstatusself":
		return true
	default:
		return false
	}
}

func ensureTopologyObservationDeviceID(local topologyDevice, baseBridgeAddress string) string {
	if mac := topologyPrimaryIdentityMAC(local.ChassisID, baseBridgeAddress); mac != "" {
		return "macAddress:" + mac
	}
	if key := strings.TrimSpace(topologyDeviceKey(local)); key != "" {
		return key
	}
	if sysName := strings.TrimSpace(local.SysName); sysName != "" {
		return "sysname:" + strings.ToLower(sysName)
	}
	if ip := normalizeIPAddress(local.ManagementIP); ip != "" {
		return "management_ip:" + ip
	}
	if managementIP := strings.TrimSpace(local.ManagementIP); managementIP != "" {
		return "management_addr:" + strings.ToLower(managementIP)
	}
	return "local-device"
}

func topologyPrimaryIdentityMAC(chassisID, baseBridgeAddress string) string {
	for _, candidate := range []string{chassisID, baseBridgeAddress} {
		if mac := normalizeMAC(candidate); mac != "" && mac != "00:00:00:00:00:00" {
			return mac
		}
	}
	return ""
}

func augmentLocalActorFromCache(data *topologyData, local topologyDevice) {
	if data == nil || len(data.Actors) == 0 {
		return
	}

	for i := range data.Actors {
		actor := &data.Actors[i]
		if !topologyengine.IsDeviceActorType(actor.ActorType) {
			continue
		}
		if !matchLocalTopologyActor(actor.Match, local) {
			continue
		}

		attrs := actor.Attributes
		if attrs == nil {
			attrs = make(map[string]any)
		}
		if len(local.ManagementAddresses) > 0 {
			attrs["management_addresses"] = local.ManagementAddresses
		}
		if len(local.Capabilities) > 0 {
			attrs["capabilities"] = local.Capabilities
		}
		if len(local.CapabilitiesSupported) > 0 {
			attrs["capabilities_supported"] = local.CapabilitiesSupported
		}
		if len(local.CapabilitiesEnabled) > 0 {
			attrs["capabilities_enabled"] = local.CapabilitiesEnabled
		}
		if sysDescr := strings.TrimSpace(local.SysDescr); sysDescr != "" {
			attrs["sys_descr"] = sysDescr
		}
		if sysContact := strings.TrimSpace(local.SysContact); sysContact != "" {
			attrs["sys_contact"] = sysContact
		}
		if sysLocation := strings.TrimSpace(local.SysLocation); sysLocation != "" {
			attrs["sys_location"] = sysLocation
		}
		if local.SysUptime > 0 {
			attrs["sys_uptime"] = local.SysUptime
		}
		if vendor := strings.TrimSpace(local.Vendor); vendor != "" {
			attrs["vendor"] = vendor
			attrs["vendor_source"] = "snmp"
			attrs["vendor_confidence"] = "high"
		}
		if model := strings.TrimSpace(local.Model); model != "" {
			attrs["model"] = model
		}
		if serial := strings.TrimSpace(local.SerialNumber); serial != "" {
			attrs["serial_number"] = serial
		}
		if software := strings.TrimSpace(local.SoftwareVersion); software != "" {
			attrs["software_version"] = software
		}
		if firmware := strings.TrimSpace(local.FirmwareVersion); firmware != "" {
			attrs["firmware_version"] = firmware
		}
		if hardware := strings.TrimSpace(local.HardwareVersion); hardware != "" {
			attrs["hardware_version"] = hardware
		}
		if managementIP := normalizeIPAddress(local.ManagementIP); managementIP != "" {
			attrs["management_ip"] = managementIP
		}
		if netdataHostID := strings.TrimSpace(local.NetdataHostID); netdataHostID != "" {
			attrs["netdata_host_id"] = netdataHostID
		}
		if chartIDPrefix := strings.TrimSpace(local.ChartIDPrefix); chartIDPrefix != "" {
			attrs["chart_id_prefix"] = chartIDPrefix
		}
		if chartContextPrefix := strings.TrimSpace(local.ChartContextPrefix); chartContextPrefix != "" {
			attrs["chart_context_prefix"] = chartContextPrefix
		}
		if len(local.DeviceCharts) > 0 {
			attrs["device_charts"] = mapStringStringToAny(local.DeviceCharts)
		}
		if len(local.InterfaceCharts) > 0 {
			if statuses, ok := attrs["if_statuses"]; ok && statuses != nil {
				attrs["if_statuses"] = enrichTopologyInterfaceStatusesWithChartRefs(statuses, local.InterfaceCharts)
			}
			if actor.Tables != nil {
				if portRows, ok := actor.Tables["ports"]; ok && len(portRows) > 0 {
					enrichTopologyTableRowsWithChartRefs(portRows, local.InterfaceCharts)
				}
			}
		}

		actor.Attributes = pruneNilAttributes(attrs)
		if actor.Labels == nil {
			actor.Labels = make(map[string]string)
		}
		for key, value := range local.Labels {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			actor.Labels[key] = value
		}
		return
	}
}

func matchLocalTopologyActor(match topology.Match, local topologyDevice) bool {
	localChassisID := strings.TrimSpace(local.ChassisID)
	if localChassisID != "" {
		for _, chassisID := range match.ChassisIDs {
			if strings.EqualFold(strings.TrimSpace(chassisID), localChassisID) {
				return true
			}
		}
	}

	localSysName := strings.TrimSpace(local.SysName)
	if localSysName != "" && strings.EqualFold(strings.TrimSpace(match.SysName), localSysName) {
		return true
	}

	localIP := normalizeIPAddress(local.ManagementIP)
	if localIP != "" {
		for _, ip := range match.IPAddresses {
			if normalizeIPAddress(ip) == localIP {
				return true
			}
		}
	}

	return false
}

func canonicalMatchKey(match topology.Match) string {
	if key := canonicalPrimaryMACListKey(match); key != "" {
		return "mac:" + key
	}
	if key := canonicalHardwareListKey(match.ChassisIDs); key != "" {
		return "chassis:" + key
	}
	if key := canonicalIPListKey(match.IPAddresses); key != "" {
		return "ip:" + key
	}
	if key := canonicalStringListKey(match.Hostnames); key != "" {
		return "hostname:" + key
	}
	if key := canonicalStringListKey(match.DNSNames); key != "" {
		return "dns:" + key
	}
	if sysName := strings.ToLower(strings.TrimSpace(match.SysName)); sysName != "" {
		return "sysname:" + sysName
	}
	if match.SysObjectID != "" {
		return "sysobjectid:" + match.SysObjectID
	}
	return ""
}

func canonicalPrimaryMACListKey(match topology.Match) string {
	seen := make(map[string]struct{}, len(match.MacAddresses)+len(match.ChassisIDs))
	for _, value := range match.MacAddresses {
		if mac := normalizeMAC(value); mac != "" {
			seen[mac] = struct{}{}
		}
	}
	for _, value := range match.ChassisIDs {
		if mac := normalizeMAC(value); mac != "" {
			seen[mac] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return ""
	}
	values := make([]string, 0, len(seen))
	for value := range seen {
		values = append(values, value)
	}
	sort.Strings(values)
	return strings.Join(values, ",")
}

func topologyMatchIdentityKeys(match topology.Match) []string {
	seen := make(map[string]struct{}, 8)
	add := func(kind, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		seen[kind+":"+value] = struct{}{}
	}

	for _, value := range match.ChassisIDs {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if mac := normalizeMAC(value); mac != "" {
			add("hw", mac)
			continue
		}
		if ip := normalizeIPAddress(value); ip != "" {
			add("ip", ip)
			continue
		}
		add("chassis", strings.ToLower(value))
	}
	for _, value := range match.MacAddresses {
		if mac := normalizeMAC(value); mac != "" {
			add("hw", mac)
		}
	}
	for _, value := range match.IPAddresses {
		if ip := normalizeIPAddress(value); ip != "" {
			add("ip", ip)
			continue
		}
		add("ipraw", strings.ToLower(strings.TrimSpace(value)))
	}
	for _, value := range match.Hostnames {
		add("hostname", strings.ToLower(strings.TrimSpace(value)))
	}
	for _, value := range match.DNSNames {
		add("dns", strings.ToLower(strings.TrimSpace(value)))
	}
	if sysName := strings.TrimSpace(match.SysName); sysName != "" {
		add("sysname", strings.ToLower(sysName))
	}

	if len(seen) == 0 {
		return nil
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func canonicalHardwareListKey(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if mac := normalizeMAC(value); mac != "" {
			out = append(out, mac)
			continue
		}
		if ip := normalizeIPAddress(value); ip != "" {
			out = append(out, ip)
			continue
		}
		out = append(out, strings.ToLower(value))
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	out = uniqueStrings(out)
	return strings.Join(out, ",")
}

func canonicalMACListKey(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if mac := normalizeMAC(value); mac != "" {
			out = append(out, mac)
		}
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	out = uniqueStrings(out)
	return strings.Join(out, ",")
}

func canonicalIPListKey(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if ip := normalizeIPAddress(value); ip != "" {
			out = append(out, ip)
			continue
		}
		out = append(out, strings.ToLower(value))
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	out = uniqueStrings(out)
	return strings.Join(out, ",")
}

func canonicalStringListKey(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	out = uniqueStrings(out)
	return strings.Join(out, ",")
}

func uniqueStrings(values []string) []string {
	if len(values) <= 1 {
		return values
	}
	out := values[:0]
	var prev string
	for i, value := range values {
		if i == 0 || value != prev {
			out = append(out, value)
			prev = value
		}
	}
	return out
}

func topologyLinkSortKey(link topology.Link) string {
	return strings.Join([]string{
		link.Protocol,
		link.Direction,
		canonicalMatchKey(link.Src.Match),
		canonicalMatchKey(link.Dst.Match),
		attrKey(link.Src.Attributes, "if_index"),
		attrKey(link.Src.Attributes, "if_name"),
		attrKey(link.Src.Attributes, "port_id"),
		attrKey(link.Dst.Attributes, "if_index"),
		attrKey(link.Dst.Attributes, "if_name"),
		attrKey(link.Dst.Attributes, "port_id"),
		link.State,
	}, "|")
}

func attrKey(attrs map[string]any, key string) string {
	if len(attrs) == 0 {
		return ""
	}
	v, ok := attrs[key]
	if !ok || v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func pruneNilAttributes(attrs map[string]any) map[string]any {
	for k, v := range attrs {
		switch vv := v.(type) {
		case string:
			if vv == "" {
				delete(attrs, k)
			}
		case []string:
			if len(vv) == 0 {
				delete(attrs, k)
			}
		case []topologyManagementAddress:
			if len(vv) == 0 {
				delete(attrs, k)
			}
		case nil:
			delete(attrs, k)
		}
	}
	if len(attrs) == 0 {
		return nil
	}
	return attrs
}

func mapStringStringToAny(in map[string]string) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneTopologyLabels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func topologyCanonicalMetadataKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return ""
	}
	key = strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(key)
	for strings.Contains(key, "__") {
		key = strings.ReplaceAll(key, "__", "_")
	}
	return strings.Trim(key, "_")
}

func topologyMetadataValue(labels map[string]string, aliases []string) string {
	if len(labels) == 0 || len(aliases) == 0 {
		return ""
	}
	byKey := make(map[string]string, len(labels))
	for key, value := range labels {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		canonical := topologyCanonicalMetadataKey(key)
		if canonical == "" {
			continue
		}
		if _, exists := byKey[canonical]; !exists {
			byKey[canonical] = value
		}
	}
	for _, alias := range aliases {
		alias = topologyCanonicalMetadataKey(alias)
		if alias == "" {
			continue
		}
		if value := strings.TrimSpace(byKey[alias]); value != "" {
			return value
		}
	}
	return ""
}

func setTopologyMetadataLabelIfMissing(labels map[string]string, key, value string) {
	if labels == nil {
		return
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return
	}
	if existing := strings.TrimSpace(labels[key]); existing == "" {
		labels[key] = value
	}
}

func enrichTopologyInterfaceStatusesWithChartRefs(
	statuses any,
	interfaceCharts map[string]topologyInterfaceChartRef,
) any {
	if len(interfaceCharts) == 0 || statuses == nil {
		return statuses
	}

	lookup := make(map[string]topologyInterfaceChartRef, len(interfaceCharts))
	for ifName, ref := range interfaceCharts {
		ifName = strings.ToLower(strings.TrimSpace(ifName))
		if ifName == "" {
			continue
		}
		if strings.TrimSpace(ref.ChartIDSuffix) == "" {
			ref.ChartIDSuffix = ifName
		}
		ref.AvailableMetrics = deduplicateSortedStrings(ref.AvailableMetrics)
		lookup[ifName] = ref
	}
	if len(lookup) == 0 {
		return statuses
	}

	switch typed := statuses.(type) {
	case []map[string]any:
		for _, status := range typed {
			ifName := strings.ToLower(strings.TrimSpace(fmt.Sprint(status["if_name"])))
			if ifName == "" {
				continue
			}
			ref, ok := lookup[ifName]
			if !ok {
				continue
			}
			status["chart_id_suffix"] = ref.ChartIDSuffix
			if len(ref.AvailableMetrics) > 0 {
				status["available_metrics"] = ref.AvailableMetrics
			}
		}
		return typed
	case []any:
		for i := range typed {
			status, ok := typed[i].(map[string]any)
			if !ok || status == nil {
				continue
			}
			ifName := strings.ToLower(strings.TrimSpace(fmt.Sprint(status["if_name"])))
			if ifName == "" {
				continue
			}
			ref, ok := lookup[ifName]
			if !ok {
				continue
			}
			status["chart_id_suffix"] = ref.ChartIDSuffix
			if len(ref.AvailableMetrics) > 0 {
				status["available_metrics"] = ref.AvailableMetrics
			}
			typed[i] = status
		}
		return typed
	default:
		return statuses
	}
}

func enrichTopologyTableRowsWithChartRefs(rows []map[string]any, interfaceCharts map[string]topologyInterfaceChartRef) {
	if len(interfaceCharts) == 0 || len(rows) == 0 {
		return
	}

	lookup := make(map[string]topologyInterfaceChartRef, len(interfaceCharts))
	for ifName, ref := range interfaceCharts {
		ifName = strings.ToLower(strings.TrimSpace(ifName))
		if ifName == "" {
			continue
		}
		if strings.TrimSpace(ref.ChartIDSuffix) == "" {
			ref.ChartIDSuffix = ifName
		}
		ref.AvailableMetrics = deduplicateSortedStrings(ref.AvailableMetrics)
		lookup[ifName] = ref
	}
	if len(lookup) == 0 {
		return
	}

	for _, row := range rows {
		name := strings.ToLower(strings.TrimSpace(fmt.Sprint(row["name"])))
		if name == "" {
			continue
		}
		ref, ok := lookup[name]
		if !ok {
			continue
		}
		row["chart_id_suffix"] = ref.ChartIDSuffix
		if len(ref.AvailableMetrics) > 0 {
			row["available_metrics"] = ref.AvailableMetrics
		}
	}
}

func normalizeTopologyDevice(dev topologyDevice) topologyDevice {
	if dev.ChartIDPrefix == "" {
		dev.ChartIDPrefix = topologyProfileChartIDPrefix
	}
	if dev.ChartContextPrefix == "" {
		dev.ChartContextPrefix = topologyProfileChartContextPrefix
	}
	if dev.ManagementIP == "" && len(dev.ManagementAddresses) > 0 {
		if ip := pickManagementIP(dev.ManagementAddresses); ip != "" {
			dev.ManagementIP = ip
		}
	}
	if len(dev.Capabilities) == 0 {
		if len(dev.CapabilitiesEnabled) > 0 {
			dev.Capabilities = dev.CapabilitiesEnabled
		} else if len(dev.CapabilitiesSupported) > 0 {
			dev.Capabilities = dev.CapabilitiesSupported
		}
	}
	if dev.Labels == nil {
		dev.Labels = make(map[string]string)
	}
	if strings.TrimSpace(dev.Labels["type"]) == "" && len(dev.Capabilities) > 0 {
		dev.Labels["type"] = inferCategoryFromCapabilities(dev.Capabilities)
	}
	if dev.ChassisID == "" && dev.ManagementIP != "" {
		dev.ChassisID = dev.ManagementIP
		dev.ChassisIDType = "management_ip"
	}
	if dev.ChassisID != "" && dev.ChassisIDType == "" {
		dev.ChassisIDType = "unknown"
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasSysDescr); value != "" && dev.SysDescr == "" {
		dev.SysDescr = value
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasSysContact); value != "" && dev.SysContact == "" {
		dev.SysContact = value
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasSysLocation); value != "" && dev.SysLocation == "" {
		dev.SysLocation = value
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasVendor); value != "" && dev.Vendor == "" {
		dev.Vendor = value
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasModel); value != "" && dev.Model == "" {
		dev.Model = value
	}
	if dev.SysUptime <= 0 {
		if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasSysUptime); value != "" {
			dev.SysUptime = parsePositiveInt64(value)
		}
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasSerial); value != "" && dev.SerialNumber == "" {
		dev.SerialNumber = value
		setTopologyMetadataLabelIfMissing(dev.Labels, "serial_number", value)
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasSoftware); value != "" && dev.SoftwareVersion == "" {
		dev.SoftwareVersion = value
		setTopologyMetadataLabelIfMissing(dev.Labels, "software_version", value)
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasFirmware); value != "" && dev.FirmwareVersion == "" {
		dev.FirmwareVersion = value
		setTopologyMetadataLabelIfMissing(dev.Labels, "firmware_version", value)
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasHardware); value != "" && dev.HardwareVersion == "" {
		dev.HardwareVersion = value
		setTopologyMetadataLabelIfMissing(dev.Labels, "hardware_version", value)
	}
	return dev
}

func topologyDeviceKey(dev topologyDevice) string {
	if dev.ChassisID == "" {
		return ""
	}
	return dev.ChassisIDType + ":" + dev.ChassisID
}

func normalizeLLDPSubtype(value string, mapping map[string]string) string {
	if v, ok := mapping[value]; ok {
		return v
	}
	return value
}

func ensureLabels(labels map[string]string) map[string]string {
	if labels == nil {
		return make(map[string]string)
	}
	return labels
}

func deduplicateSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func appendManagementAddress(addrs []topologyManagementAddress, addr topologyManagementAddress) []topologyManagementAddress {
	if addr.Address == "" {
		return addrs
	}
	for _, existing := range addrs {
		if existing.Address == addr.Address && existing.AddressType == addr.AddressType && existing.Source == addr.Source {
			return addrs
		}
	}
	return append(addrs, addr)
}

func appendCdpManagementAddresses(entry *cdpRemote, current []topologyManagementAddress) []topologyManagementAddress {
	addrs := current
	if entry.primaryMgmtAddr != "" {
		addr, addrType := normalizeManagementAddress(entry.primaryMgmtAddr, entry.primaryMgmtAddrType)
		if addr != "" {
			addrs = appendManagementAddress(addrs, topologyManagementAddress{
				Address:     addr,
				AddressType: addrType,
				Source:      "cdp_primary_mgmt",
			})
		}
	}
	if entry.secondaryMgmtAddr != "" {
		addr, addrType := normalizeManagementAddress(entry.secondaryMgmtAddr, entry.secondaryMgmtAddrType)
		if addr != "" {
			addrs = appendManagementAddress(addrs, topologyManagementAddress{
				Address:     addr,
				AddressType: addrType,
				Source:      "cdp_secondary_mgmt",
			})
		}
	}
	if entry.address != "" {
		addr, addrType := normalizeManagementAddress(entry.address, entry.addressType)
		if addr != "" {
			addrs = appendManagementAddress(addrs, topologyManagementAddress{
				Address:     addr,
				AddressType: addrType,
				Source:      "cdp_cache_address",
			})
		}
	}
	return addrs
}

func pickManagementIP(addrs []topologyManagementAddress) string {
	if len(addrs) == 0 {
		return ""
	}

	ipSet := make(map[string]struct{}, len(addrs))
	ipValues := make([]string, 0, len(addrs))
	rawSet := make(map[string]struct{}, len(addrs))
	rawValues := make([]string, 0, len(addrs))

	for _, addr := range addrs {
		value := strings.TrimSpace(addr.Address)
		if value == "" {
			continue
		}
		if ip := normalizeIPAddress(value); ip != "" {
			if _, exists := ipSet[ip]; exists {
				continue
			}
			ipSet[ip] = struct{}{}
			ipValues = append(ipValues, ip)
			continue
		}
		if _, exists := rawSet[value]; exists {
			continue
		}
		rawSet[value] = struct{}{}
		rawValues = append(rawValues, value)
	}

	if len(ipValues) > 0 {
		sort.Strings(ipValues)
		return ipValues[0]
	}
	if len(rawValues) > 0 {
		sort.Strings(rawValues)
		return rawValues[0]
	}
	return ""
}

func reconstructLldpRemMgmtAddrHex(tags map[string]string) string {
	lengthStr := strings.TrimSpace(tags[tagLldpRemMgmtAddrLen])
	length, err := strconv.Atoi(lengthStr)
	if err != nil || length <= 0 || length > net.IPv6len {
		return ""
	}

	addr := make([]byte, 0, length)
	for i := 1; i <= length; i++ {
		tag := fmt.Sprintf("%s%d", tagLldpRemMgmtAddrOctetPref, i)
		v := strings.TrimSpace(tags[tag])
		if v == "" {
			return ""
		}
		octet, err := strconv.Atoi(v)
		if err != nil || octet < 0 || octet > 255 {
			return ""
		}
		addr = append(addr, byte(octet))
	}

	return hex.EncodeToString(addr)
}

func normalizeManagementAddress(rawAddr, rawType string) (string, string) {
	rawAddr = strings.TrimSpace(rawAddr)
	if rawAddr == "" {
		return "", normalizeAddressType(rawType, "")
	}

	if ip := net.ParseIP(rawAddr); ip != nil {
		return ip.String(), normalizeAddressType(rawType, ip.String())
	}

	if bs, err := decodeHexString(rawAddr); err == nil {
		if ip := parseIPFromDecodedBytes(bs); ip != nil {
			return ip.String(), normalizeAddressType(rawType, ip.String())
		}
	}

	return rawAddr, normalizeAddressType(rawType, rawAddr)
}

func normalizeIPAddress(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if ip := net.ParseIP(value); ip != nil {
		return ip.String()
	}

	if bs, err := decodeHexString(value); err == nil {
		if ip := parseIPFromDecodedBytes(bs); ip != nil {
			return ip.String()
		}
	}

	return ""
}

func parseIPFromDecodedBytes(bs []byte) net.IP {
	if len(bs) == net.IPv4len || len(bs) == net.IPv6len {
		ip := net.IP(bs)
		if ip.To16() != nil {
			return ip
		}
	}

	ascii := decodePrintableASCII(bs)
	if ascii == "" {
		return nil
	}

	if ip := net.ParseIP(ascii); ip != nil {
		return ip
	}
	return nil
}

func decodePrintableASCII(bs []byte) string {
	if len(bs) == 0 {
		return ""
	}

	for _, b := range bs {
		if b == 0 {
			continue
		}
		if b < 32 || b > 126 {
			return ""
		}
	}

	s := strings.TrimRight(string(bs), "\x00")
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return s
}

func normalizeMAC(value string) string {
	value = normalizeSNMPHexText(value)
	if value == "" {
		return ""
	}

	if hw, err := net.ParseMAC(value); err == nil {
		return strings.ToLower(hw.String())
	}

	clean := strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(strings.ToLower(value))
	if clean == "" {
		return ""
	}

	bs, err := decodeHexString(clean)
	if err != nil || len(bs) != 6 {
		return ""
	}

	return strings.ToLower(net.HardwareAddr(bs).String())
}

func normalizeHexToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	if mac := normalizeMAC(value); mac != "" {
		return mac
	}
	if ip := normalizeIPAddress(value); ip != "" {
		return ip
	}
	return strings.TrimSpace(value)
}

func normalizeHexIdentifier(value string) string {
	value = normalizeSNMPHexText(value)
	if value == "" {
		return ""
	}

	bs, err := decodeHexString(value)
	if err == nil && len(bs) > 0 {
		return strings.ToLower(hex.EncodeToString(bs))
	}

	clean := strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(strings.ToLower(value))
	if clean == "" {
		return ""
	}
	return clean
}

type stpBridgeIDStatus uint8

const (
	stpBridgeIDInvalid stpBridgeIDStatus = iota
	stpBridgeIDEmpty
	stpBridgeIDValid
)

func stpBridgeAddressToMAC(value string) string {
	mac, status := parseSTPBridgeID(value, 0)
	if status != stpBridgeIDValid {
		return ""
	}
	return mac
}

func parseSTPBridgeID(value string, depth int) (string, stpBridgeIDStatus) {
	if depth > 2 {
		return "", stpBridgeIDInvalid
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return "", stpBridgeIDEmpty
	}

	if mac := normalizeMAC(value); mac != "" {
		if mac == "00:00:00:00:00:00" {
			return "", stpBridgeIDEmpty
		}
		return mac, stpBridgeIDValid
	}

	if priority, bridgeID, ok := splitSTPBridgeIDWithPriority(value); ok {
		if priority == "0" && isSTPAllZeroBridgeID(bridgeID) {
			return "", stpBridgeIDEmpty
		}
		return parseSTPBridgeID(bridgeID, depth+1)
	}

	bs, err := decodeHexString(value)
	if err != nil || len(bs) == 0 {
		return "", stpBridgeIDInvalid
	}
	if allBytesZero(bs) {
		return "", stpBridgeIDEmpty
	}
	if ascii := decodePrintableASCII(bs); ascii != "" && depth < 2 {
		return parseSTPBridgeID(ascii, depth+1)
	}
	switch len(bs) {
	case 6:
		mac := strings.ToLower(net.HardwareAddr(bs).String())
		if mac == "00:00:00:00:00:00" {
			return "", stpBridgeIDEmpty
		}
		return mac, stpBridgeIDValid
	case 8:
		mac := strings.ToLower(net.HardwareAddr(bs[len(bs)-6:]).String())
		if mac == "00:00:00:00:00:00" {
			return "", stpBridgeIDEmpty
		}
		return mac, stpBridgeIDValid
	default:
		return "", stpBridgeIDInvalid
	}
}

func splitSTPBridgeIDWithPriority(value string) (string, string, bool) {
	parts := strings.SplitN(value, "-", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	priority := strings.TrimSpace(parts[0])
	bridgeID := strings.TrimSpace(parts[1])
	if priority == "" || bridgeID == "" {
		return "", "", false
	}
	if _, err := strconv.Atoi(priority); err != nil {
		return "", "", false
	}
	return priority, bridgeID, true
}

func isSTPAllZeroBridgeID(value string) bool {
	mac := normalizeMAC(value)
	if mac == "00:00:00:00:00:00" {
		return true
	}
	if mac != "" {
		return false
	}
	clean := normalizeHexIdentifier(value)
	if clean == "" {
		return false
	}
	for _, r := range clean {
		if r != '0' {
			return false
		}
	}
	return true
}

func allBytesZero(bs []byte) bool {
	for _, b := range bs {
		if b != 0 {
			return false
		}
	}
	return true
}

func stpDesignatedPortString(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if _, err := strconv.Atoi(value); err == nil {
		return value
	}
	return normalizeHexIdentifier(value)
}

func normalizeAddressType(rawType, addr string) string {
	if ip := net.ParseIP(addr); ip != nil {
		if ip.To4() != nil {
			return "ipv4"
		}
		return "ipv6"
	}

	switch rawType {
	case "1":
		return "ipv4"
	case "2":
		return "ipv6"
	}
	return rawType
}

func managementAddressTypeFromIP(ip string) string {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return ""
	}
	if parsed.To4() != nil {
		return "ipv4"
	}
	return "ipv6"
}

func normalizeInterfaceAdminStatus(value string) string {
	value = canonicalSNMPEnumValue(value)
	switch value {
	case "1":
		return "up"
	case "2":
		return "down"
	case "3":
		return "testing"
	case "up", "down", "testing":
		return value
	default:
		return ""
	}
}

func normalizeInterfaceOperStatus(value string) string {
	value = canonicalSNMPEnumValue(value)
	switch value {
	case "1":
		return "up"
	case "2":
		return "down"
	case "3":
		return "testing"
	case "4":
		return "unknown"
	case "5":
		return "dormant"
	case "6":
		return "notPresent"
	case "7":
		return "lowerLayerDown"
	case "up", "down", "testing", "unknown", "dormant":
		return value
	case "notpresent":
		return "notPresent"
	case "not_present":
		return "notPresent"
	case "lowerlayerdown":
		return "lowerLayerDown"
	case "lower_layer_down":
		return "lowerLayerDown"
	default:
		return ""
	}
}

func normalizeInterfaceType(value string) string {
	value = canonicalSNMPEnumValue(value)
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if name, ok := ianaIfTypeByNumber[value]; ok {
		return name
	}
	// Unknown numeric values become "type-XXX".
	if _, err := strconv.Atoi(value); err == nil {
		return "type-" + value
	}
	return strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(value))
}

// Complete IANA ifType registry (IANAifType-MIB, updated 2026-02-24).
// Source: https://www.iana.org/assignments/ianaiftype-mib/ianaiftype-mib
var ianaIfTypeByNumber = map[string]string{
	"1":   "other",
	"2":   "regular1822",
	"3":   "hdh1822",
	"4":   "ddnx25",
	"5":   "rfc877x25",
	"6":   "ethernetcsmacd",
	"7":   "iso88023csmacd",
	"8":   "iso88024tokenbus",
	"9":   "iso88025tokenring",
	"10":  "iso88026man",
	"11":  "starlan",
	"12":  "proteon10mbit",
	"13":  "proteon80mbit",
	"14":  "hyperchannel",
	"15":  "fddi",
	"16":  "lapb",
	"17":  "sdlc",
	"18":  "ds1",
	"19":  "e1",
	"20":  "basicisdn",
	"21":  "primaryisdn",
	"22":  "proppointtopointserial",
	"23":  "ppp",
	"24":  "softwareloopback",
	"25":  "eon",
	"26":  "ethernet3mbit",
	"27":  "nsip",
	"28":  "slip",
	"29":  "ultra",
	"30":  "ds3",
	"31":  "sip",
	"32":  "framerelay",
	"33":  "rs232",
	"34":  "para",
	"35":  "arcnet",
	"36":  "arcnetplus",
	"37":  "atm",
	"38":  "miox25",
	"39":  "sonet",
	"40":  "x25ple",
	"41":  "iso88022llc",
	"42":  "localtalk",
	"43":  "smdsdxi",
	"44":  "framerelayservice",
	"45":  "v35",
	"46":  "hssi",
	"47":  "hippi",
	"48":  "modem",
	"49":  "aal5",
	"50":  "sonetpath",
	"51":  "sonetvt",
	"52":  "smdsicip",
	"53":  "propvirtual",
	"54":  "propmultiplexor",
	"55":  "ieee80212",
	"56":  "fibrechannel",
	"57":  "hippiinterface",
	"58":  "framerelayinterconnect",
	"59":  "aflane8023",
	"60":  "aflane8025",
	"61":  "cctemul",
	"62":  "fastether",
	"63":  "isdn",
	"64":  "v11",
	"65":  "v36",
	"66":  "g703at64k",
	"67":  "g703at2mb",
	"68":  "qllc",
	"69":  "fastetherfx",
	"70":  "channel",
	"71":  "ieee80211",
	"72":  "ibm370parchan",
	"73":  "escon",
	"74":  "dlsw",
	"75":  "isdns",
	"76":  "isdnu",
	"77":  "lapd",
	"78":  "ipswitch",
	"79":  "rsrb",
	"80":  "atmlogical",
	"81":  "ds0",
	"82":  "ds0bundle",
	"83":  "bsc",
	"84":  "async",
	"85":  "cnr",
	"86":  "iso88025dtr",
	"87":  "eplrs",
	"88":  "arap",
	"89":  "propcnls",
	"90":  "hostpad",
	"91":  "termpad",
	"92":  "framerelaympi",
	"93":  "x213",
	"94":  "adsl",
	"95":  "radsl",
	"96":  "sdsl",
	"97":  "vdsl",
	"98":  "iso88025crfpint",
	"99":  "myrinet",
	"100": "voiceem",
	"101": "voicefxo",
	"102": "voicefxs",
	"103": "voiceencap",
	"104": "voiceoverip",
	"105": "atmdxi",
	"106": "atmfuni",
	"107": "atmima",
	"108": "pppmultilinkbundle",
	"109": "ipovercdlc",
	"110": "ipoverclaw",
	"111": "stacktostack",
	"112": "virtualipaddress",
	"113": "mpc",
	"114": "ipoveratm",
	"115": "iso88025fiber",
	"116": "tdlc",
	"117": "gigabitethernet",
	"118": "hdlc",
	"119": "lapf",
	"120": "v37",
	"121": "x25mlp",
	"122": "x25huntgroup",
	"123": "transphdlc",
	"124": "interleave",
	"125": "fast",
	"126": "ip",
	"127": "docscablemaclayer",
	"128": "docscabledownstream",
	"129": "docscableupstream",
	"130": "a12mppswitch",
	"131": "tunnel",
	"132": "coffee",
	"133": "ces",
	"134": "atmsubinterface",
	"135": "l2vlan",
	"136": "l3ipvlan",
	"137": "l3ipxvlan",
	"138": "digitalpowerline",
	"139": "mediamailoverip",
	"140": "dtm",
	"141": "dcn",
	"142": "ipforward",
	"143": "msdsl",
	"144": "ieee1394",
	"145": "gsn",
	"146": "dvbrccmaclayer",
	"147": "dvbrccdownstream",
	"148": "dvbrccupstream",
	"149": "atmvirtual",
	"150": "mplstunnel",
	"151": "srp",
	"152": "voiceoveratm",
	"153": "voiceoverframerelay",
	"154": "idsl",
	"155": "compositelink",
	"156": "ss7siglink",
	"157": "propwirelessp2p",
	"158": "frforward",
	"159": "rfc1483",
	"160": "usb",
	"161": "ieee8023adlag",
	"162": "bgppolicyaccounting",
	"163": "frf16mfrbundle",
	"164": "h323gatekeeper",
	"165": "h323proxy",
	"166": "mpls",
	"167": "mfsiglink",
	"168": "hdsl2",
	"169": "shdsl",
	"170": "ds1fdl",
	"171": "pos",
	"172": "dvbasiin",
	"173": "dvbasiout",
	"174": "plc",
	"175": "nfas",
	"176": "tr008",
	"177": "gr303rdt",
	"178": "gr303idt",
	"179": "isup",
	"180": "propdocswirelessmaclayer",
	"181": "propdocswirelessdownstream",
	"182": "propdocswirelessupstream",
	"183": "hiperlan2",
	"184": "propbwap2mp",
	"185": "sonetoverheadchannel",
	"186": "digitalwrapperoverheadchannel",
	"187": "aal2",
	"188": "radiomac",
	"189": "atmradio",
	"190": "imt",
	"191": "mvl",
	"192": "reachdsl",
	"193": "frdlciendpt",
	"194": "atmvciendpt",
	"195": "opticalchannel",
	"196": "opticaltransport",
	"197": "propatm",
	"198": "voiceovercable",
	"199": "infiniband",
	"200": "telink",
	"201": "q2931",
	"202": "virtualtg",
	"203": "siptg",
	"204": "sipsig",
	"205": "docscableupstreamchannel",
	"206": "econet",
	"207": "pon155",
	"208": "pon622",
	"209": "bridge",
	"210": "linegroup",
	"211": "voiceemfgd",
	"212": "voicefgdeana",
	"213": "voicedid",
	"214": "mpegtransport",
	"215": "sixtofour",
	"216": "gtp",
	"217": "pdnetherloop1",
	"218": "pdnetherloop2",
	"219": "opticalchannelgroup",
	"220": "homepna",
	"221": "gfp",
	"222": "ciscoislvlan",
	"223": "actelismetaloop",
	"224": "fciplink",
	"225": "rpr",
	"226": "qam",
	"227": "lmp",
	"228": "cblvectastar",
	"229": "docscablemcmtsdownstream",
	"230": "adsl2",
	"231": "macseccontrolledif",
	"232": "macsecuncontrolledif",
	"233": "aviciopticalether",
	"234": "atmbond",
	"235": "voicefgdos",
	"236": "mocaversion1",
	"237": "ieee80216wman",
	"238": "adsl2plus",
	"239": "dvbrcsmaclayer",
	"240": "dvbtdm",
	"241": "dvbrcstdma",
	"242": "x86laps",
	"243": "wwanpp",
	"244": "wwanpp2",
	"245": "voiceebs",
	"246": "ifpwtype",
	"247": "ilan",
	"248": "pip",
	"249": "aluelp",
	"250": "gpon",
	"251": "vdsl2",
	"252": "capwapdot11profile",
	"253": "capwapdot11bss",
	"254": "capwapwtpvirtualradio",
	"255": "bits",
	"256": "docscableupstreamrfport",
	"257": "cabledownstreamrfport",
	"258": "vmwarevirtualnic",
	"259": "ieee802154",
	"260": "otnodu",
	"261": "otnotu",
	"262": "ifvfitype",
	"263": "g9981",
	"264": "g9982",
	"265": "g9983",
	"266": "aluepon",
	"267": "aluepononu",
	"268": "alueponphysicaluni",
	"269": "alueponlogicallink",
	"270": "alugpononu",
	"271": "alugponphysicaluni",
	"272": "vmwarenicteam",
	"277": "docsofdmdownstream",
	"278": "docsofdmaupstream",
	"279": "gfast",
	"280": "sdci",
	"281": "xboxwireless",
	"282": "fastdsl",
	"283": "docscablescte55d1fwdoob",
	"284": "docscablescte55d1retoob",
	"285": "docscablescte55d2dsoob",
	"286": "docscablescte55d2usoob",
	"287": "docscablendf",
	"288": "docscablendr",
	"289": "ptm",
	"290": "ghn",
	"291": "otnotsi",
	"292": "otnotuc",
	"293": "otnoduc",
	"294": "otnotsig",
	"295": "microwavecarriertermination",
	"296": "microwaveradiolinkterminal",
	"297": "ieee8021axdrni",
	"298": "ax25",
	"299": "ieee19061nanocom",
	"300": "cpri",
	"301": "omni",
	"302": "roe",
	"303": "p2poverlan",
	"304": "docscablescte25d1fwdoob",
	"305": "docscablescte25d1retoob",
	"306": "docscablescte25d2macoob",
}

func normalizeInterfaceDuplex(value string) string {
	value = canonicalSNMPEnumValue(value)
	switch value {
	case "1", "unknown":
		return "unknown"
	case "2", "half", "halfduplex", "half_duplex":
		return "half"
	case "3", "full", "fullduplex", "full_duplex":
		return "full"
	default:
		return ""
	}
}

func parsePositiveInt64(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

func canonicalSNMPEnumValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	// gosnmp often formats enums as "up(1)" / "down(2)"; keep only the symbolic token.
	if open := strings.IndexByte(value, '('); open > 0 && strings.HasSuffix(value, ")") {
		value = strings.TrimSpace(value[:open])
	}
	return value
}

func decodeHexString(value string) ([]byte, error) {
	clean := strings.TrimPrefix(strings.ToLower(normalizeSNMPHexText(value)), "0x")
	clean = strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(clean)
	if clean == "" {
		return nil, fmt.Errorf("empty hex string")
	}
	if len(clean)%2 == 1 {
		clean = "0" + clean
	}
	return hex.DecodeString(clean)
}

func normalizeSNMPHexText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	trimQuotes := func(v string) string {
		return strings.TrimSpace(strings.Trim(v, "\"'"))
	}
	value = trimQuotes(value)
	lower := strings.ToLower(value)
	for _, prefix := range []string{
		"hex-string:",
		"hex string:",
		"octet-string:",
		"octet string:",
		"string:",
	} {
		if strings.HasPrefix(lower, prefix) {
			value = trimQuotes(value[len(prefix):])
			lower = strings.ToLower(value)
		}
	}
	return value
}

func decodeLLDPCapabilities(value string) []string {
	bs, err := decodeHexString(value)
	if err != nil {
		return nil
	}

	names := []string{
		"other",
		"repeater",
		"bridge",
		"wlanAccessPoint",
		"router",
		"telephone",
		"docsisCableDevice",
		"stationOnly",
		"cVlanComponent",
		"sVlanComponent",
		"twoPortMacRelay",
	}

	caps := make([]string, 0, len(names))
	for bit, name := range names {
		if bitSet(bs, bit) {
			caps = append(caps, name)
		}
	}
	return caps
}

func inferCategoryFromCapabilities(caps []string) string {
	has := make(map[string]bool, len(caps))
	for _, c := range caps {
		has[c] = true
	}
	switch {
	case has["router"]:
		return "router"
	case has["wlanAccessPoint"]:
		return "access point"
	case has["telephone"]:
		return "voip"
	case has["bridge"]:
		return "switch"
	case has["repeater"]:
		return "switch"
	case has["docsisCableDevice"]:
		return "network device"
	default:
		return ""
	}
}

func bitSet(bs []byte, bit int) bool {
	idx := bit / 8
	if idx < 0 || idx >= len(bs) {
		return false
	}
	mask := byte(1 << uint(7-(bit%8)))
	return bs[idx]&mask != 0
}

func parseIndex(value string) int {
	if value == "" {
		return 0
	}
	v, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
