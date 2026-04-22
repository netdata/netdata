// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"
)

func (b *topologyRemoteObservationBuilder) updateRemoteIdentity(deviceID, managementIP, chassisID string) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return
	}
	if managementIP = canonicalObservationIP(managementIP); managementIP != "" {
		if _, ok := b.remoteManagementByID[deviceID]; !ok {
			b.remoteManagementByID[deviceID] = managementIP
		}
	}
	if chassisID = strings.TrimSpace(chassisID); chassisID != "" {
		if _, ok := b.remoteChassisByID[deviceID]; !ok {
			b.remoteChassisByID[deviceID] = chassisID
		}
	}
}

func (b *topologyRemoteObservationBuilder) ensureRemoteObservation(protocol, deviceID, hostname, managementIP, chassisID string) *topologyengine.L2Observation {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil
	}

	key := protocol + "|" + deviceID
	entry := b.remoteObservations[key]
	if entry == nil {
		entry = &topologyengine.L2Observation{
			DeviceID: deviceID,
			Inferred: true,
		}
		b.remoteObservations[key] = entry
		b.remoteOrder = append(b.remoteOrder, key)
	}

	entry.Hostname = selectTopologyRemoteHostname(entry.Hostname, hostname, deviceID)
	b.updateRemoteIdentity(deviceID, managementIP, chassisID)
	if entry.ManagementIP == "" {
		entry.ManagementIP = b.remoteManagementByID[deviceID]
	}
	if entry.ChassisID == "" {
		entry.ChassisID = b.remoteChassisByID[deviceID]
	}
	b.resolver.register(deviceID, []string{entry.Hostname}, entry.ChassisID, entry.ManagementIP)
	return entry
}
