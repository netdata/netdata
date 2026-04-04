// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"fmt"
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
		return selected
	}

	for _, family := range families {
		if c.selectFamilyMatcher.MatchString(familyMatchKey(family)) {
			selected[family.ID] = true
		}
	}

	return selected
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
	return selected
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
		return selected
	}

	for _, vni := range vnis {
		if c.selectVNIMatcher.MatchString(vniMatchKey(vni)) {
			selected[vni.ID] = true
		}
	}

	return selected
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
