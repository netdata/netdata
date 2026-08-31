// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func newTopologyBuilder() *topologyBuilder {
	return &topologyBuilder{
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
		vlanByFDBID:        make(map[string]fdbVLANMapping),
		vlanNameByID:       make(map[string]vlanNameMapping),
		stpPorts:           make(map[string]*stpPortEntry),
		arpEntries:         make(map[string]*arpEntry),
		ospfNeighborsByKey: make(map[string]topologymodel.OSPFNeighbor),
		bgpPeersByKey:      make(map[string]topologymodel.BGPPeer),
	}
}

func (c *Collector) freezeTopologyBuilder(cache *topologyBuilder) *topologyDeviceSnapshot {
	snapshot, stats := freezeTopologyBuilder(cache)

	if stats.droppedNoMAC > 0 {
		c.Warningf("device '%s': dropped %d topology FDB row(s) with empty MAC", stats.agentID, stats.droppedNoMAC)
	}
	if stats.unmappedPort > 0 {
		c.Warningf("device '%s': observed %d topology FDB row(s) with bridge ports missing ifIndex mapping", stats.agentID, stats.unmappedPort)
	}
	return snapshot
}

type topologyBuilderFinalizeStats struct {
	agentID      string
	droppedNoMAC int
	unmappedPort int
}

func (c *topologyBuilder) finalize() topologyBuilderFinalizeStats {
	if c == nil {
		return topologyBuilderFinalizeStats{}
	}

	finalizeLocalManagementAddresses(&c.localDevice, c.targetManagementIPs, c.ifNetmaskByIP)
	c.localManagementAddressKeys = nil
	c.rebuildTrapSourceMatchMethods()
	c.finalizeFDBVLANs()
	c.updateFDBDiagnostics()
	stats := topologyBuilderFinalizeStats{
		agentID:      c.agentID,
		droppedNoMAC: c.fdbRowsDroppedNoMAC,
		unmappedPort: c.fdbRowsUnmappedPort,
	}
	if !c.updateTime.IsZero() {
		c.lastUpdate = c.updateTime
	}
	c.preparedSnapshot, c.hasPreparedSnapshot = c.buildObservationSnapshot()
	return stats
}

func (c *topologyBuilder) rebuildTrapSourceMatchMethods() {
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

func (c *topologyBuilder) updateFDBDiagnostics() {
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
