// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"
	"strconv"
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"
)

func (c *topologyCache) appendObservedFDBEntries(observation *topologyengine.L2Observation) {
	if observation == nil {
		return
	}

	keys := make([]string, 0, len(c.fdbEntries))
	for key := range c.fdbEntries {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
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
}

func (c *topologyCache) appendObservedSTPPorts(observation *topologyengine.L2Observation) {
	if observation == nil {
		return
	}

	keys := make([]string, 0, len(c.stpPorts))
	for key := range c.stpPorts {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
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
}

func (c *topologyCache) appendObservedARPNDEntries(observation *topologyengine.L2Observation) {
	if observation == nil {
		return
	}

	keys := make([]string, 0, len(c.arpEntries))
	for key := range c.arpEntries {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		entry := c.arpEntries[key]
		if entry == nil {
			continue
		}
		mac := strings.TrimSpace(entry.mac)
		if normalizeMAC(mac) == "" {
			continue // incomplete ARP entry — no MAC means we can't place it on the L2 topology
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
}
