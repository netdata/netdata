// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"fmt"
	"sort"
	"strings"
)

func (c *Collector) selectFamilies(families []familyStats) map[string]bool {
	selected := make(map[string]bool)

	if len(families) <= c.MaxFamilies {
		for _, family := range families {
			selected[family.ID] = true
		}
		return selected
	}

	if c.selectFamilyMatcher == nil {
		for _, family := range sortedFamilies(families)[:c.MaxFamilies] {
			selected[family.ID] = true
		}
		return selected
	}

	for _, family := range families {
		if c.selectFamilyMatcher.MatchString(familyMatchKey(family)) {
			selected[family.ID] = true
		}
	}
	if len(selected) <= c.MaxFamilies {
		return selected
	}

	limited := make(map[string]bool, c.MaxFamilies)
	for _, family := range sortedFamilies(families) {
		if !selected[family.ID] {
			continue
		}
		limited[family.ID] = true
		if len(limited) == c.MaxFamilies {
			break
		}
	}
	return limited
}

func (c *Collector) selectPeers(families []familyStats, selectedFamilies map[string]bool) map[string]bool {
	selected := make(map[string]bool)

	total := 0
	for _, family := range families {
		if !selectedFamilies[family.ID] {
			continue
		}
		total += len(family.Peers)
	}
	if total <= c.MaxPeers {
		for _, family := range families {
			if !selectedFamilies[family.ID] {
				continue
			}
			for _, peer := range family.Peers {
				selected[peer.ID] = true
			}
		}
		return selected
	}
	if c.selectPeerMatcher == nil {
		for _, peerID := range sortedPeerIDs(families, selectedFamilies)[:c.MaxPeers] {
			selected[peerID] = true
		}
		return selected
	}

	for _, family := range families {
		if !selectedFamilies[family.ID] {
			continue
		}
		for _, peer := range family.Peers {
			if c.selectPeerMatcher.MatchString(peer.Address) {
				selected[peer.ID] = true
			}
		}
	}
	if len(selected) <= c.MaxPeers {
		return selected
	}

	limited := make(map[string]bool, c.MaxPeers)
	for _, peerID := range sortedSelectedIDs(selected)[:c.MaxPeers] {
		limited[peerID] = true
	}
	return limited
}

func (c *Collector) selectVNIs(vnis []vniStats) map[string]bool {
	selected := make(map[string]bool)

	if len(vnis) <= c.MaxVNIs {
		for _, vni := range vnis {
			selected[vni.ID] = true
		}
		return selected
	}

	if c.selectVNIMatcher == nil {
		for _, vni := range sortedVNIs(vnis)[:c.MaxVNIs] {
			selected[vni.ID] = true
		}
		return selected
	}

	for _, vni := range vnis {
		if c.selectVNIMatcher.MatchString(vniMatchKey(vni)) {
			selected[vni.ID] = true
		}
	}
	if len(selected) <= c.MaxVNIs {
		return selected
	}

	limited := make(map[string]bool, c.MaxVNIs)
	for _, vni := range sortedVNIs(vnis) {
		if !selected[vni.ID] {
			continue
		}
		limited[vni.ID] = true
		if len(limited) == c.MaxVNIs {
			break
		}
	}
	return limited
}

func familyMatchKey(f familyStats) string {
	return strings.ToLower(strings.TrimSpace(fmt.Sprintf("%s/%s/%s", f.VRF, f.AFI, f.SAFI)))
}

func vniMatchKey(vni vniStats) string {
	return strings.ToLower(strings.TrimSpace(fmt.Sprintf("%s/%d", vni.TenantVRF, vni.VNI)))
}

func countSelectedPeers(family familyStats, selected map[string]bool) int {
	var count int
	for _, peer := range family.Peers {
		if selected[peer.ID] {
			count++
		}
	}
	return count
}

func sortedFamilies(families []familyStats) []familyStats {
	sorted := append([]familyStats(nil), families...)
	sort.Slice(sorted, func(i, j int) bool {
		return familyMatchKey(sorted[i]) < familyMatchKey(sorted[j])
	})
	return sorted
}

func sortedPeerIDs(families []familyStats, selectedFamilies map[string]bool) []string {
	var ids []string
	for _, family := range families {
		if !selectedFamilies[family.ID] {
			continue
		}
		for _, peer := range family.Peers {
			ids = append(ids, peer.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func sortedVNIs(vnis []vniStats) []vniStats {
	sorted := append([]vniStats(nil), vnis...)
	sort.Slice(sorted, func(i, j int) bool {
		return vniMatchKey(sorted[i]) < vniMatchKey(sorted[j])
	})
	return sorted
}

func sortedSelectedIDs(selected map[string]bool) []string {
	ids := make([]string, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
