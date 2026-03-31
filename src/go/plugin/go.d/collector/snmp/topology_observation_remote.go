// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"sort"
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"
)

type topologyRemoteObservationBuilder struct {
	cache             *topologyCache
	local             topologyDevice
	localObservation  topologyengine.L2Observation
	localManagementIP string
	localSysName      string
	localGlobalID     string

	resolver             *topologyObservationIdentityResolver
	remoteObservations   map[string]*topologyengine.L2Observation
	remoteOrder          []string
	remoteManagementByID map[string]string
	remoteChassisByID    map[string]string
}

func newTopologyRemoteObservationBuilder(cache *topologyCache, local topologyDevice, localObservation topologyengine.L2Observation) *topologyRemoteObservationBuilder {
	localManagementIP := normalizeIPAddress(local.ManagementIP)
	if localManagementIP == "" {
		localManagementIP = pickManagementIP(local.ManagementAddresses)
	}

	localGlobalID := strings.TrimSpace(localObservation.Hostname)
	if localGlobalID == "" {
		localGlobalID = localObservation.DeviceID
	}

	return &topologyRemoteObservationBuilder{
		cache:                cache,
		local:                local,
		localObservation:     localObservation,
		localManagementIP:    localManagementIP,
		localSysName:         strings.TrimSpace(local.SysName),
		localGlobalID:        localGlobalID,
		resolver:             newTopologyObservationIdentityResolver(localObservation),
		remoteObservations:   make(map[string]*topologyengine.L2Observation),
		remoteOrder:          make([]string, 0, len(cache.lldpRemotes)+len(cache.cdpRemotes)),
		remoteManagementByID: make(map[string]string),
		remoteChassisByID:    make(map[string]string),
	}
}

func (c *topologyCache) buildEngineObservations(local topologyDevice) ([]topologyengine.L2Observation, string) {
	localObservation := c.buildEngineObservation(local)
	localObservation.DeviceID = strings.TrimSpace(localObservation.DeviceID)
	if localObservation.DeviceID == "" {
		return nil, ""
	}

	builder := newTopologyRemoteObservationBuilder(c, local, localObservation)
	builder.collectLLDPRemoteObservations()
	builder.collectCDPRemoteObservations()

	return builder.observations(), localObservation.DeviceID
}

func (b *topologyRemoteObservationBuilder) observations() []topologyengine.L2Observation {
	observations := make([]topologyengine.L2Observation, 0, 1+len(b.remoteObservations))
	observations = append(observations, b.localObservation)

	sort.Strings(b.remoteOrder)
	for _, key := range b.remoteOrder {
		entry := b.remoteObservations[key]
		if entry == nil {
			continue
		}
		if entry.ManagementIP == "" {
			entry.ManagementIP = b.remoteManagementByID[entry.DeviceID]
		}
		if entry.ChassisID == "" {
			entry.ChassisID = b.remoteChassisByID[entry.DeviceID]
		}
		if len(entry.LLDPRemotes) == 0 && len(entry.CDPRemotes) == 0 {
			continue
		}
		if entry.Hostname == "" {
			entry.Hostname = entry.DeviceID
		}
		observations = append(observations, *entry)
	}

	return observations
}

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
			firstNonEmpty(remoteDeviceToken, remoteSysName, remoteDeviceID),
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

func selectTopologyRemoteHostname(current, candidate, deviceID string) string {
	current = strings.TrimSpace(current)
	candidate = strings.TrimSpace(candidate)
	deviceID = strings.TrimSpace(deviceID)
	if candidate == "" {
		if current != "" {
			return current
		}
		return deviceID
	}
	if current == "" || current == deviceID {
		return candidate
	}
	return current
}
