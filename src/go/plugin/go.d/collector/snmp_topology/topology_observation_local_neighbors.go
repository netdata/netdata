// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"
)

func (c *topologyCache) appendObservedLLDPRemotes(observation *topologyengine.L2Observation) {
	if observation == nil {
		return
	}

	keys := make([]string, 0, len(c.lldpRemotes))
	for key := range c.lldpRemotes {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
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
}

func (c *topologyCache) appendObservedCDPRemotes(observation *topologyengine.L2Observation) {
	if observation == nil {
		return
	}

	keys := make([]string, 0, len(c.cdpRemotes))
	for key := range c.cdpRemotes {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
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
}
