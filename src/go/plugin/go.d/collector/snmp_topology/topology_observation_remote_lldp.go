// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"
)

func (b *topologyRemoteObservationBuilder) collectLLDPRemoteObservations() {
	keys := make([]string, 0, len(b.cache.lldpRemotes))
	for key := range b.cache.lldpRemotes {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		remote := b.cache.lldpRemotes[key]
		if remote == nil {
			continue
		}

		remoteSysName := strings.TrimSpace(remote.sysName)
		remoteChassisID := strings.TrimSpace(remote.chassisID)
		remoteManagementIP := normalizeIPAddress(remote.managementAddr)
		if remoteManagementIP == "" {
			remoteManagementIP = pickManagementIP(remote.managementAddrs)
		}

		remoteDeviceID := b.resolver.resolve(
			[]string{remoteSysName},
			remoteChassisID,
			strings.TrimSpace(remote.chassisIDSubtype),
			remoteManagementIP,
		)
		if remoteDeviceID == "" || remoteDeviceID == b.localObservation.DeviceID {
			continue
		}
		b.updateRemoteIdentity(remoteDeviceID, remoteManagementIP, remoteChassisID)

		remoteObservation := b.ensureRemoteObservation(
			"lldp",
			remoteDeviceID,
			firstNonEmpty(remoteSysName, remoteDeviceID),
			remoteManagementIP,
			remoteChassisID,
		)
		if remoteObservation == nil {
			continue
		}

		localPort := b.cache.lldpLocPorts[remote.localPortNum]
		localPortID := ""
		localPortIDSubtype := ""
		localPortDesc := ""
		if localPort != nil {
			localPortID = strings.TrimSpace(localPort.portID)
			localPortIDSubtype = strings.TrimSpace(localPort.portIDSubtype)
			localPortDesc = strings.TrimSpace(localPort.portDesc)
		}

		if strings.TrimSpace(remote.portID) == "" &&
			strings.TrimSpace(remote.portDesc) == "" &&
			localPortID == "" &&
			localPortDesc == "" {
			continue
		}

		remoteObservation.LLDPRemotes = append(remoteObservation.LLDPRemotes, topologyengine.LLDPRemoteObservation{
			LocalPortNum:       strings.TrimSpace(remote.remIndex),
			RemoteIndex:        strings.TrimSpace(remote.localPortNum),
			LocalPortID:        strings.TrimSpace(remote.portID),
			LocalPortIDSubtype: strings.TrimSpace(remote.portIDSubtype),
			LocalPortDesc:      strings.TrimSpace(remote.portDesc),
			ChassisID:          strings.TrimSpace(b.local.ChassisID),
			SysName:            b.localSysName,
			PortID:             localPortID,
			PortIDSubtype:      localPortIDSubtype,
			PortDesc:           localPortDesc,
			ManagementIP:       b.localManagementIP,
		})
	}
}
