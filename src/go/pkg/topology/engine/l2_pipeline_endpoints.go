// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"strconv"
	"strings"
)

func (s *l2BuildState) applyBridge(observations []L2Observation) {
	for _, obs := range observations {
		sourceID := strings.TrimSpace(obs.DeviceID)
		if sourceID == "" {
			continue
		}

		bridgePortToIfIndex := make(map[string]int, len(obs.BridgePorts))
		for _, bridgePort := range sortedBridgePorts(obs.BridgePorts) {
			basePort := strings.TrimSpace(bridgePort.BasePort)
			if basePort == "" || bridgePort.IfIndex <= 0 {
				continue
			}
			bridgePortToIfIndex[basePort] = bridgePort.IfIndex
		}

		for _, candidate := range buildFDBCandidates(obs.FDBEntries, bridgePortToIfIndex) {
			endpointID := "mac:" + candidate.mac
			attachment := Attachment{
				DeviceID:   sourceID,
				IfIndex:    candidate.ifIndex,
				EndpointID: endpointID,
				Method:     "fdb",
			}

			labels := make(map[string]string)
			if candidate.bridgePort != "" {
				labels["bridge_port"] = candidate.bridgePort
			}
			if status := strings.TrimSpace(candidate.statusRaw); status != "" {
				labels["fdb_status"] = status
			}
			if candidate.ifIndex > 0 {
				ifName := strings.TrimSpace(s.ifNameByDeviceIfIndex[deviceIfIndexKey(sourceID, candidate.ifIndex)])
				if ifName != "" {
					labels["if_name"] = ifName
				}
				labels["bridge_domain"] = deriveBridgeDomainFromIfIndex(sourceID, candidate.ifIndex)
			} else if candidate.bridgePort != "" {
				labels["bridge_domain"] = deriveBridgeDomainFromBridgePort(sourceID, candidate.bridgePort)
			}
			if candidate.vlanID != "" {
				labels["vlan_id"] = candidate.vlanID
				labels["vlan"] = candidate.vlanID
			}
			if candidate.vlanName != "" {
				labels["vlan_name"] = candidate.vlanName
			}
			if len(labels) > 0 {
				attachment.Labels = labels
			}

			if addAttachment(s.attachments, attachment) {
				s.attachmentsFDB++
				s.endpointIDs[endpointID] = struct{}{}
				if domain := attachmentDomain(attachment); domain != "" {
					s.bridgeDomains[domain] = struct{}{}
				}
			}
		}
	}
}

func (s *l2BuildState) applyARP(observations []L2Observation) {
	for _, obs := range observations {
		sourceID := strings.TrimSpace(obs.DeviceID)
		if sourceID == "" {
			continue
		}
		for _, entry := range sortedARPNDEntries(obs.ARPNDEntries) {
			mac := normalizeMAC(entry.MAC)
			ip := canonicalIP(entry.IP)
			if mac == "" {
				continue
			}

			endpointID := "mac:" + mac
			acc := ensureEnrichmentAccumulator(s.enrichments, endpointID)
			acc.EndpointID = endpointID
			acc.MAC = mac
			if ip != "" {
				addr := parseAddr(ip)
				if addr.IsValid() {
					acc.IPs[addr.String()] = addr
				}
			}

			protocol := canonicalARPProtocol(entry.Protocol)
			acc.Protocols[protocol] = struct{}{}
			acc.DeviceIDs[sourceID] = struct{}{}
			if entry.IfIndex > 0 {
				acc.IfIndexes[strconv.Itoa(entry.IfIndex)] = struct{}{}
			}
			ifName := strings.TrimSpace(entry.IfName)
			if ifName == "" && entry.IfIndex > 0 {
				ifName = strings.TrimSpace(s.ifNameByDeviceIfIndex[deviceIfIndexKey(sourceID, entry.IfIndex)])
			}
			if ifName != "" {
				acc.IfNames[ifName] = struct{}{}
			}
			if state := strings.TrimSpace(entry.State); state != "" {
				acc.States[state] = struct{}{}
			}
			if addrType := canonicalAddrType(entry.AddrType, ip); addrType != "" {
				acc.AddrTypes[addrType] = struct{}{}
			}
		}
	}
	s.refreshEndpointIndex()
	s.enrichmentsARPND = len(s.enrichments)
}
