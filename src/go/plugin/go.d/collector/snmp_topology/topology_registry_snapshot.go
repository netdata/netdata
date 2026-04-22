// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"
	"time"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"
)

type topologyObservationSnapshot struct {
	l2Observations []topologyengine.L2Observation
	localDevice    topologyDevice
	localDeviceID  string
	agentID        string
	collectedAt    time.Time
}

type topologyObservationAggregate struct {
	snapshots      []topologyObservationSnapshot
	l2Observations []topologyengine.L2Observation
	localDeviceID  string
	agentID        string
	collectedAt    time.Time
}

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
	if normalizeMAC(local.ChassisID) == "" {
		if mac := normalizeMAC(localObservation.BaseBridgeAddress); mac != "" {
			local.ChassisID = mac
			local.ChassisIDType = "macAddress"
		}
	}

	return topologyObservationSnapshot{
		l2Observations: []topologyengine.L2Observation{localObservation},
		localDevice:    local,
		localDeviceID:  localObservation.DeviceID,
		agentID:        strings.TrimSpace(c.agentID),
		collectedAt:    c.lastUpdate,
	}, true
}
