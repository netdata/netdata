// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"sort"
	"strconv"
	"strings"
)

type deviceInterfaceCollector struct {
	ifIndexes    map[string]struct{}
	ifNames      map[string]struct{}
	ifTypes      map[string]int
	adminCounts  map[string]int
	operCounts   map[string]int
	portStatuses []topologyDevicePortStatus
	portEvidence map[int]*topologyDevicePortEvidence
}

type deviceInterfaceSummaryBuilder struct {
	interfaces          []Interface
	attachments         []Attachment
	adjacencies         []Adjacency
	deviceByID          map[string]Device
	ifIndexByDeviceName map[string]int
	bridgeLinks         []bridgeBridgeLinkRecord
	reporterAliases     map[string][]string
	collectors          map[string]*deviceInterfaceCollector
	managedAliasOwners  map[string]map[string]struct{}
}

func buildTopologyDeviceInterfaceSummaries(
	interfaces []Interface,
	attachments []Attachment,
	adjacencies []Adjacency,
	deviceByID map[string]Device,
	ifIndexByDeviceName map[string]int,
	bridgeLinks []bridgeBridgeLinkRecord,
	reporterAliases map[string][]string,
) map[string]topologyDeviceInterfaceSummary {
	return newDeviceInterfaceSummaryBuilder(
		interfaces,
		attachments,
		adjacencies,
		deviceByID,
		ifIndexByDeviceName,
		bridgeLinks,
		reporterAliases,
	).build()
}

func newDeviceInterfaceSummaryBuilder(
	interfaces []Interface,
	attachments []Attachment,
	adjacencies []Adjacency,
	deviceByID map[string]Device,
	ifIndexByDeviceName map[string]int,
	bridgeLinks []bridgeBridgeLinkRecord,
	reporterAliases map[string][]string,
) *deviceInterfaceSummaryBuilder {
	return &deviceInterfaceSummaryBuilder{
		interfaces:          interfaces,
		attachments:         attachments,
		adjacencies:         adjacencies,
		deviceByID:          deviceByID,
		ifIndexByDeviceName: ifIndexByDeviceName,
		bridgeLinks:         bridgeLinks,
		reporterAliases:     reporterAliases,
		collectors:          make(map[string]*deviceInterfaceCollector),
	}
}

func (b *deviceInterfaceSummaryBuilder) build() map[string]topologyDeviceInterfaceSummary {
	if len(b.interfaces) == 0 {
		return nil
	}

	b.collectInterfaces()
	if len(b.collectors) == 0 {
		return nil
	}

	b.managedAliasOwners = buildFDBAliasOwnerMap(b.reporterAliases)
	b.collectFDBAttachments()
	b.collectAdjacencyEvidence()
	b.collectBridgeLinkEvidence()

	return b.buildSummaries()
}

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

