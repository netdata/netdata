// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
)

func (c *topologyCache) buildEngineObservation(local topologymodel.Device) topologyengine.L2Observation {
	localManagementIP := topologyutil.NormalizeIPAddress(local.ManagementIP)
	if localManagementIP == "" {
		localManagementIP = pickManagementIP(local.ManagementAddresses)
	}

	baseBridgeAddress := c.resolveLocalBaseBridgeAddress(localManagementIP)
	if baseBridgeAddress != "" && topologyutil.NormalizeMAC(local.ChassisID) == "" {
		local.ChassisID = baseBridgeAddress
		local.ChassisIDType = "macAddress"
	}

	observation := topologyengine.L2Observation{
		DeviceID:          ensureTopologyObservationDeviceID(local, baseBridgeAddress),
		Hostname:          strings.TrimSpace(local.SysName),
		ManagementIP:      localManagementIP,
		SysObjectID:       strings.TrimSpace(local.SysObjectID),
		ChassisID:         strings.TrimSpace(local.ChassisID),
		BaseBridgeAddress: baseBridgeAddress,
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
