// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"sort"
	"strings"
)

func topologyPortVLANAttributes(vlanIDs []string, vlanNames map[string]string, linkMode string) []map[string]any {
	if len(vlanIDs) == 0 {
		return nil
	}
	tagged := len(vlanIDs) != 1 || !strings.EqualFold(strings.TrimSpace(linkMode), "access")
	out := make([]map[string]any, 0, len(vlanIDs))
	for _, vlanID := range vlanIDs {
		vlanID = normalizeTopologyVLANID(vlanID)
		if vlanID == "" {
			continue
		}
		entry := map[string]any{
			"vlan_id": vlanID,
			"tagged":  tagged,
		}
		if vlanName := strings.TrimSpace(vlanNames[vlanID]); vlanName != "" {
			entry["vlan_name"] = vlanName
		}
		out = append(out, entry)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeTopologySTPState(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "0":
		return ""
	case "1", "disabled":
		return "disabled"
	case "2", "blocking", "discarding":
		return "blocking"
	case "3", "listening":
		return "listening"
	case "4", "learning":
		return "learning"
	case "5", "forwarding":
		return "forwarding"
	case "6", "broken":
		return "broken"
	default:
		return value
	}
}

func summarizeTopologySTPState(states map[string]struct{}) string {
	if len(states) == 0 {
		return ""
	}

	rank := map[string]int{
		"forwarding": 1,
		"learning":   2,
		"listening":  3,
		"blocking":   4,
		"disabled":   5,
		"broken":     6,
	}
	selected := ""
	selectedRank := -1
	for state := range states {
		state = normalizeTopologySTPState(state)
		if state == "" {
			continue
		}
		currentRank, ok := rank[state]
		if !ok {
			currentRank = 7
		}
		if currentRank > selectedRank {
			selected = state
			selectedRank = currentRank
		}
	}
	return selected
}

func ensureTopologyPortEvidence(
	evidenceByIfIndex map[int]*topologyDevicePortEvidence,
	ifIndex int,
) *topologyDevicePortEvidence {
	if ifIndex <= 0 {
		return nil
	}
	evidence := evidenceByIfIndex[ifIndex]
	if evidence == nil {
		evidence = &topologyDevicePortEvidence{
			vlanIDs:        make(map[string]struct{}),
			vlanNames:      make(map[string]string),
			fdbEndpointIDs: make(map[string]struct{}),
			stpStates:      make(map[string]struct{}),
			neighbors:      make(map[string]topologyPortNeighborStatus),
		}
		evidenceByIfIndex[ifIndex] = evidence
	}
	return evidence
}

func resolveAdjacencySourceIfIndex(adj Adjacency, ifIndexByDeviceName map[string]int) int {
	ifIndex := 0
	if ifName := strings.TrimSpace(adj.SourcePort); ifName != "" {
		ifIndex = resolveIfIndexByPortName(adj.SourceID, ifName, ifIndexByDeviceName)
	}
	return ifIndex
}

func classifyTopologyPortLinkMode(evidence *topologyDevicePortEvidence) (mode string, confidence string, sources []string, vlans []string) {
	mode = "unknown"
	confidence = "low"
	if evidence == nil {
		return mode, confidence, nil, nil
	}

	if len(evidence.vlanIDs) > 0 {
		vlans = sortedTopologySet(evidence.vlanIDs)
	}
	if evidence.hasFDB {
		sources = append(sources, "fdb")
	}
	if evidence.hasSTP {
		sources = append(sources, "stp")
	}
	if evidence.hasPeer {
		sources = append(sources, "peer_link")
	}

	switch vlanCount := len(evidence.vlanIDs); {
	case vlanCount >= 2:
		mode = "trunk"
		if evidence.hasFDB && evidence.hasSTP {
			confidence = "high"
		} else {
			confidence = "medium"
		}
	case vlanCount == 1 && !evidence.hasPeer:
		mode = "access"
		confidence = "medium"
	default:
		mode = "unknown"
		confidence = "low"
	}
	return mode, confidence, sources, vlans
}

func classifyTopologyPortRole(evidence *topologyDevicePortEvidence) (role string, confidence string, sources []string) {
	role = "unknown"
	confidence = "low"
	if evidence == nil {
		return role, confidence, nil
	}

	if evidence.hasPeer {
		sources = append(sources, "peer_link")
	}
	if evidence.hasBridgeLink {
		sources = append(sources, "bridge_link")
	}
	if evidence.hasSTP {
		sources = append(sources, "stp")
	}
	if evidence.hasFDB {
		sources = append(sources, "fdb")
	}
	if evidence.hasFDBManagedAlias {
		sources = append(sources, "fdb_managed_alias")
	}
	if evidence.isLAG {
		sources = append(sources, "lag_interface")
	}

	switch {
	case evidence.hasPeer || evidence.hasBridgeLink:
		role = "switch_facing"
		confidence = "high"
	case evidence.hasSTP && evidence.hasFDBManagedAlias:
		role = "switch_facing"
		confidence = "medium"
	case evidence.isLAG && evidence.hasFDB:
		role = "switch_facing"
		confidence = "medium"
	case evidence.hasFDB && len(evidence.fdbEndpointIDs) == 1 && !evidence.hasSTP:
		role = "host_facing"
		confidence = "medium"
	case evidence.hasFDB && !evidence.hasSTP:
		role = "host_candidate"
		confidence = "low"
	default:
		role = "unknown"
		confidence = "low"
	}
	return role, confidence, sources
}

func isTopologyLAGInterfaceType(ifType string) bool {
	switch strings.ToLower(strings.TrimSpace(ifType)) {
	case "ieee8023adlag", "lag", "bond":
		return true
	default:
		return false
	}
}

func intCountMapToAny(in map[string]int) map[string]any {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for key := range in {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return nil
	}
	sort.Strings(keys)
	out := make(map[string]any, len(keys))
	for _, key := range keys {
		out[key] = in[key]
	}
	return out
}
