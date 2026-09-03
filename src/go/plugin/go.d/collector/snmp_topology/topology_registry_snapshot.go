// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
)

func (c *topologyBuilder) buildObservationSnapshot() (topologymodel.ObservationSnapshot, bool) {
	if c == nil {
		return topologymodel.ObservationSnapshot{}, false
	}
	local := normalizeTopologyDevice(c.localDevice)
	localObservation := c.buildEngineObservation(local)
	localObservation.DeviceID = strings.TrimSpace(localObservation.DeviceID)
	if localObservation.DeviceID == "" {
		return topologymodel.ObservationSnapshot{}, false
	}
	if topologyutil.NormalizeMAC(local.ChassisID) == "" {
		if mac := topologyutil.NormalizeMAC(localObservation.BaseBridgeAddress); mac != "" {
			local.ChassisID = mac
			local.ChassisIDType = "macAddress"
		}
	}
	l3Interfaces := c.snapshotL3Interfaces(localObservation.DeviceID)

	return topologymodel.ObservationSnapshot{
		L2Observations: []topologyengine.L2Observation{localObservation},
		L3Interfaces:   l3Interfaces,
		OSPFNeighbors:  c.snapshotOSPFNeighbors(localObservation.DeviceID, l3Interfaces),
		BGPPeers:       c.snapshotBGPPeers(localObservation.DeviceID),
		LocalDevice:    local,
		LocalDeviceID:  localObservation.DeviceID,
		AgentID:        strings.TrimSpace(c.agentID),
		CollectedAt:    c.lastUpdate,
	}, true
}
