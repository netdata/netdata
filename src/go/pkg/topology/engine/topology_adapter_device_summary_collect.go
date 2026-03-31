// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"strconv"
	"strings"
)

func (b *deviceInterfaceSummaryBuilder) collectInterfaces() {
	for _, iface := range b.interfaces {
		deviceID := strings.TrimSpace(iface.DeviceID)
		if deviceID == "" || iface.IfIndex <= 0 {
			continue
		}
		col := b.collectors[deviceID]
		if col == nil {
			col = &deviceInterfaceCollector{
				ifIndexes:    make(map[string]struct{}),
				ifNames:      make(map[string]struct{}),
				ifTypes:      make(map[string]int),
				adminCounts:  make(map[string]int),
				operCounts:   make(map[string]int),
				portEvidence: make(map[int]*topologyDevicePortEvidence),
			}
			b.collectors[deviceID] = col
		}

		ifIndex := strconv.Itoa(iface.IfIndex)
		col.ifIndexes[ifIndex] = struct{}{}
		if ifName := strings.TrimSpace(iface.IfName); ifName != "" {
			col.ifNames[ifName] = struct{}{}
		}

		admin := strings.TrimSpace(iface.Labels["admin_status"])
		oper := strings.TrimSpace(iface.Labels["oper_status"])
		ifType := strings.TrimSpace(iface.Labels["if_type"])
		ifAlias := strings.TrimSpace(iface.Labels["if_alias"])
		ifDescr := strings.TrimSpace(iface.IfDescr)
		if ifDescr == "" {
			ifDescr = strings.TrimSpace(iface.IfName)
		}
		speedBps := parseTopologyLabelInt64(iface.Labels["speed_bps"])
		lastChange := parseTopologyLabelInt64(iface.Labels["last_change"])
		duplex := normalizeTopologyDuplex(iface.Labels["duplex"])
		mac := normalizeMAC(iface.MAC)
		if mac == "" {
			mac = normalizeMAC(iface.Labels["mac"])
		}
		if ifType != "" {
			col.ifTypes[ifType]++
		}
		if admin != "" {
			col.adminCounts[admin]++
		}
		if oper != "" {
			col.operCounts[oper]++
		}

		col.portStatuses = append(col.portStatuses, topologyDevicePortStatus{
			IfIndex:        iface.IfIndex,
			IfName:         strings.TrimSpace(iface.IfName),
			IfDescr:        ifDescr,
			IfAlias:        ifAlias,
			MAC:            mac,
			SpeedBps:       speedBps,
			LastChange:     lastChange,
			Duplex:         duplex,
			InterfaceType:  ifType,
			AdminStatus:    admin,
			OperStatus:     oper,
			LinkMode:       "unknown",
			ModeConfidence: "low",
			TopologyRole:   "unknown",
			RoleConfidence: "low",
		})

		if isTopologyLAGInterfaceType(ifType) {
			evidence := ensureTopologyPortEvidence(col.portEvidence, iface.IfIndex)
			evidence.isLAG = true
		}
	}
}

func (b *deviceInterfaceSummaryBuilder) collectFDBAttachments() {
	for _, attachment := range b.attachments {
		deviceID := strings.TrimSpace(attachment.DeviceID)
		if deviceID == "" || attachment.IfIndex <= 0 {
			continue
		}
		col := b.collectors[deviceID]
		if col == nil {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(attachment.Method), "fdb") {
			continue
		}
		fdbStatus := strings.ToLower(strings.TrimSpace(attachment.Labels["fdb_status"]))
		if fdbStatus == "ignored" {
			continue
		}

		evidence := ensureTopologyPortEvidence(col.portEvidence, attachment.IfIndex)
		evidence.hasFDB = true
		endpointID := normalizeFDBEndpointID(attachment.EndpointID)
		if endpointID == "" {
			endpointID = strings.TrimSpace(attachment.EndpointID)
		}
		if endpointID != "" {
			evidence.fdbEndpointIDs[endpointID] = struct{}{}
			if aliasOwners, ok := b.managedAliasOwners[endpointID]; ok {
				for aliasOwnerID := range aliasOwners {
					if !strings.EqualFold(strings.TrimSpace(aliasOwnerID), deviceID) {
						evidence.hasFDBManagedAlias = true
						break
					}
				}
			}
		}
		vlanID := normalizeTopologyVLANID(firstNonEmpty(attachment.Labels["vlan_id"], attachment.Labels["vlan"]))
		if vlanID != "" {
			evidence.vlanIDs[vlanID] = struct{}{}
			if vlanName := strings.TrimSpace(attachment.Labels["vlan_name"]); vlanName != "" {
				if _, exists := evidence.vlanNames[vlanID]; !exists {
					evidence.vlanNames[vlanID] = vlanName
				}
			}
		}
	}
}

func (b *deviceInterfaceSummaryBuilder) collectAdjacencyEvidence() {
	for _, adj := range b.adjacencies {
		deviceID := strings.TrimSpace(adj.SourceID)
		if deviceID == "" {
			continue
		}
		col := b.collectors[deviceID]
		if col == nil {
			continue
		}

		ifIndex := resolveAdjacencySourceIfIndex(adj, b.ifIndexByDeviceName)
		if ifIndex <= 0 {
			continue
		}
		evidence := ensureTopologyPortEvidence(col.portEvidence, ifIndex)
		protocol := strings.ToLower(strings.TrimSpace(adj.Protocol))
		switch protocol {
		case "stp":
			evidence.hasSTP = true
			vlanID := normalizeTopologyVLANID(firstNonEmpty(adj.Labels["vlan_id"], adj.Labels["vlan"]))
			if vlanID != "" {
				evidence.vlanIDs[vlanID] = struct{}{}
				if vlanName := strings.TrimSpace(adj.Labels["vlan_name"]); vlanName != "" {
					if _, exists := evidence.vlanNames[vlanID]; !exists {
						evidence.vlanNames[vlanID] = vlanName
					}
				}
			}
			if state := normalizeTopologySTPState(adj.Labels["stp_state"]); state != "" {
				evidence.stpStates[state] = struct{}{}
			}
		case "lldp", "cdp":
			evidence.hasPeer = true
			neighbor := buildTopologyPortNeighborStatus(protocol, adj, b.deviceByID)
			if key := topologyPortNeighborStatusKey(neighbor); key != "" {
				evidence.neighbors[key] = neighbor
			}
		}
	}
}

func (b *deviceInterfaceSummaryBuilder) collectBridgeLinkEvidence() {
	for _, link := range b.bridgeLinks {
		for _, port := range []bridgePortRef{link.designatedPort, link.port} {
			deviceID := strings.TrimSpace(port.deviceID)
			if deviceID == "" || port.ifIndex <= 0 {
				continue
			}
			col := b.collectors[deviceID]
			if col == nil {
				continue
			}
			evidence := ensureTopologyPortEvidence(col.portEvidence, port.ifIndex)
			evidence.hasBridgeLink = true
		}
	}
}
