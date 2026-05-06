// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"
	"time"
)

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
	c.fdbRowsDroppedNoMAC = src.fdbRowsDroppedNoMAC
	c.fdbRowsUnmappedPort = src.fdbRowsUnmappedPort
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

func (c *Collector) finalizeTopologyCache() {
	cache := c.topologyCache
	if cache == nil {
		return
	}

	cache.mu.Lock()
	cache.updateFDBDiagnostics()
	droppedNoMAC := cache.fdbRowsDroppedNoMAC
	unmappedPort := cache.fdbRowsUnmappedPort
	agentID := cache.agentID
	cache.lastUpdate = cache.updateTime
	cache.mu.Unlock()

	if droppedNoMAC > 0 {
		c.Warningf("device '%s': dropped %d topology FDB row(s) with empty MAC", agentID, droppedNoMAC)
	}
	if unmappedPort > 0 {
		c.Warningf("device '%s': observed %d topology FDB row(s) with bridge ports missing ifIndex mapping", agentID, unmappedPort)
	}
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
		if parseIndex(c.bridgePortToIf[bridgePort]) == 0 {
			c.fdbRowsUnmappedPort++
		}
	}
}
