// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
	"strings"
	"time"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
)

func (c *topologyCache) snapshotEngineObservations() (topologyObservationSnapshot, bool) {
	if c == nil {
		return topologyObservationSnapshot{}, false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.hasFreshSnapshotAt(time.Now()) {
		return topologyObservationSnapshot{}, false
	}

	local := normalizeTopologyDevice(c.localDevice)
	localObservation := c.buildEngineObservation(local)
	localObservation.DeviceID = strings.TrimSpace(localObservation.DeviceID)
	if localObservation.DeviceID == "" {
		return topologyObservationSnapshot{}, false
	}
	if topologyutil.NormalizeMAC(local.ChassisID) == "" {
		if mac := topologyutil.NormalizeMAC(localObservation.BaseBridgeAddress); mac != "" {
			local.ChassisID = mac
			local.ChassisIDType = "macAddress"
		}
	}

	return topologyObservationSnapshot{
		L2Observations: []topologyengine.L2Observation{localObservation},
		L3Interfaces:   c.snapshotL3Interfaces(localObservation.DeviceID),
		OSPFNeighbors:  c.snapshotOSPFNeighbors(localObservation.DeviceID),
		BGPPeers:       c.snapshotBGPPeers(localObservation.DeviceID),
		LocalDevice:    local,
		LocalDeviceID:  localObservation.DeviceID,
		AgentID:        strings.TrimSpace(c.agentID),
		CollectedAt:    c.lastUpdate,
	}, true
}
