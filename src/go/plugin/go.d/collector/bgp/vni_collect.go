// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import "time"

func (c *Collector) collectEVPNVNIs(scrape *scrapeMetrics, families []familyStats, selectedFamilies map[string]bool) []vniStats {
	if !hasSelectedFamily(families, selectedFamilies, "l2vpn", "evpn") {
		return nil
	}

	client, ok := c.client.(frrClientAPI)
	if !ok {
		return nil
	}

	data, err := client.EVPNVNI()
	if err != nil {
		scrape.noteQueryError(err, false)
		c.Debugf("collect EVPN VNIs: %v", err)
		return nil
	}

	vnis, err := parseFRREVPNVNIs(data, c.Backend)
	if err != nil {
		scrape.noteParseError(false)
		c.Debugf("parse EVPN VNIs: %v", err)
		return nil
	}

	return vnis
}

func (c *Collector) emitVNIMetrics(mx map[string]int64, vnis []vniStats, selectedVNIs map[string]bool, now time.Time) {
	for _, vni := range vnis {
		if !selectedVNIs[vni.ID] {
			continue
		}

		if _, ok := c.vniSeen[vni.ID]; !ok {
			c.addVNICharts(vni)
		}
		c.vniSeen[vni.ID] = now

		vniKey := "vni_" + vni.ID + "_"
		mx[vniKey+"macs"] = vni.MACs
		mx[vniKey+"arp_nd"] = vni.ARPND
		mx[vniKey+"remote_vteps"] = vni.RemoteVTEPs
	}
}

func hasSelectedFamily(families []familyStats, selected map[string]bool, afi, safi string) bool {
	for _, family := range families {
		if family.AFI != afi || family.SAFI != safi {
			continue
		}
		if selected[family.ID] {
			return true
		}
	}
	return false
}
