// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"strconv"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

const (
	metricLldpLocPortEntry = "lldpLocPortEntry"
	metricLldpRemEntry     = "lldpRemEntry"
	metricCdpCacheEntry    = "cdpCacheEntry"
)

const (
	tagLldpLocChassisID        = "lldp_loc_chassis_id"
	tagLldpLocChassisIDSubtype = "lldp_loc_chassis_id_subtype"
	tagLldpLocSysName          = "lldp_loc_sys_name"
	tagLldpLocSysDesc          = "lldp_loc_sys_desc"

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

	tagCdpIfIndex     = "cdp_if_index"
	tagCdpIfName      = "cdp_if_name"
	tagCdpDeviceIndex = "cdp_device_index"
	tagCdpDeviceID    = "cdp_device_id"
	tagCdpDevicePort  = "cdp_device_port"
	tagCdpPlatform    = "cdp_platform"
	tagCdpCaps        = "cdp_capabilities"
	tagCdpAddress     = "cdp_address"
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
	managementAddr     string
	managementAddrType string
}

type cdpRemote struct {
	ifIndex     string
	ifName      string
	deviceIndex string
	deviceID    string
	devicePort  string
	platform    string
	capabilities string
	address     string
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
	case metricLldpRemEntry:
		c.topologyCache.updateLldpRemote(m.Tags)
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
	case metricLldpLocPortEntry, metricLldpRemEntry, metricCdpCacheEntry:
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
	if v := tags[tagLldpRemMgmtAddr]; v != "" {
		entry.managementAddr = v
	}
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
	if v := tags[tagCdpDevicePort]; v != "" {
		entry.devicePort = v
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

	links := make([]topologyLink, 0, len(c.lldpRemotes)+len(c.cdpRemotes))
	linksLLDP := 0
	linksCDP := 0

	for _, rem := range c.lldpRemotes {
		remoteDev := topologyDevice{
			ChassisID:     rem.chassisID,
			ChassisIDType: rem.chassisIDSubtype,
			SysName:       rem.sysName,
			SysDescr:      rem.sysDesc,
			ManagementIP:  rem.managementAddr,
			Discovered:    true,
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
			ChassisID:     remoteDev.ChassisID,
			ChassisIDType: remoteDev.ChassisIDType,
			PortID:        rem.portID,
			PortIDType:    rem.portIDSubtype,
			PortDescr:     rem.portDesc,
			SysName:       rem.sysName,
			ManagementIP:  rem.managementAddr,
		}

		links = append(links, topologyLink{
			Protocol:     "lldp",
			Src:          src,
			Dst:          dst,
			DiscoveredAt: c.lastUpdate,
			LastSeen:     c.lastUpdate,
		})
		linksLLDP++
	}

	for _, rem := range c.cdpRemotes {
		remoteDev := topologyDevice{
			ChassisID:     rem.deviceID,
			ChassisIDType: "cdp_device_id",
			SysName:       rem.deviceID,
			ManagementIP:  rem.address,
			Vendor:        "",
			Model:         rem.platform,
			Discovered:    true,
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
			SysName:       rem.deviceID,
			ManagementIP:  rem.address,
		}

		links = append(links, topologyLink{
			Protocol:     "cdp",
			Src:          src,
			Dst:          dst,
			DiscoveredAt: c.lastUpdate,
			LastSeen:     c.lastUpdate,
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

	return topologyData{
		SchemaVersion: topologySchemaVersion,
		AgentID:       c.agentID,
		CollectedAt:   c.lastUpdate,
		Devices:       devices,
		Links:         links,
		Stats:         stats,
	}, true
}

func normalizeTopologyDevice(dev topologyDevice) topologyDevice {
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
