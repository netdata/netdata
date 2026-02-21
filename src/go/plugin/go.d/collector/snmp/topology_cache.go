// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"encoding/hex"
	"fmt"
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
	metricLldpLocPortEntry     = "lldpLocPortEntry"
	metricLldpLocManAddrEntry  = "lldpLocManAddrEntry"
	metricLldpRemEntry         = "lldpRemEntry"
	metricLldpRemManAddrEntry  = "lldpRemManAddrEntry"
	metricLldpRemManAddrCompat = "lldpRemManAddrCompatEntry"
	metricCdpCacheEntry        = "cdpCacheEntry"
	metricTopologyIfNameEntry  = "topologyIfNameEntry"
	metricBridgePortMapEntry   = "dot1dBasePortIfIndexEntry"
	metricFdbEntry             = "dot1dTpFdbEntry"
	metricArpEntry             = "ipNetToPhysicalEntry"
	metricArpLegacyEntry       = "ipNetToMediaEntry"
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

	tagTopoIfIndex = "topo_if_index"
	tagTopoIfName  = "topo_if_name"

	tagBridgeBasePort = "bridge_base_port"
	tagBridgeIfIndex  = "bridge_if_index"

	tagFdbMac        = "fdb_mac"
	tagFdbBridgePort = "fdb_bridge_port"
	tagFdbStatus     = "fdb_status"

	tagArpIfIndex  = "arp_if_index"
	tagArpIfName   = "arp_if_name"
	tagArpIP       = "arp_ip"
	tagArpMac      = "arp_mac"
	tagArpType     = "arp_type"
	tagArpState    = "arp_state"
	tagArpAddrType = "arp_addr_type"
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

