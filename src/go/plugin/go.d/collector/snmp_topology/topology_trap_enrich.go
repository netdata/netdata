// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"
	"strings"
)

type TrapTopologyEnrichment struct {
	DeviceHostname string
	DeviceVendor   string
	SourceVnodeID  string
	Interface      string
	Neighbors      []string
}

// TrapEnrichmentForIP returns topology enrichment data for a trap received from
// the given source IP. Returns nil when no topology state exists for that IP.
// It copies active cache pointers under the registry lock, reads each cache
// under its own lock, and never blocks on I/O.
func TrapEnrichmentForIP(ip string) *TrapTopologyEnrichment {
	ip = normalizeIPAddress(ip)
	if ip == "" {
		return nil
	}

	caches := snmpTopologyRegistry.activeCaches()
	if len(caches) == 0 {
		return nil
	}

	for _, cache := range caches {
		if enrichment := cache.trapEnrichmentForIP(ip); enrichment != nil {
			return enrichment
		}
	}
	return nil
}

func (c *topologyCache) trapEnrichmentForIP(ip string) *TrapTopologyEnrichment {
	c.mu.RLock()
	defer c.mu.RUnlock()

	ifIndex, ok := c.ifIndexByIP[ip]
	if !ok || ifIndex == "" {
		if !c.localDeviceHasIP(ip) {
			return nil
		}
	}

	enrich := &TrapTopologyEnrichment{}

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

	if ifName, ok := c.ifNamesByIndex[ifIndex]; ok && ifName != "" {
		enrich.Interface = ifName
	}

	neighborNames := make(map[string]struct{})
	for _, r := range c.lldpRemotes {
		if r.sysName != "" {
			name := strings.TrimSpace(r.sysName)
			if name != "" {
				neighborNames[name] = struct{}{}
			}
		}
	}
	for _, r := range c.cdpRemotes {
		if r.sysName != "" {
			name := strings.TrimSpace(r.sysName)
			if name != "" {
				neighborNames[name] = struct{}{}
			}
		}
	}
	if len(neighborNames) > 0 {
		enrich.Neighbors = make([]string, 0, len(neighborNames))
		for name := range neighborNames {
			enrich.Neighbors = append(enrich.Neighbors, name)
		}
		sort.Strings(enrich.Neighbors)
	}

	return enrich
}

func (c *topologyCache) localDeviceHasIP(ip string) bool {
	if normalizeIPAddress(c.localDevice.ManagementIP) == ip {
		return true
	}
	for _, addr := range c.localDevice.ManagementAddresses {
		if normalizeIPAddress(addr.Address) == ip {
			return true
		}
	}
	return false
}
