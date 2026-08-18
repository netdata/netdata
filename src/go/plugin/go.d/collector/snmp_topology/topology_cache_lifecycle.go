// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func newTopologyCache() *topologyCache {
	return &topologyCache{
		lldpLocPorts:       make(map[string]*lldpLocPort),
		lldpRemotes:        make(map[string]*lldpRemote),
		cdpRemotes:         make(map[string]*cdpRemote),
		ifNamesByIndex:     make(map[string]string),
		ifStatusByIndex:    make(map[string]ifStatus),
		ifIndexByIP:        make(map[string]string),
		ifNetmaskByIP:      make(map[string]string),
		l3InterfacesByIP:   make(map[string]topologymodel.L3Interface),
		bridgePortToIf:     make(map[string]string),
		fdbEntries:         make(map[string]*fdbEntry),
		fdbIDToVlanID:      make(map[string]string),
		vlanIDToName:       make(map[string]string),
		stpPorts:           make(map[string]*stpPortEntry),
		arpEntries:         make(map[string]*arpEntry),
		ospfNeighborsByKey: make(map[string]topologymodel.OSPFNeighbor),
		bgpPeersByKey:      make(map[string]topologymodel.BGPPeer),
	}
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
	c.l3InterfacesByIP = src.l3InterfacesByIP
	c.trapMatchMethodByIP = src.trapMatchMethodByIP
	c.bridgePortToIf = src.bridgePortToIf
	c.fdbEntries = src.fdbEntries
	c.fdbIDToVlanID = src.fdbIDToVlanID
	c.vlanIDToName = src.vlanIDToName
	c.fdbRowsDroppedNoMAC = src.fdbRowsDroppedNoMAC
	c.fdbRowsUnmappedPort = src.fdbRowsUnmappedPort
	c.vtpVersion = src.vtpVersion
	c.stpBaseBridgeAddress = src.stpBaseBridgeAddress
	c.stpDesignatedRoot = src.stpDesignatedRoot
	c.stpPorts = src.stpPorts
	c.arpEntries = src.arpEntries
	c.ospfNeighborsByKey = src.ospfNeighborsByKey
	c.bgpPeersByKey = src.bgpPeersByKey
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

