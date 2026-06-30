// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"
	"strings"
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

type TrapTopologyEnrichment struct {
	SourceIP        string
	DeviceStatus    string
	DeviceMethod    string
	DeviceMatches   int
	DeviceHostname  string
	DeviceVendor    string
	SourceVnodeID   string
	InterfaceIndex  string
	InterfaceStatus string
	Interface       string
	NeighborStatus  string
	Neighbors       []string
}

// TrapEnrichmentHandle exposes the currently running topology registry to trap enrichment consumers.
type TrapEnrichmentHandle struct {
	registry atomic.Pointer[topologyRegistry]
}

// NewTrapEnrichmentHandle returns an empty process-local trap enrichment handle.
func NewTrapEnrichmentHandle() *TrapEnrichmentHandle {
	return &TrapEnrichmentHandle{}
}

func (c *Collector) publishTrapTopologyEnrichment() {
	if c.trapEnrichment != nil && c.topologyRegistry != nil {
		c.trapEnrichment.registry.Store(c.topologyRegistry)
	}
}

func (c *Collector) unpublishTrapTopologyEnrichment() {
	if c.trapEnrichment != nil && c.topologyRegistry != nil {
		c.trapEnrichment.registry.CompareAndSwap(c.topologyRegistry, nil)
	}
}

// EnrichmentForSource returns topology enrichment data for a trap received
// from the given source IP and, when available, the trap subject ifIndex.
// Interface and neighbor enrichment only use the trap ifIndex after the source
// IP matches exactly one local topology cache.
func (h *TrapEnrichmentHandle) EnrichmentForSource(ip, trapIfIndex string) *TrapTopologyEnrichment {
	if h == nil {
		return nil
	}
	registry := h.registry.Load()
	if registry == nil {
		return nil
	}
	return registry.trapEnrichmentForSource(ip, trapIfIndex)
}

// trapEnrichmentForSource copies active cache pointers under the registry lock,
// reads each cache under its own lock, and never blocks on I/O.
func (r *topologyRegistry) trapEnrichmentForSource(ip, trapIfIndex string) *TrapTopologyEnrichment {
	if r == nil {
		return nil
	}

	ip = topologyutil.NormalizeIPAddress(ip)
	if ip == "" {
		return nil
	}

	caches := r.activeCaches()
	if len(caches) == 0 {
		return nil
	}

	matches := make([]*TrapTopologyEnrichment, 0, 1)
	for _, cache := range caches {
		if enrichment := cache.trapEnrichmentForSource(ip, trapIfIndex); enrichment != nil {
			matches = append(matches, enrichment)
		}
	}
	if len(matches) != 1 {
		status := "no_match"
		if len(matches) > 1 {
			status = "ambiguous"
		}
		return &TrapTopologyEnrichment{
			SourceIP:      ip,
			DeviceStatus:  status,
			DeviceMatches: len(matches),
		}
	}
	matches[0].DeviceMatches = 1
	return matches[0]
}

func (c *topologyCache) trapEnrichmentForSource(ip, trapIfIndex string) *TrapTopologyEnrichment {
	c.mu.RLock()
	defer c.mu.RUnlock()

	method := c.localDeviceIPMatchMethod(ip)
	if method == "" {
		return nil
	}

	trapIfIndex = strings.TrimSpace(trapIfIndex)
	enrich := &TrapTopologyEnrichment{
		SourceIP:      ip,
		DeviceStatus:  "matched",
		DeviceMethod:  method,
		DeviceMatches: 1,
	}

	if sysName := strings.TrimSpace(c.localDevice.SysName); sysName != "" {
		enrich.DeviceHostname = sysName
	}
	if vendor := strings.TrimSpace(c.localDevice.Vendor); vendor != "" {
		enrich.DeviceVendor = vendor
	}
	if nodeID := strings.TrimSpace(c.localDevice.NetdataHostID); nodeID != "" {
		enrich.SourceVnodeID = nodeID
	} else if nodeID := strings.TrimSpace(c.localDevice.AgentID); nodeID != "" {
		enrich.SourceVnodeID = nodeID
	}

	if trapIfIndex == "" {
		enrich.InterfaceStatus = "skipped"
		enrich.NeighborStatus = "skipped"
		return enrich
	}

	enrich.InterfaceIndex = trapIfIndex
	enrich.InterfaceStatus = "no_match"
	if ifName, ok := c.ifNamesByIndex[trapIfIndex]; ok && strings.TrimSpace(ifName) != "" {
		enrich.Interface = ifName
		enrich.InterfaceStatus = "matched"
	}

	enrich.NeighborStatus = "no_match"
	enrich.Neighbors = c.trapNeighborNamesForInterface(trapIfIndex)
	if len(enrich.Neighbors) > 0 {
		enrich.NeighborStatus = "matched"
	}

	return enrich
}

func (c *topologyCache) trapNeighborNamesForInterface(ifIndex string) []string {
	neighborNames := make(map[string]struct{})
	for key, r := range c.lldpRemotes {
		if r == nil || lldpRemoteLocalPortNum(key, r) != ifIndex {
			continue
		}
		if name := strings.TrimSpace(r.sysName); name != "" {
			neighborNames[name] = struct{}{}
		}
	}
	for key, r := range c.cdpRemotes {
		if r == nil || cdpRemoteIfIndex(key, r) != ifIndex {
			continue
		}
		if name := strings.TrimSpace(r.sysName); name != "" {
			neighborNames[name] = struct{}{}
		}
	}
	if len(neighborNames) == 0 {
		return nil
	}

	neighbors := make([]string, 0, len(neighborNames))
	for name := range neighborNames {
		neighbors = append(neighbors, name)
	}
	sort.Strings(neighbors)
	return neighbors
}

func lldpRemoteLocalPortNum(key string, r *lldpRemote) string {
	if r != nil && strings.TrimSpace(r.localPortNum) != "" {
		return strings.TrimSpace(r.localPortNum)
	}
	if before, _, ok := strings.Cut(key, ":"); ok {
		return strings.TrimSpace(before)
	}
	return ""
}

func cdpRemoteIfIndex(key string, r *cdpRemote) string {
	if r != nil && strings.TrimSpace(r.ifIndex) != "" {
		return strings.TrimSpace(r.ifIndex)
	}
	if before, _, ok := strings.Cut(key, ":"); ok {
		return strings.TrimSpace(before)
	}
	return ""
}

func (c *topologyCache) localDeviceIPMatchMethod(ip string) string {
	if topologyutil.NormalizeIPAddress(c.localDevice.ManagementIP) == ip {
		return "management_ip"
	}
	for _, addr := range c.localDevice.ManagementAddresses {
		if topologyutil.NormalizeIPAddress(addr.Address) == ip {
			return "management_address"
		}
	}
	if _, ok := c.ifIndexByIP[ip]; ok {
		return "local_interface_ip"
	}
	return ""
}
