// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

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