type topologyCache struct {
	mu         sync.RWMutex
	lastUpdate time.Time
	updateTime time.Time

	agentID     string
	localDevice topologyDevice

	lldpLocPorts map[string]*lldpLocPort
	lldpRemotes  map[string]*lldpRemote
	cdpRemotes   map[string]*cdpRemote

	ifNamesByIndex map[string]string
	bridgePortToIf map[string]string
	fdbEntries     map[string]*fdbEntry
	arpEntries     map[string]*arpEntry
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
		lldpLocPorts:   make(map[string]*lldpLocPort),
		lldpRemotes:    make(map[string]*lldpRemote),
		cdpRemotes:     make(map[string]*cdpRemote),
		ifNamesByIndex: make(map[string]string),
		bridgePortToIf: make(map[string]string),
		fdbEntries:     make(map[string]*fdbEntry),
		arpEntries:     make(map[string]*arpEntry),
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
	c.topologyCache.bridgePortToIf = make(map[string]string)
	c.topologyCache.fdbEntries = make(map[string]*fdbEntry)
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

		if v := pm.Tags[tagLldpLocChassisID]; v != "" {
			c.topologyCache.localDevice.ChassisID = v
		}
		if v := pm.Tags[tagLldpLocChassisIDSubtype]; v != "" {
			c.topologyCache.localDevice.ChassisIDType = normalizeLLDPSubtype(v, lldpChassisIDSubtypeMap)
		}
		if v := pm.Tags[tagLldpLocSysName]; v != "" {
			c.topologyCache.localDevice.SysName = v
		}
		if v := pm.Tags[tagLldpLocSysDesc]; v != "" {
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
	case metricBridgePortMapEntry:
		c.topologyCache.updateBridgePortMap(m.Tags)
	case metricFdbEntry:
		c.topologyCache.updateFdbEntry(m.Tags)
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

func isTopologyMetric(name string) bool {
	switch name {
	case metricLldpLocPortEntry, metricLldpLocManAddrEntry, metricLldpRemEntry, metricLldpRemManAddrEntry, metricLldpRemManAddrCompat, metricCdpCacheEntry,
		metricTopologyIfNameEntry, metricBridgePortMapEntry, metricFdbEntry, metricArpEntry, metricArpLegacyEntry:
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
		ManagementIP: c.Hostname,
	}

	if c.sysInfo == nil {
		return device
	}

	device.SysObjectID = c.sysInfo.SysObjectID
	device.SysName = c.sysInfo.Name
	device.SysDescr = c.sysInfo.Descr
	device.SysLocation = c.sysInfo.Location

	if c.sysInfo.Vendor != "" {
		device.Vendor = c.sysInfo.Vendor
	} else if c.sysInfo.Organization != "" {
		device.Vendor = c.sysInfo.Organization
	}
	device.Model = c.sysInfo.Model

	if c.vnode != nil {
		device.AgentID = c.vnode.GUID
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
	if ifName == "" {
		return
	}

	c.ifNamesByIndex[ifIndex] = ifName
}

func (c *topologyCache) updateBridgePortMap(tags map[string]string) {
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
	mac := normalizeMAC(tags[tagFdbMac])
	if mac == "" {
		return
	}

	bridgePort := strings.TrimSpace(tags[tagFdbBridgePort])
	if bridgePort == "" || bridgePort == "0" {
		return
	}

	key := mac + "|" + bridgePort
	entry := c.fdbEntries[key]
	if entry == nil {
		entry = &fdbEntry{
			mac:        mac,
			bridgePort: bridgePort,
		}
		c.fdbEntries[key] = entry
	}

	if v := strings.TrimSpace(tags[tagFdbStatus]); v != "" {
		entry.status = v
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
		SchemaVersion: topologySchemaVersion,
		Source:        "snmp",
		Layer:         "2",
		View:          "summary",
		AgentID:       c.agentID,
		LocalDeviceID: localDeviceID,
		CollectedAt:   c.lastUpdate,
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
	ipToID      map[string]string
	fallbackSeq int
}

func newTopologyObservationIdentityResolver(local topologyengine.L2Observation) *topologyObservationIdentityResolver {
	resolver := &topologyObservationIdentityResolver{
		hostToID:    make(map[string]string),
		chassisToID: make(map[string]string),
		ipToID:      make(map[string]string),
	}
	resolver.register(local.DeviceID, []string{local.Hostname}, local.ChassisID, local.ManagementIP)
	return resolver
}

func (r *topologyObservationIdentityResolver) resolve(hostAliases []string, chassisID, chassisType, managementIP string) string {
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
	id := strings.TrimSpace(ensureTopologyObservationDeviceID(candidate))
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

	observation := topologyengine.L2Observation{
		DeviceID:     ensureTopologyObservationDeviceID(local),
		Hostname:     strings.TrimSpace(local.SysName),
		ManagementIP: localManagementIP,
		SysObjectID:  strings.TrimSpace(local.SysObjectID),
		ChassisID:    strings.TrimSpace(local.ChassisID),
	}

	ifNameKeys := make([]string, 0, len(c.ifNamesByIndex))
	for key := range c.ifNamesByIndex {
		ifNameKeys = append(ifNameKeys, key)
	}
	sort.Strings(ifNameKeys)
	for _, ifIndex := range ifNameKeys {
		ifName := strings.TrimSpace(c.ifNamesByIndex[ifIndex])
		if ifName == "" {
			continue
		}
		idx := parseIndex(ifIndex)
		if idx <= 0 {
			continue
		}
		observation.Interfaces = append(observation.Interfaces, topologyengine.ObservedInterface{
			IfIndex: idx,
			IfName:  ifName,
			IfDescr: ifName,
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
		observation.FDBEntries = append(observation.FDBEntries, topologyengine.FDBObservation{
			MAC:        strings.TrimSpace(entry.mac),
			BridgePort: strings.TrimSpace(entry.bridgePort),
			IfIndex:    ifIndex,
			Status:     strings.TrimSpace(entry.status),
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

func ensureTopologyObservationDeviceID(local topologyDevice) string {
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

func augmentLocalActorFromCache(data *topologyData, local topologyDevice) {
	if data == nil || len(data.Actors) == 0 {
		return
	}

	for i := range data.Actors {
		actor := &data.Actors[i]
		if actor.ActorType != "device" {
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
		if sysLocation := strings.TrimSpace(local.SysLocation); sysLocation != "" {
			attrs["sys_location"] = sysLocation
		}
		if managementIP := normalizeIPAddress(local.ManagementIP); managementIP != "" {
			attrs["management_ip"] = managementIP
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
	if len(match.ChassisIDs) > 0 {
		return "chassis:" + canonicalListKey(match.ChassisIDs)
	}
	if len(match.MacAddresses) > 0 {
		return "mac:" + canonicalListKey(match.MacAddresses)
	}
	if len(match.IPAddresses) > 0 {
		return "ip:" + canonicalListKey(match.IPAddresses)
	}
	if len(match.Hostnames) > 0 {
		return "hostname:" + canonicalListKey(match.Hostnames)
	}
	if len(match.DNSNames) > 0 {
		return "dns:" + canonicalListKey(match.DNSNames)
	}
	if match.SysName != "" {
		return "sysname:" + match.SysName
	}
	if match.SysObjectID != "" {
		return "sysobjectid:" + match.SysObjectID
	}
	return ""
}

func canonicalListKey(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	return strings.Join(out, ",")
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

func normalizeTopologyDevice(dev topologyDevice) topologyDevice {
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
	if dev.ChassisID == "" && dev.ManagementIP != "" {
		dev.ChassisID = dev.ManagementIP
		dev.ChassisIDType = "management_ip"
	}
	if dev.ChassisID != "" && dev.ChassisIDType == "" {
		dev.ChassisIDType = "unknown"
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
	value = strings.TrimSpace(value)
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

func decodeHexString(value string) ([]byte, error) {
	clean := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "0x")
	clean = strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(clean)
	if clean == "" {
		return nil, fmt.Errorf("empty hex string")
	}
	if len(clean)%2 == 1 {
		clean = "0" + clean
	}
	return hex.DecodeString(clean)
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
