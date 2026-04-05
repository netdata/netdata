// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"
)

func (c *topologyCache) appendObservedInterfaces(observation *topologyengine.L2Observation) {
	if observation == nil {
		return
	}

	ifaceKeys := make(map[string]struct{}, len(c.ifNamesByIndex)+len(c.ifStatusByIndex))
	for key := range c.ifNamesByIndex {
		ifaceKeys[key] = struct{}{}
	}
	for key := range c.ifStatusByIndex {
		ifaceKeys[key] = struct{}{}
	}

	ifaceKeyList := make([]string, 0, len(ifaceKeys))
	for key := range ifaceKeys {
		ifaceKeyList = append(ifaceKeyList, key)
	}
	sort.Strings(ifaceKeyList)

	for _, ifIndex := range ifaceKeyList {
		idx := parseIndex(ifIndex)
		if idx <= 0 {
			continue
		}
		ifName := strings.TrimSpace(c.ifNamesByIndex[ifIndex])
		if ifName == "" {
			ifName = ifIndex
		}
		status := c.ifStatusByIndex[ifIndex]
		ifDescr := strings.TrimSpace(status.ifDescr)
		if ifDescr == "" {
			ifDescr = ifName
		}
		observation.Interfaces = append(observation.Interfaces, topologyengine.ObservedInterface{
			IfIndex:       idx,
			IfName:        ifName,
			IfDescr:       ifDescr,
			IfAlias:       strings.TrimSpace(status.ifAlias),
			MAC:           strings.TrimSpace(status.mac),
			SpeedBps:      status.speedBps,
			LastChange:    status.lastChange,
			Duplex:        strings.TrimSpace(status.duplex),
			InterfaceType: strings.TrimSpace(status.ifType),
			AdminStatus:   strings.TrimSpace(status.admin),
			OperStatus:    strings.TrimSpace(status.oper),
		})
	}
}

func (c *topologyCache) appendObservedBridgePorts(observation *topologyengine.L2Observation) {
	if observation == nil {
		return
	}

	keys := make([]string, 0, len(c.bridgePortToIf))
	for key := range c.bridgePortToIf {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, basePort := range keys {
		ifIndex := parseIndex(c.bridgePortToIf[basePort])
		if ifIndex <= 0 {
			continue
		}
		observation.BridgePorts = append(observation.BridgePorts, topologyengine.BridgePortObservation{
			BasePort: strings.TrimSpace(basePort),
			IfIndex:  ifIndex,
		})
	}
}
