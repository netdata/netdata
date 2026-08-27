// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
)

func (c *topologyBuilder) appendObservedInterfaces(observation *topologyengine.L2Observation) {
	if observation == nil {
		return
	}

	ifaceKeys := make(map[string]struct{}, len(c.ifNamesByIndex)+len(c.ifStatusByIndex))
	if !c.chargeWork(uint64(len(c.ifNamesByIndex) + len(c.ifStatusByIndex))) {
		return
	}
	for key := range c.ifNamesByIndex {
		ifaceKeys[key] = struct{}{}
	}
	for key := range c.ifStatusByIndex {
		ifaceKeys[key] = struct{}{}
	}

	var ifaceKeyList []string
	if c.workLimiter == nil {
		ifaceKeyList = make([]string, 0, len(ifaceKeys))
	}
	ifaceKeyList = sortedBuilderKeys(c, ifaceKeys, ifaceKeyList)
	if !c.chargeWork(uint64(len(ifaceKeyList))) {
		return
	}

	for _, ifIndex := range ifaceKeyList {
		idx := topologyutil.ParseIndex(ifIndex)
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

func (c *topologyBuilder) appendObservedBridgePorts(observation *topologyengine.L2Observation) {
	if observation == nil {
		return
	}

	var keys []string
	if c.workLimiter == nil {
		keys = make([]string, 0, len(c.bridgePortToIf))
	}
	keys = sortedBuilderKeys(c, c.bridgePortToIf, keys)
	if !c.chargeWork(uint64(len(keys))) {
		return
	}

	for _, basePort := range keys {
		ifIndex := topologyutil.ParseIndex(c.bridgePortToIf[basePort])
		if ifIndex <= 0 {
			continue
		}
		observation.BridgePorts = append(observation.BridgePorts, topologyengine.BridgePortObservation{
			BasePort: strings.TrimSpace(basePort),
			IfIndex:  ifIndex,
		})
	}
}