func (b *deviceInterfaceSummaryBuilder) buildSummaries() map[string]topologyDeviceInterfaceSummary {
	out := make(map[string]topologyDeviceInterfaceSummary, len(b.collectors))
	for deviceID, col := range b.collectors {
		sort.Slice(col.portStatuses, func(i, j int) bool {
			left, right := col.portStatuses[i], col.portStatuses[j]
			if left.IfIndex != right.IfIndex {
				return left.IfIndex < right.IfIndex
			}
			return left.IfName < right.IfName
		})

		modeCounts := make(map[string]int)
		roleCounts := make(map[string]int)
		deviceVLANIDs := make(map[string]struct{})
		portsUp := 0
		portsDown := 0
		portsAdminDown := 0
		totalBandwidthBps := int64(0)
		fdbTotalMACs := 0
		lldpNeighborCount := 0
		cdpNeighborCount := 0
		portStatuses := make([]map[string]any, 0, len(col.portStatuses))
		for _, st := range col.portStatuses {
			evidence := col.portEvidence[st.IfIndex]
			mode, confidence, sources, vlans := classifyTopologyPortLinkMode(evidence)
			role, roleConfidence, roleSources := classifyTopologyPortRole(evidence)
			st.LinkMode = mode
			st.ModeConfidence = confidence
			st.ModeSources = sources
			st.VLANIDs = vlans
			st.TopologyRole = role
			st.RoleConfidence = roleConfidence
			st.RoleSources = roleSources
			if evidence != nil {
				st.FDBMACCount = len(evidence.fdbEndpointIDs)
				st.STPState = summarizeTopologySTPState(evidence.stpStates)
				st.VLANs = topologyPortVLANAttributes(st.VLANIDs, evidence.vlanNames, st.LinkMode)
				st.Neighbors = sortedTopologyPortNeighbors(evidence.neighbors)
			}

			for _, vlanID := range st.VLANIDs {
				deviceVLANIDs[vlanID] = struct{}{}
			}
			if strings.EqualFold(strings.TrimSpace(st.OperStatus), "up") {
				portsUp++
				totalBandwidthBps = safeTopologyInt64Add(totalBandwidthBps, st.SpeedBps)
			} else if strings.EqualFold(strings.TrimSpace(st.OperStatus), "down") || strings.EqualFold(strings.TrimSpace(st.OperStatus), "lowerlayerdown") {
				portsDown++
			}
			if strings.EqualFold(strings.TrimSpace(st.AdminStatus), "down") || strings.EqualFold(strings.TrimSpace(st.AdminStatus), "administrativelydown") {
				portsAdminDown++
			}
			fdbTotalMACs += st.FDBMACCount
			for _, neighbor := range st.Neighbors {
				switch strings.ToLower(strings.TrimSpace(neighbor.Protocol)) {
				case "lldp":
					lldpNeighborCount++
				case "cdp":
					cdpNeighborCount++
				}
			}

			modeCounts[mode]++
			roleCounts[role]++
			portStatuses = append(portStatuses, buildTopologyDevicePortStatusAttributes(st))
		}

		out[deviceID] = topologyDeviceInterfaceSummary{
			portsTotal:        len(col.ifIndexes),
			ifIndexes:         sortedTopologySet(col.ifIndexes),
			ifNames:           sortedTopologySet(col.ifNames),
			adminStatusCount:  intCountMapToAny(col.adminCounts),
			operStatusCount:   intCountMapToAny(col.operCounts),
			linkModeCount:     intCountMapToAny(modeCounts),
			roleCount:         intCountMapToAny(roleCounts),
			portsUp:           portsUp,
			portsDown:         portsDown,
			portsAdminDown:    portsAdminDown,
			totalBandwidthBps: totalBandwidthBps,
			fdbTotalMACs:      fdbTotalMACs,
			vlanCount:         len(deviceVLANIDs),
			lldpNeighborCount: lldpNeighborCount,
			cdpNeighborCount:  cdpNeighborCount,
			portStatuses:      portStatuses,
		}
	}
	return out
}

func buildTopologyDevicePortStatusAttributes(st topologyDevicePortStatus) map[string]any {
	portStatus := map[string]any{
		"if_index":                 st.IfIndex,
		"if_name":                  strings.TrimSpace(st.IfName),
		"if_descr":                 strings.TrimSpace(st.IfDescr),
		"if_alias":                 strings.TrimSpace(st.IfAlias),
		"mac":                      strings.TrimSpace(st.MAC),
		"duplex":                   strings.TrimSpace(st.Duplex),
		"link_mode":                st.LinkMode,
		"link_mode_confidence":     st.ModeConfidence,
		"topology_role":            st.TopologyRole,
		"topology_role_confidence": st.RoleConfidence,
	}
	if st.SpeedBps > 0 {
		portStatus["speed"] = st.SpeedBps
	}
	if st.LastChange > 0 {
		portStatus["last_change"] = st.LastChange
	}
	if len(st.ModeSources) > 0 {
		portStatus["link_mode_sources"] = st.ModeSources
	}
	if len(st.RoleSources) > 0 {
		portStatus["topology_role_sources"] = st.RoleSources
	}
	if len(st.VLANIDs) > 0 {
		portStatus["vlan_ids"] = st.VLANIDs
	}
	if len(st.VLANs) > 0 {
		portStatus["vlans"] = st.VLANs
	}
	if st.FDBMACCount > 0 {
		portStatus["fdb_mac_count"] = st.FDBMACCount
	}
	if st.STPState != "" {
		portStatus["stp_state"] = st.STPState
	}
	if len(st.Neighbors) > 0 {
		neighbors := make([]map[string]any, 0, len(st.Neighbors))
		for _, neighbor := range st.Neighbors {
			if attrs := topologyPortNeighborStatusToAttributes(neighbor); len(attrs) > 0 {
				neighbors = append(neighbors, attrs)
			}
		}
		if len(neighbors) > 0 {
			portStatus["neighbors"] = neighbors
		}
	}
	if st.AdminStatus != "" {
		portStatus["admin_status"] = st.AdminStatus
	}
	if st.OperStatus != "" {
		portStatus["oper_status"] = st.OperStatus
	}
	if st.InterfaceType != "" {
		portStatus["if_type"] = st.InterfaceType
	}
	return pruneTopologyAttributes(portStatus)
}
