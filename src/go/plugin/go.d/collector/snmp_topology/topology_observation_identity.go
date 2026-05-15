// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"
)

type topologyObservationIdentityResolver struct {
	hostToID    map[string]string
	chassisToID map[string]string
	macToID     map[string]string
	ipToID      map[string]string
	fallbackSeq int
}

func newTopologyObservationIdentityResolver(local topologyengine.L2Observation) *topologyObservationIdentityResolver {
	resolver := &topologyObservationIdentityResolver{
		hostToID:    make(map[string]string),
		chassisToID: make(map[string]string),
		macToID:     make(map[string]string),
		ipToID:      make(map[string]string),
	}
	resolver.register(local.DeviceID, []string{local.Hostname}, local.ChassisID, local.ManagementIP)
	return resolver
}

func (r *topologyObservationIdentityResolver) resolve(hostAliases []string, chassisID, chassisType, managementIP string) string {
	if mac := canonicalObservationMAC(chassisID); mac != "" {
		if id := r.macToID[mac]; id != "" {
			r.register(id, hostAliases, chassisID, managementIP)
			return id
		}

		candidate := normalizeTopologyDevice(topologyDevice{
			ChassisID:     mac,
			ChassisIDType: "macAddress",
			SysName:       firstNonEmpty(hostAliases...),
			ManagementIP:  normalizeIPAddress(managementIP),
		})
		id := strings.TrimSpace(ensureTopologyObservationDeviceID(candidate, ""))
		if id == "" || id == "local-device" {
			r.fallbackSeq++
			id = fmt.Sprintf("remote-device-%d", r.fallbackSeq)
		}
		r.register(id, hostAliases, mac, managementIP)
		return id
	}

	for _, host := range hostAliases {
		if id := r.hostToID[canonicalObservationHost(host)]; id != "" {
			r.register(id, hostAliases, chassisID, managementIP)
			return id
		}
	}
	if id := r.chassisToID[canonicalObservationChassis(chassisID)]; id != "" {
		r.register(id, hostAliases, chassisID, managementIP)
		return id
	}
	if id := r.ipToID[canonicalObservationIP(managementIP)]; id != "" {
		r.register(id, hostAliases, chassisID, managementIP)
		return id
	}

	candidate := normalizeTopologyDevice(topologyDevice{
		ChassisID:     strings.TrimSpace(chassisID),
		ChassisIDType: strings.TrimSpace(chassisType),
		SysName:       firstNonEmpty(hostAliases...),
		ManagementIP:  normalizeIPAddress(managementIP),
	})
	id := strings.TrimSpace(ensureTopologyObservationDeviceID(candidate, ""))
	if id == "" || id == "local-device" {
		r.fallbackSeq++
		id = fmt.Sprintf("remote-device-%d", r.fallbackSeq)
	}
	r.register(id, hostAliases, chassisID, managementIP)
	return id
}

func (r *topologyObservationIdentityResolver) register(id string, hostAliases []string, chassisID, managementIP string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	for _, host := range hostAliases {
		if key := canonicalObservationHost(host); key != "" {
			if _, exists := r.hostToID[key]; !exists {
				r.hostToID[key] = id
			}
		}
	}
	if key := canonicalObservationChassis(chassisID); key != "" {
		if _, exists := r.chassisToID[key]; !exists {
			r.chassisToID[key] = id
		}
	}
	if mac := canonicalObservationMAC(chassisID); mac != "" {
		if _, exists := r.macToID[mac]; !exists {
			r.macToID[mac] = id
		}
	}
	if key := canonicalObservationIP(managementIP); key != "" {
		if _, exists := r.ipToID[key]; !exists {
			r.ipToID[key] = id
		}
	}
}

func canonicalObservationHost(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func canonicalObservationChassis(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if mac := normalizeMAC(value); mac != "" {
		return mac
	}
	return strings.ToLower(value)
}

func canonicalObservationMAC(value string) string {
	if mac := normalizeMAC(value); mac != "" {
		return mac
	}
	return ""
}

func canonicalObservationIP(value string) string {
	value = normalizeIPAddress(value)
	if value != "" {
		return strings.ToLower(value)
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
