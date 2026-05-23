// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"sort"
	"strings"
)

type fdbCandidate struct {
	mac        string
	bridgePort string
	ifIndex    int
	statusRaw  string
	vlanID     string
	vlanName   string
}

func buildFDBCandidates(entries []FDBObservation, bridgePortToIfIndex map[string]int) []fdbCandidate {
	if len(entries) == 0 {
		return nil
	}

	sorted := sortedFDBEntries(entries)
	selfMACs := make(map[string]struct{}, len(sorted))
	for _, entry := range sorted {
		if canonicalFDBStatus(entry.Status) != fdbStatusSelf {
			continue
		}
		mac := normalizeMAC(entry.MAC)
		if mac == "" {
			continue
		}
		selfMACs[mac] = struct{}{}
	}

	candidatesByEndpoint := make(map[string]fdbCandidate, len(sorted))
	duplicates := make(map[string]struct{})
	for _, entry := range sorted {
		mac := normalizeMAC(entry.MAC)
		if mac == "" {
			continue
		}
		if _, isSelf := selfMACs[mac]; isSelf {
			continue
		}
		if canonicalFDBStatus(entry.Status) != fdbStatusLearned {
			continue
		}

		bridgePort := strings.TrimSpace(entry.BridgePort)
		ifIndex := entry.IfIndex
		if ifIndex <= 0 && bridgePort != "" {
			if mappedIfIndex, ok := bridgePortToIfIndex[bridgePort]; ok {
				ifIndex = mappedIfIndex
			}
		}

		candidate := fdbCandidate{
			mac:        mac,
			bridgePort: bridgePort,
			ifIndex:    ifIndex,
			statusRaw:  strings.TrimSpace(entry.Status),
			vlanID:     strings.TrimSpace(entry.VLANID),
			vlanName:   strings.TrimSpace(entry.VLANName),
		}
		candidateKey := opaqueCompositeKey(mac)
		if candidate.vlanID != "" {
			candidateKey = opaqueCompositeKey(mac, "vlan:"+strings.ToLower(candidate.vlanID))
		}
		if _, duplicated := duplicates[candidateKey]; duplicated {
			continue
		}

		existing, exists := candidatesByEndpoint[candidateKey]
		if !exists {
			candidatesByEndpoint[candidateKey] = candidate
			continue
		}

		if sameFDBDestination(existing, candidate) {
			updated := existing
			if candidate.statusRaw != "" {
				updated.statusRaw = candidate.statusRaw
			}
			if updated.vlanName == "" && candidate.vlanName != "" {
				updated.vlanName = candidate.vlanName
			}
			candidatesByEndpoint[candidateKey] = updated
			continue
		}

		delete(candidatesByEndpoint, candidateKey)
		duplicates[candidateKey] = struct{}{}
	}

	out := make([]fdbCandidate, 0, len(candidatesByEndpoint))
	for _, candidate := range candidatesByEndpoint {
		out = append(out, candidate)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].mac != out[j].mac {
			return out[i].mac < out[j].mac
		}
		if out[i].vlanID != out[j].vlanID {
			return out[i].vlanID < out[j].vlanID
		}
		if out[i].ifIndex != out[j].ifIndex {
			return out[i].ifIndex < out[j].ifIndex
		}
		return out[i].bridgePort < out[j].bridgePort
	})
	return out
}

func canonicalFDBStatus(status string) string {
	normalized := strings.ToLower(strings.TrimSpace(status))
	switch normalized {
	case "", "3", "learned", "dot1d_tp_fdb_status_learned", "dot1dtpfdbstatuslearned":
		return fdbStatusLearned
	case "4", "self", "dot1d_tp_fdb_status_self", "dot1dtpfdbstatusself":
		return fdbStatusSelf
	default:
		if strings.Contains(normalized, "learned") {
			return fdbStatusLearned
		}
		if strings.Contains(normalized, "self") {
			return fdbStatusSelf
		}
		return fdbStatusIgnored
	}
}

func sameFDBDestination(left, right fdbCandidate) bool {
	if strings.TrimSpace(left.vlanID) != strings.TrimSpace(right.vlanID) {
		return false
	}
	if left.ifIndex > 0 && right.ifIndex > 0 {
		return left.ifIndex == right.ifIndex
	}
	return left.bridgePort != "" && left.bridgePort == right.bridgePort
}