func (c *topologyCache) hasRenderableObservationAt(now time.Time) bool {
	if c == nil {
		return false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.hasFreshSnapshotAt(now) {
		return false
	}

	local := normalizeTopologyDevice(c.localDevice)
	localManagementIP := topologyutil.NormalizeIPAddress(local.ManagementIP)
	baseBridgeAddress := c.resolveLocalBaseBridgeAddress(localManagementIP)
	return strings.TrimSpace(ensureTopologyObservationDeviceID(local, baseBridgeAddress)) != ""
}

func (c *Collector) finalizeTopologyCache(cache *topologyCache) {
	if cache == nil {
		return
	}

	stats := cache.finalizeTopologyCache()
	c.logTemporaryTopologyAliasEvidence(cache)

	if stats.droppedNoMAC > 0 {
		c.Warningf("device '%s': dropped %d topology FDB row(s) with empty MAC", stats.agentID, stats.droppedNoMAC)
	}
	if stats.unmappedPort > 0 {
		c.Warningf("device '%s': observed %d topology FDB row(s) with bridge ports missing ifIndex mapping", stats.agentID, stats.unmappedPort)
	}
}

const temporaryTopologyAliasTraceIP = "8.8.8.8"

func (c *Collector) logTemporaryTopologyAliasEvidence(cache *topologyCache) {
	cache.mu.RLock()
	defer cache.mu.RUnlock()

	reporter := strings.TrimSpace(cache.agentID)
	sysName := strings.TrimSpace(cache.localDevice.SysName)
	trace := func(source, ip, mac, detail string) {
		c.Warningf(
			"TEMP topology alias trace: reporter=%q sys_name=%q source=%q ip=%q mac=%q detail=%q",
			reporter,
			sysName,
			source,
			ip,
			mac,
			detail,
		)
	}

	if topologyutil.NormalizeIPAddress(cache.localDevice.ManagementIP) == temporaryTopologyAliasTraceIP {
		trace("selected_management", temporaryTopologyAliasTraceIP, "", "")
	}
	for _, address := range cache.localDevice.ManagementAddresses {
		if addr, ok := topologymodel.ParseManagementAddressIP(address); ok && addr.String() == temporaryTopologyAliasTraceIP {
			trace("management_address", addr.String(), "", address.Source)
		}
	}
	if ifIndex, ok := cache.ifIndexByIP[temporaryTopologyAliasTraceIP]; ok {
		trace("ip_mib", temporaryTopologyAliasTraceIP, "", ifIndex)
	}
	for _, entry := range cache.arpEntries {
		if entry != nil && topologyutil.NormalizeIPAddress(entry.ip) == temporaryTopologyAliasTraceIP {
			trace("arp", temporaryTopologyAliasTraceIP, topologyutil.NormalizeMAC(entry.mac), entry.ifIndex+"|"+entry.ifName)
		}
	}
	for _, remote := range cache.lldpRemotes {
		if remote == nil {
			continue
		}
		for _, address := range remote.managementAddrs {
			if addr, ok := topologymodel.ParseManagementAddressIP(address); ok && addr.String() == temporaryTopologyAliasTraceIP {
				trace("lldp", addr.String(), topologyutil.NormalizeMAC(remote.chassisID), remote.sysName)
			}
		}
	}
	for _, remote := range cache.cdpRemotes {
		if remote == nil {
			continue
		}
		for _, address := range remote.managementAddrs {
			if addr, ok := topologymodel.ParseManagementAddressIP(address); ok && addr.String() == temporaryTopologyAliasTraceIP {
				trace("cdp", addr.String(), "", remote.deviceID+"|"+remote.sysName)
			}
		}
	}
}

type topologyCacheFinalizeStats struct {
	agentID      string
	droppedNoMAC int
	unmappedPort int
}

func (c *topologyCache) finalizeTopologyCache() topologyCacheFinalizeStats {
	if c == nil {
		return topologyCacheFinalizeStats{}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	finalizeLocalManagementAddresses(&c.localDevice, c.targetManagementIPs, c.ifNetmaskByIP)
	c.rebuildTrapSourceMatchMethods()
	c.updateFDBDiagnostics()
	stats := topologyCacheFinalizeStats{
		agentID:      c.agentID,
		droppedNoMAC: c.fdbRowsDroppedNoMAC,
		unmappedPort: c.fdbRowsUnmappedPort,
	}
	c.lastUpdate = c.updateTime
	return stats
}

func (c *topologyCache) rebuildTrapSourceMatchMethods() {
	methods := make(map[string]string, len(c.ifIndexByIP)+len(c.localDevice.ManagementAddresses)+1)
	for value := range c.ifIndexByIP {
		if addr, ok := topologyutil.ParseIPAddress(value); ok {
			methods[addr.String()] = "local_interface_ip"
		}
	}
	for _, address := range c.localDevice.ManagementAddresses {
		if addr, ok := topologymodel.ParseManagementAddressIP(address); ok && !addr.IsUnspecified() {
			methods[addr.String()] = "management_address"
		}
	}
	if addr, ok := topologyutil.ParseIPAddress(c.localDevice.ManagementIP); ok {
		methods[addr.String()] = "management_ip"
	}
	c.trapMatchMethodByIP = methods
}

func (c *topologyCache) updateFDBDiagnostics() {
	c.fdbRowsUnmappedPort = 0
	for _, entry := range c.fdbEntries {
		if entry == nil || strings.TrimSpace(entry.mac) == "" {
			continue
		}
		bridgePort := strings.TrimSpace(entry.bridgePort)
		if bridgePort == "" || bridgePort == "0" {
			continue
		}
		if topologyutil.ParseIndex(c.bridgePortToIf[bridgePort]) == 0 {
			c.fdbRowsUnmappedPort++
		}
	}
}
