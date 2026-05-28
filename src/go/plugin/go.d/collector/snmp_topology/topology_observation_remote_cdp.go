// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"
)

func (b *topologyRemoteObservationBuilder) collectCDPRemoteObservations() {
	keys := make([]string, 0, len(b.cache.cdpRemotes))
	for key := range b.cache.cdpRemotes {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		remote := b.cache.cdpRemotes[key]
		if remote == nil {
			continue
		}

		remoteDeviceToken := strings.TrimSpace(remote.deviceID)
		remoteSysName := strings.TrimSpace(remote.sysName)
		remoteManagementIP := normalizeIPAddress(remote.address)
		if remoteManagementIP == "" {
			remoteManagementIP = pickManagementIP(remote.managementAddrs)
		}
		if remoteManagementIP == "" && remoteDeviceToken == "" && remoteSysName == "" {
			continue
		}

		remoteDeviceID := b.resolver.resolve(
			[]string{remoteDeviceToken, remoteSysName},
			"",
			"",
			remoteManagementIP,
		)
		if remoteDeviceID == "" || remoteDeviceID == b.localObservation.DeviceID {
			continue
		}
		b.updateRemoteIdentity(remoteDeviceID, remoteManagementIP, "")

		remoteIfName := strings.TrimSpace(remote.devicePort)
		localIfName := strings.TrimSpace(remote.ifName)
		if localIfName == "" && strings.TrimSpace(remote.ifIndex) != "" {
			localIfName = strings.TrimSpace(b.cache.ifNamesByIndex[remote.ifIndex])
		}
		if remoteIfName == "" || localIfName == "" {
			continue
		}

		remoteObservation := b.ensureRemoteObservation(
			"cdp",
			remoteDeviceID,
			firstNonEmpty(remoteSysName, remoteDeviceToken, remoteDeviceID),
			remoteManagementIP,
			"",
		)
		if remoteObservation == nil {
			continue
		}

		remoteObservation.CDPRemotes = append(remoteObservation.CDPRemotes, topologyengine.CDPRemoteObservation{
			LocalIfName: remoteIfName,
			DeviceID:    b.localGlobalID,
			SysName:     b.localSysName,
			DevicePort:  localIfName,
			Address:     b.localManagementIP,
		})
	}
}
