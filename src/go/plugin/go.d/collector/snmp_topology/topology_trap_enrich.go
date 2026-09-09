// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"maps"
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

type topologyTrapDeviceGeneration struct {
	matchMethodByIP  map[string]string
	deviceHostname   string
	deviceVendor     string
	sourceVnodeID    string
	interfaceByIndex map[string]string
	neighborsByIndex map[string][]string
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
// IP matches exactly one published device generation. The caller owns the returned
// value and its Neighbors slice.
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

// trapEnrichmentForSource acquires one immutable topology generation and never
// blocks on collection or I/O.
func (r *topologyRegistry) trapEnrichmentForSource(ip, trapIfIndex string) *TrapTopologyEnrichment {
	if r == nil {
		return nil
	}

	addr, ok := topologyutil.ParseIPAddress(ip)
	if !ok {
		return nil
	}
	ip = addr.String()

	generation := r.acquireGeneration()
	if generation == nil {
		return nil
	}

	matches := make([]*TrapTopologyEnrichment, 0, 1)
	for _, device := range generation.devices {
		if device == nil {
			continue
		}
		if enrichment := device.trap.enrichmentForCanonicalSource(ip, trapIfIndex); enrichment != nil {
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

func (g topologyTrapDeviceGeneration) enrichmentForCanonicalSource(ip, trapIfIndex string) *TrapTopologyEnrichment {
	method := g.matchMethodByIP[ip]
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

	enrich.DeviceHostname = g.deviceHostname
	enrich.DeviceVendor = g.deviceVendor
	enrich.SourceVnodeID = g.sourceVnodeID

	if trapIfIndex == "" {
		enrich.InterfaceStatus = "skipped"
		enrich.NeighborStatus = "skipped"
		return enrich
	}

	enrich.InterfaceIndex = trapIfIndex
	enrich.InterfaceStatus = "no_match"
	if ifName := g.interfaceByIndex[trapIfIndex]; ifName != "" {
		enrich.Interface = ifName
		enrich.InterfaceStatus = "matched"
	}

	enrich.NeighborStatus = "no_match"
	enrich.Neighbors = append([]string(nil), g.neighborsByIndex[trapIfIndex]...)
	if len(enrich.Neighbors) > 0 {
		enrich.NeighborStatus = "matched"
	}

	return enrich
}

func newTopologyTrapDeviceGeneration(builder *topologyBuilder) topologyTrapDeviceGeneration {
	if builder == nil {
		return topologyTrapDeviceGeneration{}
	}

	generation := topologyTrapDeviceGeneration{
		matchMethodByIP:  make(map[string]string, len(builder.trapMatchMethodByIP)),
		interfaceByIndex: make(map[string]string, len(builder.ifNamesByIndex)),
		neighborsByIndex: make(map[string][]string),
		deviceHostname:   strings.TrimSpace(builder.localDevice.SysName),
		deviceVendor:     strings.TrimSpace(builder.localDevice.Vendor),
	}
	if nodeID := strings.TrimSpace(builder.localDevice.NetdataHostID); nodeID != "" {
		generation.sourceVnodeID = nodeID
	} else {
		generation.sourceVnodeID = strings.TrimSpace(builder.localDevice.AgentID)
	}
	maps.Copy(generation.matchMethodByIP, builder.trapMatchMethodByIP)
	for ifIndex, ifName := range builder.ifNamesByIndex {
		if ifName = strings.TrimSpace(ifName); ifName != "" {
			generation.interfaceByIndex[ifIndex] = ifName
		}
	}

	neighborSets := make(map[string]map[string]struct{})
	addNeighbor := func(ifIndex, name string) {
		ifIndex = strings.TrimSpace(ifIndex)
		name = strings.TrimSpace(name)
		if ifIndex == "" || name == "" {
			return
		}
		if neighborSets[ifIndex] == nil {
			neighborSets[ifIndex] = make(map[string]struct{})
		}
		neighborSets[ifIndex][name] = struct{}{}
	}
	for key, remote := range builder.lldpRemotes {
		if remote != nil {
			addNeighbor(lldpRemoteLocalPortNum(key, remote), remote.sysName)
		}
	}
	for key, remote := range builder.cdpRemotes {
		if remote != nil {
			addNeighbor(cdpRemoteIfIndex(key, remote), remote.sysName)
		}
	}
	for ifIndex, names := range neighborSets {
		neighbors := make([]string, 0, len(names))
		for name := range names {
			neighbors = append(neighbors, name)
		}
		sort.Strings(neighbors)
		generation.neighborsByIndex[ifIndex] = neighbors
	}
	return generation
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
