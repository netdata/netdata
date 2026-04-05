// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import "strings"

func (s *l2BuildState) applyLLDP(observations []L2Observation) {
	lldpLinks := buildLLDPMatchLinks(observations)
	annotateLLDPLinkMatchIdentities(lldpLinks, s.hostToID, s.chassisToID, s.ipToID)
	lldpPairs := matchLLDPLinksEnlinkdPassOrder(lldpLinks)
	lldpTargetOverrides := buildLLDPTargetOverrides(lldpLinks, lldpPairs)
	lldpPairMetadata := buildLLDPPairMetadata(lldpLinks, lldpPairs)

	for _, link := range lldpLinks {
		targetID := strings.TrimSpace(lldpTargetOverrides[link.index])
		if targetID == "" {
			targetID = s.resolveRemote(link.remoteSysName, link.remoteChassisID, link.remoteManagement, link.remoteFallbackID)
		}

		adj := Adjacency{
			Protocol:   "lldp",
			SourceID:   link.sourceDeviceID,
			SourcePort: link.sourcePort,
			TargetID:   targetID,
			TargetPort: link.targetPort,
		}
		applyAdjacencyPairMetadata(&adj, lldpPairMetadata[link.index])
		if addAdjacency(s.adjacencies, adj) {
			s.linksLLDP++
		}
	}
}

func (s *l2BuildState) applyCDP(observations []L2Observation) {
	cdpLinks := buildCDPMatchLinks(observations)
	cdpPairs := matchCDPLinksEnlinkdPassOrder(cdpLinks)
	cdpTargetOverrides := buildCDPTargetOverrides(cdpLinks, cdpPairs)
	cdpPairMetadata := buildCDPPairMetadata(cdpLinks, cdpPairs)

	for _, link := range cdpLinks {
		rawAddress := strings.TrimSpace(link.remoteAddressRaw)
		targetID := strings.TrimSpace(cdpTargetOverrides[link.index])
		if targetID == "" {
			targetIP := canonicalIP(rawAddress)
			targetID = s.resolveRemoteEnforcingHostnameMACGuard(link.remoteHost, link.remoteDeviceID, targetIP, link.remoteDeviceID)
		}

		adj := Adjacency{
			Protocol:   "cdp",
			SourceID:   link.sourceDeviceID,
			SourcePort: link.localInterfaceName,
			TargetID:   targetID,
			TargetPort: link.remoteDevicePort,
		}
		if rawAddress != "" {
			adj.Labels = map[string]string{
				"remote_address_raw": strings.ToLower(rawAddress),
			}
		}
		applyAdjacencyPairMetadata(&adj, cdpPairMetadata[link.index])
		if addAdjacency(s.adjacencies, adj) {
			s.linksCDP++
		}
	}
}

func (s *l2BuildState) applySTP(observations []L2Observation) {
	for _, obs := range observations {
		sourceID := strings.TrimSpace(obs.DeviceID)
		if sourceID == "" {
			continue
		}

		localBridgeAddr := canonicalBridgeAddr(obs.BaseBridgeAddress, obs.ChassisID)
		bridgePortToIfIndex := make(map[string]int, len(obs.BridgePorts))
		for _, bridgePort := range sortedBridgePorts(obs.BridgePorts) {
			basePort := strings.TrimSpace(bridgePort.BasePort)
			if basePort == "" || bridgePort.IfIndex <= 0 {
				continue
			}
			bridgePortToIfIndex[basePort] = bridgePort.IfIndex
		}

		for _, entry := range sortedSTPPortEntries(obs.STPPorts) {
			remoteBridgeAddr := canonicalBridgeAddr(entry.DesignatedBridge, "")
			if remoteBridgeAddr == "" {
				continue
			}
			if localBridgeAddr != "" && localBridgeAddr == remoteBridgeAddr {
				continue
			}

			targetID := strings.TrimSpace(s.bridgeAddrToID[remoteBridgeAddr])
			if targetID == "" || targetID == sourceID {
				continue
			}

			ifIndex := entry.IfIndex
			if ifIndex <= 0 {
				ifIndex = bridgePortToIfIndex[strings.TrimSpace(entry.Port)]
			}
			sourcePort := strings.TrimSpace(entry.IfName)
			if sourcePort == "" && ifIndex > 0 {
				sourcePort = strings.TrimSpace(s.ifNameByDeviceIfIndex[deviceIfIndexKey(sourceID, ifIndex)])
			}
			if sourcePort == "" {
				sourcePort = strings.TrimSpace(entry.Port)
			}

			adj := Adjacency{
				Protocol:   "stp",
				SourceID:   sourceID,
				SourcePort: sourcePort,
				TargetID:   targetID,
				TargetPort: strings.TrimSpace(entry.DesignatedPort),
			}
			labels := make(map[string]string)
			if v := strings.TrimSpace(entry.Port); v != "" {
				labels["stp_port"] = v
			}
			if v := strings.TrimSpace(entry.State); v != "" {
				labels["stp_state"] = v
			}
			if v := strings.TrimSpace(entry.Enable); v != "" {
				labels["stp_enable"] = v
			}
			if v := strings.TrimSpace(entry.PathCost); v != "" {
				labels["stp_path_cost"] = v
			}
			if v := strings.TrimSpace(entry.DesignatedRoot); v != "" {
				labels["stp_designated_root"] = v
			}
			if v := strings.TrimSpace(entry.VLANID); v != "" {
				labels["vlan_id"] = v
				labels["vlan"] = v
			}
			if v := strings.TrimSpace(entry.VLANName); v != "" {
				labels["vlan_name"] = v
			}
			if len(labels) > 0 {
				adj.Labels = labels
			}
			if addAdjacency(s.adjacencies, adj) {
				s.linksSTP++
			}
		}
	}
}
