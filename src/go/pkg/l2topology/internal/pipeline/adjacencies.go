// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
)

func (s *l2BuildState) applyLLDP(observations []model.L2Observation) {
	lldpLinks := buildLLDPMatchLinks(observations)
	annotateLLDPLinkMatchIdentities(lldpLinks, s.hostToID, s.chassisToID, s.directIPToID)
	lldpPairs := matchLLDPLinksEnlinkdPassOrder(lldpLinks)
	lldpTargetOverrides := buildLLDPTargetOverrides(lldpLinks, lldpPairs)
	lldpPairMetadata := buildLLDPPairMetadata(lldpLinks, lldpPairs)
	lldpPairedTargetPortEvidence := buildLLDPPairedTargetPortEvidence(lldpLinks, lldpPairs)

	for _, link := range lldpLinks {
		managementIP := canonicalUsableIPAddress(link.remoteManagement)
		managementIPValue := ""
		if managementIP.IsValid() {
			managementIPValue = managementIP.String()
		}
		targetID := strings.TrimSpace(lldpTargetOverrides[link.index])
		if targetID == "" {
			targetID = s.resolveRemote(link.remoteSysName, link.remoteChassisID, managementIPValue, link.remoteFallbackID)
		}
		s.recordRemoteManagementAddress(targetID, managementIPValue)

		adj := model.Adjacency{
			Protocol:           "lldp",
			SourceID:           link.sourceDeviceID,
			SourcePort:         link.sourcePort,
			SourcePortEvidence: lldpLocalPortEvidence(link),
			TargetID:           targetID,
			TargetPort:         link.targetPort,
			TargetPortEvidence: mergeAdjacencyPortEvidence(
				lldpPairedTargetPortEvidence[link.index],
				lldpRemotePortEvidence(link),
			),
		}
		applyAdjacencyPairMetadata(&adj, lldpPairMetadata[link.index])
		if addAdjacency(s.adjacencies, adj) {
			s.linksLLDP++
		}
	}
}

func (s *l2BuildState) applyCDP(observations []model.L2Observation) {
	cdpLinks := buildCDPMatchLinks(observations)
	cdpPairs := matchCDPLinksEnlinkdPassOrder(cdpLinks)
	cdpTargetOverrides := buildCDPTargetOverrides(cdpLinks, cdpPairs)
	cdpPairMetadata := buildCDPPairMetadata(cdpLinks, cdpPairs)
	cdpPairedTargetPortEvidence := buildCDPPairedTargetPortEvidence(cdpLinks, cdpPairs)

	for _, link := range cdpLinks {
		managementAddr := canonicalUsableIPAddress(link.remoteManagementIP)
		managementIP := ""
		if managementAddr.IsValid() {
			managementIP = managementAddr.String()
		}
		rawAddress := link.remoteAddressRaw
		targetID := strings.TrimSpace(cdpTargetOverrides[link.index])
		if targetID == "" {
			targetID = s.resolveRemoteEnforcingHostnameMACGuard(link.remoteHost, link.remoteDeviceID, managementIP, link.remoteDeviceID)
		}
		s.recordRemoteManagementAddress(targetID, managementIP)

		adj := model.Adjacency{
			Protocol:           "cdp",
			SourceID:           link.sourceDeviceID,
			SourcePort:         link.localInterfaceName,
			SourcePortEvidence: cdpLocalPortEvidence(link),
			TargetID:           targetID,
			TargetPort:         link.remoteDevicePort,
			TargetPortEvidence: cdpPairedTargetPortEvidence[link.index],
		}
		if managementIP != "" || strings.TrimSpace(rawAddress) != "" {
			adj.Labels = make(map[string]string, 2)
			if managementIP != "" {
				adj.Labels[adjacencyLabelRemoteManagementIP] = managementIP
			}
			if strings.TrimSpace(rawAddress) != "" {
				adj.Labels[adjacencyLabelRemoteAddressRaw] = rawAddress
			}
		}
		applyAdjacencyPairMetadata(&adj, cdpPairMetadata[link.index])
		if addAdjacency(s.adjacencies, adj) {
			s.linksCDP++
		}
	}
}

func (s *l2BuildState) applySTP(observations []model.L2Observation) {
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

			sourcePort := strings.TrimSpace(entry.Port)
			ifIndex := entry.IfIndex
			if ifIndex <= 0 {
				ifIndex = bridgePortToIfIndex[sourcePort]
			}
			sourceIfName := strings.TrimSpace(entry.IfName)
			if sourceIfName == "" && ifIndex > 0 {
				sourceIfName = strings.TrimSpace(s.ifNameByDeviceIfIndex[deviceIfIndexKey(sourceID, ifIndex)])
			}
			targetPort := strings.TrimSpace(entry.DesignatedPort)

			adj := model.Adjacency{
				Protocol:   "stp",
				SourceID:   sourceID,
				SourcePort: sourcePort,
				SourcePortEvidence: model.AdjacencyPortEvidence{
					IfIndex:    ifIndex,
					IfName:     sourceIfName,
					BridgePort: sourcePort,
				},
				TargetID:   targetID,
				TargetPort: targetPort,
				TargetPortEvidence: model.AdjacencyPortEvidence{
					BridgePort: stpBridgePortFromPortID(targetPort),
				},
			}
			labels := make(map[string]string)
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
