// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func (c *topologyBuilder) appendObservedLLDPRemotes(observation *topologyengine.L2Observation) {
	if observation == nil {
		return
	}

	var keys []string
	if c.workLimiter == nil {
		keys = make([]string, 0, len(c.lldpRemotes))
	}
	keys = sortedBuilderKeys(c, c.lldpRemotes, keys)
	if !c.chargeWork(uint64(len(keys))) {
		return
	}

	for _, key := range keys {
		remote := c.lldpRemotes[key]
		if remote == nil {
			continue
		}

		managementIP := pickManagementIP(remote.managementAddrs)

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

func (c *topologyBuilder) appendObservedCDPRemotes(observation *topologyengine.L2Observation) {
	if observation == nil {
		return
	}

	var keys []string
	if c.workLimiter == nil {
		keys = make([]string, 0, len(c.cdpRemotes))
	}
	keys = sortedBuilderKeys(c, c.cdpRemotes, keys)
	if !c.chargeWork(uint64(len(keys))) {
		return
	}

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

		address := pickManagementIP(remote.managementAddrs)

		observation.CDPRemotes = append(observation.CDPRemotes, topologyengine.CDPRemoteObservation{
			LocalIfIndex: topologyutil.ParseIndex(remote.ifIndex),
			LocalIfName:  ifName,
			DeviceIndex:  strings.TrimSpace(remote.deviceIndex),
			DeviceID:     deviceID,
			SysName:      sysName,
			DevicePort:   strings.TrimSpace(remote.devicePort),
			Address:      address,
			RawAddress:   remote.rawAddress,
		})
	}
}
