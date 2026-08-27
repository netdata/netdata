// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
)

func (c *topologyBuilder) buildEngineObservation(local topologymodel.Device) topologyengine.L2Observation {
	if !c.chargeWork(uint64(len(local.Labels))) {
		return topologyengine.L2Observation{}
	}
	localManagementIP := normalizeEligibleManagementIP(local.ManagementIP)

	baseBridgeAddress := c.resolveLocalBaseBridgeAddress(localManagementIP)
	if baseBridgeAddress != "" && topologyutil.NormalizeMAC(local.ChassisID) == "" {
		local.ChassisID = baseBridgeAddress
		local.ChassisIDType = "macAddress"
	}

	observation := topologyengine.L2Observation{
		DeviceID:          ensureTopologyObservationDeviceID(local, baseBridgeAddress),
		Hostname:          strings.TrimSpace(local.SysName),
		ManagementIP:      localManagementIP,
		ManagementAliases: c.engineManagementAliases(local.ManagementAddresses),
		SysObjectID:       strings.TrimSpace(local.SysObjectID),
		ChassisID:         strings.TrimSpace(local.ChassisID),
		BaseBridgeAddress: baseBridgeAddress,
		Labels:            cloneTopologyLabels(local.Labels),
	}
	if observation.BaseBridgeAddress == "" {
		observation.BaseBridgeAddress = stpBridgeAddressToMAC(observation.ChassisID)
	}

	c.appendObservedInterfaces(&observation)
	c.appendObservedBridgePorts(&observation)
	c.appendObservedFDBEntries(&observation)
	c.appendObservedSTPPorts(&observation)
	c.appendObservedARPNDEntries(&observation)
	c.appendObservedLLDPRemotes(&observation)
	c.appendObservedCDPRemotes(&observation)

	return observation
}

func engineManagementAliases(addresses []topologymodel.ManagementAddress) []string {
	return (&topologyBuilder{}).engineManagementAliases(addresses)
}

func (c *topologyBuilder) engineManagementAliases(addresses []topologymodel.ManagementAddress) []string {
	if !c.chargeWork(uint64(len(addresses))) {
		return nil
	}
	aliases := make([]string, 0, len(addresses))
	for _, address := range addresses {
		addr, ok := topologymodel.ParseManagementAddressIP(address)
		if !ok {
			continue
		}
		if ip := normalizeEligibleManagementIP(addr.String()); ip != "" {
			aliases = append(aliases, ip)
		}
	}
	aliases, err := topologyutil.DeduplicateSortedStringsWithLimiter(c.workLimiter, aliases)
	if err != nil {
		c.workErr = err
		return nil
	}
	return aliases
}
