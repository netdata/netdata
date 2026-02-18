// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

const (
	metricLldpLocPortEntry    = "lldpLocPortEntry"
	metricLldpLocManAddrEntry = "lldpLocManAddrEntry"
	metricLldpRemEntry        = "lldpRemEntry"
	metricLldpRemManAddrEntry = "lldpRemManAddrEntry"
	metricCdpCacheEntry       = "cdpCacheEntry"
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

func newTopologyCache() *topologyCache {
	return &topologyCache{
		lldpLocPorts: make(map[string]*lldpLocPort),
		lldpRemotes:  make(map[string]*lldpRemote),
		cdpRemotes:   make(map[string]*cdpRemote),
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
	case metricLldpRemManAddrEntry:
		c.topologyCache.updateLldpRemManAddr(m.Tags)
	case metricCdpCacheEntry:
		c.topologyCache.updateCdpRemote(m.Tags)
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
	case metricLldpLocPortEntry, metricLldpLocManAddrEntry, metricLldpRemEntry, metricLldpRemManAddrEntry, metricCdpCacheEntry:
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

func (c *topologyCache) snapshot() (topologyData, bool) {
	if c.lastUpdate.IsZero() {
		return topologyData{}, false
	}

	local := c.localDevice
	local = normalizeTopologyDevice(local)

	devices := make([]topologyDevice, 0, 1+len(c.lldpRemotes)+len(c.cdpRemotes))
	deviceIndex := make(map[string]struct{})

	addDevice := func(dev topologyDevice) {
		dev = normalizeTopologyDevice(dev)
		key := topologyDeviceKey(dev)
		if key == "" {
			return
		}
		if _, ok := deviceIndex[key]; ok {
			return
		}
		deviceIndex[key] = struct{}{}
		devices = append(devices, dev)
	}

	addDevice(local)

	links := make([]topology.Link, 0, len(c.lldpRemotes)+len(c.cdpRemotes))
	linksLLDP := 0
	linksCDP := 0

	for _, rem := range c.lldpRemotes {
		remoteDev := topologyDevice{
			ChassisID:             rem.chassisID,
			ChassisIDType:         rem.chassisIDSubtype,
			SysName:               rem.sysName,
			SysDescr:              rem.sysDesc,
			ManagementIP:          rem.managementAddr,
			ManagementAddresses:   rem.managementAddrs,
			CapabilitiesSupported: decodeLLDPCapabilities(rem.sysCapSupported),
			CapabilitiesEnabled:   decodeLLDPCapabilities(rem.sysCapEnabled),
			Discovered:            true,
		}
		if rem.sysCapSupported != "" || rem.sysCapEnabled != "" {
			remoteDev.Labels = ensureLabels(remoteDev.Labels)
			if rem.sysCapSupported != "" {
				remoteDev.Labels[tagLldpRemSysCapSupported] = rem.sysCapSupported
			}
			if rem.sysCapEnabled != "" {
				remoteDev.Labels[tagLldpRemSysCapEnabled] = rem.sysCapEnabled
			}
		}
		remoteDev = normalizeTopologyDevice(remoteDev)
		if topologyDeviceKey(remoteDev) == "" {
			continue
		}

		addDevice(remoteDev)

		localPort := c.lldpLocPorts[rem.localPortNum]
		src := topologyEndpoint{
			ChassisID:     local.ChassisID,
			ChassisIDType: local.ChassisIDType,
		}
		if localPort != nil {
			src.PortID = localPort.portID
			src.PortIDType = localPort.portIDSubtype
			src.PortDescr = localPort.portDesc
		}
		if idx := parseIndex(rem.localPortNum); idx > 0 {
			src.IfIndex = idx
		}

		dst := topologyEndpoint{
			ChassisID:           remoteDev.ChassisID,
			ChassisIDType:       remoteDev.ChassisIDType,
			PortID:              rem.portID,
			PortIDType:          rem.portIDSubtype,
			PortDescr:           rem.portDesc,
			SysName:             rem.sysName,
			ManagementIP:        rem.managementAddr,
			ManagementAddresses: rem.managementAddrs,
		}

		links = append(links, topology.Link{
			Layer:        "2",
			Protocol:     "lldp",
			Direction:    "unidirectional",
			Src:          endpointToLink(src),
			Dst:          endpointToLink(dst),
			DiscoveredAt: timePtr(c.lastUpdate),
			LastSeen:     timePtr(c.lastUpdate),
		})
		linksLLDP++
	}

	for _, rem := range c.cdpRemotes {
		sysName := rem.deviceID
		if rem.sysName != "" {
			sysName = rem.sysName
		}

		remoteDev := topologyDevice{
			ChassisID:           rem.deviceID,
			ChassisIDType:       "cdp_device_id",
			SysObjectID:         rem.sysObjectID,
			SysName:             sysName,
			ManagementIP:        rem.address,
			ManagementAddresses: rem.managementAddrs,
			Vendor:              "",
			Model:               rem.platform,
			Discovered:          true,
		}
		remoteDev.Labels = ensureLabels(remoteDev.Labels)
		if rem.version != "" {
			remoteDev.Labels[tagCdpVersion] = rem.version
		}
		if rem.vtpMgmtDomain != "" {
			remoteDev.Labels[tagCdpVTPDomain] = rem.vtpMgmtDomain
		}
		if rem.physicalLocation != "" {
			remoteDev.Labels[tagCdpPhysicalLocation] = rem.physicalLocation
		}
		if rem.lastChange != "" {
			remoteDev.Labels[tagCdpLastChange] = rem.lastChange
		}
		if rem.addressType != "" {
			remoteDev.Labels[tagCdpAddressType] = rem.addressType
		}
		if rem.capabilities != "" {
			remoteDev.Labels[tagCdpCaps] = rem.capabilities
		}
		if rem.platform != "" {
			remoteDev.Labels[tagCdpPlatform] = rem.platform
		}
		if rem.sysName != "" {
			remoteDev.Labels[tagCdpSysName] = rem.sysName
		}
		if rem.sysObjectID != "" {
			remoteDev.Labels[tagCdpSysObjectID] = rem.sysObjectID
		}
		if remoteDev.ChassisID == "" && rem.address != "" {
			remoteDev.ChassisID = rem.address
			remoteDev.ChassisIDType = "management_ip"
		}
		remoteDev = normalizeTopologyDevice(remoteDev)
		if topologyDeviceKey(remoteDev) == "" {
			continue
		}

		addDevice(remoteDev)

		src := topologyEndpoint{
			ChassisID:     local.ChassisID,
			ChassisIDType: local.ChassisIDType,
			IfName:        rem.ifName,
		}
		if idx := parseIndex(rem.ifIndex); idx > 0 {
			src.IfIndex = idx
		}

		dst := topologyEndpoint{
			ChassisID:     remoteDev.ChassisID,
			ChassisIDType: remoteDev.ChassisIDType,
			PortID:        rem.devicePort,
			SysName:       sysName,
			ManagementIP:  rem.address,
		}
		dst.ManagementAddresses = rem.managementAddrs
		if rem.nativeVLAN != "" || rem.duplex != "" || rem.mtu != "" || rem.powerConsumption != "" {
			dst.Labels = ensureLabels(dst.Labels)
			if rem.nativeVLAN != "" {
				dst.Labels[tagCdpNativeVLAN] = rem.nativeVLAN
			}
			if rem.duplex != "" {
				dst.Labels[tagCdpDuplex] = rem.duplex
			}
			if rem.mtu != "" {
				dst.Labels[tagCdpMTU] = rem.mtu
			}
			if rem.powerConsumption != "" {
				dst.Labels[tagCdpPower] = rem.powerConsumption
			}
		}

		links = append(links, topology.Link{
			Layer:        "2",
			Protocol:     "cdp",
			Direction:    "unidirectional",
			Src:          endpointToLink(src),
			Dst:          endpointToLink(dst),
			DiscoveredAt: timePtr(c.lastUpdate),
			LastSeen:     timePtr(c.lastUpdate),
		})
		linksCDP++
	}

	stats := map[string]any{
		"devices_total":      len(devices),
		"devices_discovered": maxInt(len(devices)-1, 0),
		"links_total":        len(links),
		"links_lldp":         linksLLDP,
		"links_cdp":          linksCDP,
	}

	actors := make([]topology.Actor, 0, len(devices))
	actorIndex := make(map[string]struct{})
	for _, dev := range devices {
		act := deviceToActor(dev, c.agentID)
		key := actorKey(act.Match)
		if key == "" {
			continue
		}
		if _, ok := actorIndex[key]; ok {
			continue
		}
		actorIndex[key] = struct{}{}
		actors = append(actors, act)
	}

	return topologyData{
		SchemaVersion: topologySchemaVersion,
		Source:        "snmp",
		Layer:         "2",
		AgentID:       c.agentID,
		CollectedAt:   c.lastUpdate,
		View:          "summary",
		Actors:        actors,
		Links:         links,
		Stats:         stats,
	}, true
}

func deviceToActor(dev topologyDevice, agentID string) topology.Actor {
	match := topology.Match{
		SysObjectID: dev.SysObjectID,
		SysName:     dev.SysName,
	}
	if dev.ChassisID != "" {
		match.ChassisIDs = []string{dev.ChassisID}
		if strings.EqualFold(dev.ChassisIDType, "macAddress") {
			match.MacAddresses = append(match.MacAddresses, dev.ChassisID)
		}
	}

	if dev.ManagementIP != "" {
		match.IPAddresses = append(match.IPAddresses, dev.ManagementIP)
	}
	for _, addr := range dev.ManagementAddresses {
		if ip := net.ParseIP(addr.Address); ip != nil {
			match.IPAddresses = append(match.IPAddresses, ip.String())
		}
	}
	match.IPAddresses = uniqueStrings(match.IPAddresses)

	attrs := map[string]any{
		"chassis_id_type":        dev.ChassisIDType,
		"sys_descr":              dev.SysDescr,
		"sys_location":           dev.SysLocation,
		"management_ip":          dev.ManagementIP,
		"management_addresses":   dev.ManagementAddresses,
		"agent_id":               agentID,
		"agent_job_id":           dev.AgentJobID,
		"vendor":                 dev.Vendor,
		"model":                  dev.Model,
		"capabilities":           dev.Capabilities,
		"capabilities_supported": dev.CapabilitiesSupported,
		"capabilities_enabled":   dev.CapabilitiesEnabled,
		"discovered":             dev.Discovered,
	}

	return topology.Actor{
		ActorType:  "device",
		Layer:      "2",
		Source:     "snmp",
		Match:      match,
		Attributes: pruneNilAttributes(attrs),
		Labels:     dev.Labels,
	}
}

func endpointToLink(ep topologyEndpoint) topology.LinkEndpoint {
	match := topology.Match{}
	if ep.ChassisID != "" {
		match.ChassisIDs = []string{ep.ChassisID}
		if strings.EqualFold(ep.ChassisIDType, "macAddress") {
			match.MacAddresses = append(match.MacAddresses, ep.ChassisID)
		}
	}
	if ep.ManagementIP != "" {
		match.IPAddresses = append(match.IPAddresses, ep.ManagementIP)
	}
	for _, addr := range ep.ManagementAddresses {
		if ip := net.ParseIP(addr.Address); ip != nil {
			match.IPAddresses = append(match.IPAddresses, ip.String())
		}
	}
	match.IPAddresses = uniqueStrings(match.IPAddresses)

	attrs := map[string]any{
		"chassis_id_type":      ep.ChassisIDType,
		"if_index":             ep.IfIndex,
		"if_name":              ep.IfName,
		"port_id":              ep.PortID,
		"port_id_type":         ep.PortIDType,
		"port_descr":           ep.PortDescr,
		"sys_name":             ep.SysName,
		"management_ip":        ep.ManagementIP,
		"management_addresses": ep.ManagementAddresses,
		"agent_id":             ep.AgentID,
		"labels":               ep.Labels,
	}

	return topology.LinkEndpoint{
		Match:      match,
		Attributes: pruneNilAttributes(attrs),
	}
}

func actorKey(match topology.Match) string {
	if len(match.ChassisIDs) > 0 {
		return "chassis:" + strings.Join(match.ChassisIDs, ",")
	}
	if len(match.MacAddresses) > 0 {
		return "mac:" + strings.Join(match.MacAddresses, ",")
	}
	if len(match.IPAddresses) > 0 {
		return "ip:" + strings.Join(match.IPAddresses, ",")
	}
	if match.SysName != "" {
		return "sysname:" + match.SysName
	}
	return ""
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
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

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
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
	for _, addr := range addrs {
		if ip := net.ParseIP(addr.Address); ip != nil {
			return ip.String()
		}
	}
	for _, addr := range addrs {
		if addr.Address != "" {
			return addr.Address
		}
	}
	return ""
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
		if len(bs) == net.IPv4len || len(bs) == net.IPv6len {
			ip := net.IP(bs)
			return ip.String(), normalizeAddressType(rawType, ip.String())
		}
	}

	return rawAddr, normalizeAddressType(rawType, rawAddr)
}

func normalizeAddressType(rawType, addr string) string {
	switch rawType {
	case "2":
		return "ipv4"
	case "3":
		return "ipv6"
	case "1":
		if ip := net.ParseIP(addr); ip != nil && ip.To4() != nil {
			return "ipv4"
		}
	}
	if ip := net.ParseIP(addr); ip != nil {
		if ip.To4() != nil {
			return "ipv4"
		}
		return "ipv6"
	}
	return rawType
}

func decodeHexString(value string) ([]byte, error) {
	clean := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "0x")
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
